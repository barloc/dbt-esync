// Package config loads and validates the YAML --config contract.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/barloc/dbt-esync/app/source"
)

// Config is the full dbt-esync YAML schema.
type Config struct {
	SavePath     string       `yaml:"save_path"`
	DefaultOwner DefaultOwner `yaml:"default_owner"`
	Manifest     Manifest     `yaml:"manifest"`
	Parse        Parse        `yaml:"parse"`
	Source       Source       `yaml:"source"`
	Sink         Sink         `yaml:"sink"`
}

// DefaultOwner is used when a source does not provide an owner.
// Per-source owner overrides are not supported — owners live only here.
type DefaultOwner struct {
	Name  string `yaml:"name"`
	Email string `yaml:"email"`
	Slack string `yaml:"slack"`
}

// Manifest locates dbt manifest.json (exactly one of URL or File).
type Manifest struct {
	URL        string `yaml:"url"`
	File       string `yaml:"file"`
	FilterName string `yaml:"filter_name"`
}

// Parse holds SQL/Qlik reference filters.
type Parse struct {
	ExcludedCatalogs []string `yaml:"excluded_catalogs"`
	SchemaAllowlist  []string `yaml:"schema_allowlist"`
}

// Source selects one collector and its typed options.
type Source struct {
	Type       string            `yaml:"type"`
	Qlik       *QlikConfig       `yaml:"qlik,omitempty"`
	Redash     *RedashConfig     `yaml:"redash,omitempty"`
	Gitscan    *GitscanConfig    `yaml:"gitscan,omitempty"`
	Hive       *SQLCatalog       `yaml:"hive,omitempty"`
	MySQL      *SQLCatalog       `yaml:"mysql,omitempty"`
	GrowthBook *GrowthBookConfig `yaml:"growthbook,omitempty"`
}

// QlikConfig is source.type: qlik.
type QlikConfig struct {
	Host                string    `yaml:"host"`
	User                string    `yaml:"user"`
	Password            string    `yaml:"password"`
	Datasource          string    `yaml:"datasource"`
	StreamInclude       []string  `yaml:"stream_include"`
	StreamExclude       []string  `yaml:"stream_exclude"`
	ExposureURLTemplate string    `yaml:"exposure_url_template"`
	QRC                 QRCConfig `yaml:"qrc"`
}

// QRCConfig is optional QRS/QRC identity for tags/maturity.
type QRCConfig struct {
	Host          string `yaml:"host"`
	CertB64       string `yaml:"cert_b64"`
	Password      string `yaml:"password"`
	UserDirectory string `yaml:"user_directory"`
	UserID        string `yaml:"user_id"`
}

// RedashConfig is source.type: redash.
type RedashConfig struct {
	Host                   string   `yaml:"host"`
	APIKey                 string   `yaml:"api_key"`
	DatasourceNameContains string   `yaml:"datasource_name_contains"`
	EntityTypes            []string `yaml:"entity_types"`
	HostToDisplay          string   `yaml:"host_to_display"`
}

// GitscanConfig is source.type: gitscan.
type GitscanConfig struct {
	RepoURL      string   `yaml:"repo_url"`
	Login        string   `yaml:"login"`
	Token        string   `yaml:"token"`
	LocalPath    string   `yaml:"local_path"`
	Dirs         []string `yaml:"dirs"`
	Extensions   []string `yaml:"extensions"`
	PathDBTokens []string `yaml:"path_db_tokens"`
	VCS          string   `yaml:"vcs"`
}

// SQLCatalog is the shared schema for source.type hive and mysql:
// read rows (id + SQL/script text) from a table or custom query.
type SQLCatalog struct {
	Host                string `yaml:"host"`
	Port                int    `yaml:"port"`
	User                string `yaml:"user"`
	Password            string `yaml:"password"`
	Database            string `yaml:"database"`
	Query               string `yaml:"query"`
	Table               string `yaml:"table"`
	IDColumn            string `yaml:"id_column"`
	SQLColumn           string `yaml:"sql_column"`
	DescriptionColumn   string `yaml:"description_column"`
	ExtraFilters        string `yaml:"extra_filters"`
	ExposurePrefix      string `yaml:"exposure_prefix"`
	ExposureURLTemplate string `yaml:"exposure_url_template"`
}

// GrowthBookConfig is source.type: growthbook.
type GrowthBookConfig struct {
	Host         string `yaml:"host"`
	APIKey       string `yaml:"api_key"`
	FilterSource string `yaml:"filter_source"`
	ExposureURL  string `yaml:"exposure_url"`
}

// Sink writes exposures to a target dbt repo (and optional MR/PR).
type Sink struct {
	Git GitSink `yaml:"git"`
	MR  MRSink  `yaml:"mr"`
}

// GitSink is the write target for exposure YAML.
type GitSink struct {
	RepoURL       string `yaml:"repo_url"`
	User          string `yaml:"user"`
	Token         string `yaml:"token"`
	LocalPath     string `yaml:"local_path"`
	ExposuresPath string `yaml:"exposures_path"`
	Branch        string `yaml:"branch"`
	BaseBranch    string `yaml:"base_branch"`
}

// MRSink opens a GitLab MR or Bitbucket PR after push.
type MRSink struct {
	Enabled  bool   `yaml:"enabled"`
	Provider string `yaml:"provider"`
	Host     string `yaml:"host"`
}

// Load reads and unmarshals a YAML config file, then applies DBT_ESYNC_* env overrides.
// It does not validate — call Validate separately.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := ApplyEnvOverrides(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Validate checks required fields and invariants.
func (c *Config) Validate() error {
	if c.Source.Type == "" {
		return fmt.Errorf("config: source.type is required")
	}
	if !knownSourceType(c.Source.Type) {
		return fmt.Errorf("config: unknown source.type %q", c.Source.Type)
	}

	hasURL := c.Manifest.URL != ""
	hasFile := c.Manifest.File != ""
	switch {
	case hasURL && hasFile:
		return fmt.Errorf("config: manifest.url and manifest.file are mutually exclusive")
	case !hasURL && !hasFile:
		return fmt.Errorf("config: manifest.url or manifest.file is required")
	}

	if c.SavePath == "" {
		return fmt.Errorf("config: save_path is required")
	}
	if err := c.validateSink(); err != nil {
		return err
	}

	switch c.Source.Type {
	case "qlik":
		return requireHostBlock("source.qlik", c.Source.Qlik == nil, func() string { return c.Source.Qlik.Host })
	case "redash":
		return requireHostBlock("source.redash", c.Source.Redash == nil, func() string { return c.Source.Redash.Host })
	case "gitscan":
		if c.Source.Gitscan == nil {
			return fmt.Errorf("config: source.gitscan is required when source.type is gitscan")
		}
		if c.Source.Gitscan.RepoURL == "" {
			return fmt.Errorf("config: source.gitscan.repo_url is required")
		}
	case "hive":
		return requireSQLCatalog("source.hive", c.Source.Hive)
	case "mysql":
		return requireSQLCatalog("source.mysql", c.Source.MySQL)
	case "growthbook":
		return requireHostBlock("source.growthbook", c.Source.GrowthBook == nil, func() string { return c.Source.GrowthBook.Host })
	}
	return nil
}

func (c *Config) validateSink() error {
	g := c.Sink.Git
	switch {
	case g.RepoURL == "":
		return fmt.Errorf("config: sink.git.repo_url is required")
	case g.ExposuresPath == "":
		return fmt.Errorf("config: sink.git.exposures_path is required")
	case g.User == "":
		return fmt.Errorf("config: sink.git.user is required")
	case g.Token == "":
		return fmt.Errorf("config: sink.git.token is required")
	case g.LocalPath == "":
		return fmt.Errorf("config: sink.git.local_path is required")
	case g.Branch == "":
		return fmt.Errorf("config: sink.git.branch is required")
	case g.BaseBranch == "":
		return fmt.Errorf("config: sink.git.base_branch is required")
	}

	if !c.Sink.MR.Enabled {
		return nil
	}
	switch c.Sink.MR.Provider {
	case "gitlab", "bitbucket":
	case "":
		return fmt.Errorf("config: sink.mr.provider is required when sink.mr.enabled")
	default:
		return fmt.Errorf("config: sink.mr.provider must be gitlab or bitbucket")
	}
	if c.Sink.MR.Host == "" {
		return fmt.Errorf("config: sink.mr.host is required when sink.mr.enabled")
	}
	return nil
}

func requireSQLCatalog(block string, cat *SQLCatalog) error {
	if cat == nil {
		return fmt.Errorf("config: %s is required when source.type matches", block)
	}
	if cat.Host == "" {
		return fmt.Errorf("config: %s.host is required", block)
	}
	return nil
}

func requireHostBlock(block string, missing bool, host func() string) error {
	if missing {
		return fmt.Errorf("config: %s is required when source.type matches", block)
	}
	if host() == "" {
		return fmt.Errorf("config: %s.host is required", block)
	}
	return nil
}

func knownSourceType(typ string) bool {
	for _, k := range source.KnownTypes {
		if k == typ {
			return true
		}
	}
	return false
}

// Package config loads and validates the YAML --config contract.
package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the full dbt-esync YAML schema.
type Config struct {
	DefaultOwner DefaultOwner  `yaml:"default_owner"`
	Manifest     Manifest      `yaml:"manifest"`
	Parse        Parse         `yaml:"parse"`
	Source       []SourceEntry `yaml:"source"`
	Sink         Sink          `yaml:"sink"`
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

// SourceEntry is one named collector in the source array.
// Name is the package directory under sink.git.exposures_path.
type SourceEntry struct {
	Name       string            `yaml:"name"`
	Type       string            `yaml:"type"`
	Qlik       *QlikConfig       `yaml:"qlik,omitempty"`
	Redash     *RedashConfig     `yaml:"redash,omitempty"`
	Gitscan    *GitscanConfig    `yaml:"gitscan,omitempty"`
	Hive       *SQLCatalog       `yaml:"hive,omitempty"`
	MySQL      *SQLCatalog       `yaml:"mysql,omitempty"`
	GrowthBook *GrowthBookConfig `yaml:"growthbook,omitempty"`
	Superset   *SupersetConfig   `yaml:"superset,omitempty"`
}

// QlikConfig is source[].type: qlik.
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

// RedashConfig is source[].type: redash.
type RedashConfig struct {
	Host                   string   `yaml:"host"`
	APIKey                 string   `yaml:"api_key"`
	DatasourceNameContains string   `yaml:"datasource_name_contains"`
	EntityTypes            []string `yaml:"entity_types"`
	HostToDisplay          string   `yaml:"host_to_display"`
	UseAPIOwner            bool     `yaml:"use_api_owner"` // false → default_owner; true → Redash user
}

// GitscanConfig is source[].type: gitscan.
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

// SQLCatalog is the shared schema for source[].type hive and mysql:
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

// GrowthBookConfig is source[].type: growthbook.
type GrowthBookConfig struct {
	Host         string `yaml:"host"`
	APIKey       string `yaml:"api_key"`
	FilterSource string `yaml:"filter_source"`
	ExposureURL  string `yaml:"exposure_url"`
}

// SupersetConfig is source[].type: superset.
// Auth: exactly one of APIKey or (Username + Password → JWT login).
type SupersetConfig struct {
	Host                 string `yaml:"host"`
	APIKey               string `yaml:"api_key"`
	Username             string `yaml:"username"`
	Password             string `yaml:"password"`
	DatabaseNameContains string `yaml:"database_name_contains"`
	HostToDisplay        string `yaml:"host_to_display"`
	UseAPIOwner          bool   `yaml:"use_api_owner"` // false → default_owner; true → first owner
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
	// CommitName / CommitEmail override the default commit author (dbt-esync@localhost).
	// Corp GitLab push rules often require a real company email.
	CommitName  string `yaml:"commit_name"`
	CommitEmail string `yaml:"commit_email"`
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
	if len(c.Source) == 0 {
		return fmt.Errorf("config: source must contain at least one entry")
	}

	hasURL := c.Manifest.URL != ""
	hasFile := c.Manifest.File != ""
	switch {
	case hasURL && hasFile:
		return fmt.Errorf("config: manifest.url and manifest.file are mutually exclusive")
	case !hasURL && !hasFile:
		return fmt.Errorf("config: manifest.url or manifest.file is required")
	}

	if err := c.validateSink(); err != nil {
		return err
	}

	seen := make(map[string]struct{}, len(c.Source))
	for i := range c.Source {
		if err := validateSourceEntry(i, &c.Source[i], seen); err != nil {
			return err
		}
	}
	return nil
}

func validateSourceEntry(i int, e *SourceEntry, seen map[string]struct{}) error {
	prefix := fmt.Sprintf("source[%d]", i)
	if e.Name == "" {
		return fmt.Errorf("config: %s.name is required", prefix)
	}
	if err := validatePackageName(e.Name); err != nil {
		return fmt.Errorf("config: %s.name: %w", prefix, err)
	}
	if _, dup := seen[e.Name]; dup {
		return fmt.Errorf("config: duplicate source name %q", e.Name)
	}
	seen[e.Name] = struct{}{}

	if e.Type == "" {
		return fmt.Errorf("config: %s.type is required", prefix)
	}
	if !knownSourceType(e.Type) {
		return fmt.Errorf("config: %s: unknown source.type %q", prefix, e.Type)
	}

	switch e.Type {
	case "qlik":
		return validateQlik(prefix+".qlik", e.Qlik)
	case "redash":
		return validateRedash(prefix+".redash", e.Redash)
	case "gitscan":
		return validateGitscan(prefix+".gitscan", e.Gitscan)
	case "hive":
		return requireSQLCatalog(prefix+".hive", e.Hive)
	case "mysql":
		return requireSQLCatalog(prefix+".mysql", e.MySQL)
	case "growthbook":
		return validateGrowthBook(prefix+".growthbook", e.GrowthBook)
	case "superset":
		return validateSuperset(prefix+".superset", e.Superset)
	}
	return nil
}

func validateGrowthBook(block string, gb *GrowthBookConfig) error {
	if gb == nil {
		return fmt.Errorf("config: %s is required when source.type matches", block)
	}
	switch {
	case gb.Host == "":
		return fmt.Errorf("config: %s.host is required", block)
	case gb.APIKey == "":
		return fmt.Errorf("config: %s.api_key is required", block)
	case gb.FilterSource == "":
		return fmt.Errorf("config: %s.filter_source is required", block)
	case gb.ExposureURL == "":
		return fmt.Errorf("config: %s.exposure_url is required", block)
	}
	return nil
}

func validateQlik(block string, q *QlikConfig) error {
	if q == nil {
		return fmt.Errorf("config: %s is required when source.type matches", block)
	}
	switch {
	case q.Host == "":
		return fmt.Errorf("config: %s.host is required", block)
	case q.Datasource == "":
		return fmt.Errorf("config: %s.datasource is required", block)
	case q.ExposureURLTemplate == "":
		return fmt.Errorf("config: %s.exposure_url_template is required", block)
	}
	return nil
}

func validateGitscan(block string, g *GitscanConfig) error {
	if g == nil {
		return fmt.Errorf("config: %s is required when source.type matches", block)
	}
	switch {
	case g.RepoURL == "":
		return fmt.Errorf("config: %s.repo_url is required", block)
	case g.LocalPath == "":
		return fmt.Errorf("config: %s.local_path is required", block)
	case len(g.Extensions) == 0:
		return fmt.Errorf("config: %s.extensions is required", block)
	}
	switch strings.ToLower(g.VCS) {
	case "gitlab", "bitbucket":
	case "":
		return fmt.Errorf("config: %s.vcs is required", block)
	default:
		return fmt.Errorf("config: %s.vcs: unknown %q (want gitlab or bitbucket)", block, g.VCS)
	}
	return nil
}

func validateRedash(block string, r *RedashConfig) error {
	if r == nil {
		return fmt.Errorf("config: %s is required when source.type matches", block)
	}
	switch {
	case r.Host == "":
		return fmt.Errorf("config: %s.host is required", block)
	case r.APIKey == "":
		return fmt.Errorf("config: %s.api_key is required", block)
	case r.DatasourceNameContains == "":
		return fmt.Errorf("config: %s.datasource_name_contains is required", block)
	}
	seen := map[string]struct{}{}
	for _, et := range r.EntityTypes {
		switch et {
		case "dashboard", "alerts":
		default:
			return fmt.Errorf("config: %s.entity_types: unknown %q (want dashboard or alerts)", block, et)
		}
		if _, dup := seen[et]; dup {
			return fmt.Errorf("config: %s.entity_types: duplicate %q", block, et)
		}
		seen[et] = struct{}{}
	}
	return nil
}

func validateSuperset(block string, s *SupersetConfig) error {
	if s == nil {
		return fmt.Errorf("config: %s is required when source.type matches", block)
	}
	if s.Host == "" {
		return fmt.Errorf("config: %s.host is required", block)
	}
	if s.DatabaseNameContains == "" {
		return fmt.Errorf("config: %s.database_name_contains is required", block)
	}
	hasKey := s.APIKey != ""
	hasUser := s.Username != ""
	hasPass := s.Password != ""
	hasLogin := hasUser && hasPass
	switch {
	case hasUser != hasPass:
		return fmt.Errorf("config: %s.username and %s.password must both be set for login auth", block, block)
	case hasKey && hasLogin:
		return fmt.Errorf("config: %s: set either api_key or username/password, not both", block)
	case !hasKey && !hasLogin:
		return fmt.Errorf("config: %s: api_key or username/password is required", block)
	}
	return nil
}

func validatePackageName(name string) error {
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("must not contain path separators")
	}
	if name == ".." {
		return fmt.Errorf("invalid package name %q", name)
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
	switch {
	case cat.Host == "":
		return fmt.Errorf("config: %s.host is required", block)
	case cat.User == "":
		return fmt.Errorf("config: %s.user is required", block)
	case cat.Database == "":
		return fmt.Errorf("config: %s.database is required", block)
	case cat.IDColumn == "":
		return fmt.Errorf("config: %s.id_column is required", block)
	case cat.SQLColumn == "":
		return fmt.Errorf("config: %s.sql_column is required", block)
	case cat.ExposurePrefix == "":
		return fmt.Errorf("config: %s.exposure_prefix is required", block)
	case cat.ExposureURLTemplate == "":
		return fmt.Errorf("config: %s.exposure_url_template is required", block)
	}

	hasQuery := cat.Query != ""
	hasTable := cat.Table != ""
	switch {
	case hasQuery && hasTable:
		return fmt.Errorf("config: %s.query and %s.table are mutually exclusive", block, block)
	case !hasQuery && !hasTable:
		return fmt.Errorf("config: %s.query or %s.table is required", block, block)
	}
	return nil
}

// KnownSourceTypes is the single source of truth for source[].type values.
// app/source.KnownTypes aliases this (config cannot import source).
var KnownSourceTypes = []string{
	"growthbook",
	"mysql",
	"redash",
	"hive",
	"gitscan",
	"qlik",
	"superset",
}

func knownSourceType(typ string) bool {
	for _, k := range KnownSourceTypes {
		if k == typ {
			return true
		}
	}
	return false
}

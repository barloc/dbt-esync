package config_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/barloc/dbt-esync/app/config"
)

func TestUnitLoadAndValidateQlikExample(t *testing.T) {
	// No t.Parallel: Load reads process env (see env_test.go).
	cfg, err := config.Load(writeTempYAML(t, validQlikYAML()))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(cfg.Source) != 1 || cfg.Source[0].Type != "qlik" {
		t.Fatalf("source: %+v", cfg.Source)
	}
	if cfg.Source[0].Name != "bi_qlik_sense" {
		t.Fatalf("name: got %q", cfg.Source[0].Name)
	}
	if cfg.Source[0].Qlik == nil {
		t.Fatal("expected source[0].qlik")
	}
}

func TestUnitAcceptStreamLists(t *testing.T) {
	cfg, err := config.Load(writeTempYAML(t, `
manifest:
  file: m.json
source:
  - name: bi_qlik_sense
    type: qlik
    qlik:
      host: qlik.example.com
      stream_include: ["Prod", "Shared"]
      stream_exclude: ["99-Archive", "92-TMP"]
`+validSinkYAML()))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	gotIn := cfg.Source[0].Qlik.StreamInclude
	if len(gotIn) != 2 || gotIn[0] != "Prod" || gotIn[1] != "Shared" {
		t.Fatalf("stream_include: got %v", gotIn)
	}
	gotEx := cfg.Source[0].Qlik.StreamExclude
	if len(gotEx) != 2 || gotEx[0] != "99-Archive" || gotEx[1] != "92-TMP" {
		t.Fatalf("stream_exclude: got %v", gotEx)
	}
}

func TestUnitValidateRequiresSourceEntries(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Manifest: config.Manifest{File: "m.json"},
		Sink:     config.Sink{Git: validGitSink()},
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "source must contain at least one entry") {
		t.Fatalf("got %v", err)
	}
}

func TestUnitValidateRequiresName(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Manifest: config.Manifest{File: "m.json"},
		Source:   []config.SourceEntry{{Type: "qlik", Qlik: &config.QlikConfig{Host: "q"}}},
		Sink:     config.Sink{Git: validGitSink()},
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "name is required") {
		t.Fatalf("got %v", err)
	}
}

func TestUnitValidateRejectsBadPackageName(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"a/b", `a\b`, ".."} {
		cfg := &config.Config{
			Manifest: config.Manifest{File: "m.json"},
			Source:   []config.SourceEntry{{Name: name, Type: "qlik", Qlik: &config.QlikConfig{Host: "q"}}},
			Sink:     config.Sink{Git: validGitSink()},
		}
		err := cfg.Validate()
		if err == nil {
			t.Fatalf("name %q: expected error", name)
		}
	}
}

func TestUnitValidateRejectsDuplicateName(t *testing.T) {
	t.Parallel()

	entry := config.SourceEntry{Name: "pkg", Type: "qlik", Qlik: &config.QlikConfig{Host: "q"}}
	cfg := &config.Config{
		Manifest: config.Manifest{File: "m.json"},
		Source:   []config.SourceEntry{entry, entry},
		Sink:     config.Sink{Git: validGitSink()},
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "duplicate source name") {
		t.Fatalf("got %v", err)
	}
}

func TestUnitValidateRejectsEmptyType(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Manifest: config.Manifest{File: "m.json"},
		Source:   []config.SourceEntry{{Name: "pkg"}},
		Sink:     config.Sink{Git: validGitSink()},
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "type is required") {
		t.Fatalf("got %v", err)
	}
}

func TestUnitValidateRejectsUnknownType(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Manifest: config.Manifest{File: "m.json"},
		Source:   []config.SourceEntry{{Name: "pkg", Type: "nope"}},
		Sink:     config.Sink{Git: validGitSink()},
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "unknown source.type") {
		t.Fatalf("got %v", err)
	}
}

func TestUnitValidateManifestExclusive(t *testing.T) {
	t.Parallel()

	both := &config.Config{
		Manifest: config.Manifest{URL: "https://example/m.json", File: "m.json"},
		Source:   []config.SourceEntry{{Name: "pkg", Type: "qlik", Qlik: &config.QlikConfig{Host: "q"}}},
		Sink:     config.Sink{Git: validGitSink()},
	}
	if err := both.Validate(); err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("both: got %v", err)
	}

	neither := &config.Config{
		Source: []config.SourceEntry{{Name: "pkg", Type: "qlik", Qlik: &config.QlikConfig{Host: "q"}}},
		Sink:   config.Sink{Git: validGitSink()},
	}
	if err := neither.Validate(); err == nil || !strings.Contains(err.Error(), "manifest.url or manifest.file") {
		t.Fatalf("neither: got %v", err)
	}
}

func TestUnitValidateRequiresSourceBlock(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Manifest: config.Manifest{File: "m.json"},
		Source:   []config.SourceEntry{{Name: "pkg", Type: "qlik"}},
		Sink:     config.Sink{Git: validGitSink()},
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "qlik is required") {
		t.Fatalf("got %v", err)
	}
}

func TestUnitValidateGrowthBookFields(t *testing.T) {
	t.Parallel()

	base := func(gb *config.GrowthBookConfig) *config.Config {
		return &config.Config{
			Manifest: config.Manifest{File: "m.json"},
			Source:   []config.SourceEntry{{Name: "pkg", Type: "growthbook", GrowthBook: gb}},
			Sink:     config.Sink{Git: validGitSink()},
		}
	}
	full := &config.GrowthBookConfig{
		Host:         "https://gb.example.com/api/v1/data-sources",
		APIKey:       "k",
		FilterSource: "starrocks",
		ExposureURL:  "https://gb.example.com/datasources",
	}
	if err := base(full).Validate(); err != nil {
		t.Fatalf("full: %v", err)
	}
	if err := base(nil).Validate(); err == nil || !strings.Contains(err.Error(), "growthbook is required") {
		t.Fatalf("nil: got %v", err)
	}
	cases := []struct {
		mut  func(*config.GrowthBookConfig)
		want string
	}{
		{func(g *config.GrowthBookConfig) { g.Host = "" }, "host is required"},
		{func(g *config.GrowthBookConfig) { g.APIKey = "" }, "api_key is required"},
		{func(g *config.GrowthBookConfig) { g.FilterSource = "" }, "filter_source is required"},
		{func(g *config.GrowthBookConfig) { g.ExposureURL = "" }, "exposure_url is required"},
	}
	for _, tc := range cases {
		g := *full
		tc.mut(&g)
		err := base(&g).Validate()
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s: got %v", tc.want, err)
		}
	}
}

func TestUnitValidateRequiresSinkGit(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Manifest: config.Manifest{File: "m.json"},
		Source:   []config.SourceEntry{{Name: "pkg", Type: "qlik", Qlik: &config.QlikConfig{Host: "q"}}},
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "sink.git.repo_url") {
		t.Fatalf("got %v", err)
	}
}

func TestUnitValidateRequiresExposuresPath(t *testing.T) {
	t.Parallel()

	g := validGitSink()
	g.ExposuresPath = ""
	cfg := &config.Config{
		Manifest: config.Manifest{File: "m.json"},
		Source:   []config.SourceEntry{{Name: "pkg", Type: "qlik", Qlik: &config.QlikConfig{Host: "q"}}},
		Sink:     config.Sink{Git: g},
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "sink.git.exposures_path") {
		t.Fatalf("got %v", err)
	}
}

func TestUnitValidateRequiresSinkGitFields(t *testing.T) {
	t.Parallel()

	fields := []struct {
		name string
		mut  func(*config.GitSink)
		want string
	}{
		{"user", func(g *config.GitSink) { g.User = "" }, "sink.git.user"},
		{"token", func(g *config.GitSink) { g.Token = "" }, "sink.git.token"},
		{"local_path", func(g *config.GitSink) { g.LocalPath = "" }, "sink.git.local_path"},
		{"branch", func(g *config.GitSink) { g.Branch = "" }, "sink.git.branch"},
		{"base_branch", func(g *config.GitSink) { g.BaseBranch = "" }, "sink.git.base_branch"},
	}
	for _, tc := range fields {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := validGitSink()
			tc.mut(&g)
			cfg := &config.Config{
				Manifest: config.Manifest{File: "m.json"},
				Source:   []config.SourceEntry{{Name: "pkg", Type: "qlik", Qlik: &config.QlikConfig{Host: "q"}}},
				Sink:     config.Sink{Git: g},
			}
			err := cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
		})
	}
}

func TestUnitValidateMRWhenEnabled(t *testing.T) {
	t.Parallel()

	base := func(mr config.MRSink) *config.Config {
		return &config.Config{
			Manifest: config.Manifest{File: "m.json"},
			Source:   []config.SourceEntry{{Name: "pkg", Type: "qlik", Qlik: &config.QlikConfig{Host: "q"}}},
			Sink:     config.Sink{Git: validGitSink(), MR: mr},
		}
	}

	if err := base(config.MRSink{Enabled: true}).Validate(); err == nil || !strings.Contains(err.Error(), "sink.mr.provider") {
		t.Fatalf("missing provider: %v", err)
	}
	if err := base(config.MRSink{Enabled: true, Provider: "gitea", Host: "https://git.example"}).Validate(); err == nil || !strings.Contains(err.Error(), "gitlab or bitbucket") {
		t.Fatalf("bad provider: %v", err)
	}
	if err := base(config.MRSink{Enabled: true, Provider: "gitlab"}).Validate(); err == nil || !strings.Contains(err.Error(), "sink.mr.host") {
		t.Fatalf("missing host: %v", err)
	}
	if err := base(config.MRSink{Enabled: true, Provider: "gitlab", Host: "https://git.example"}).Validate(); err != nil {
		t.Fatalf("valid mr: %v", err)
	}
	if err := base(config.MRSink{}).Validate(); err != nil {
		t.Fatalf("mr disabled: %v", err)
	}
}

func TestUnitValidateSQLCatalogFields(t *testing.T) {
	t.Parallel()

	full := func() *config.SQLCatalog {
		return &config.SQLCatalog{
			Host:                "db.example.com",
			User:                "readonly",
			Database:            "scripts",
			Table:               "catalog",
			IDColumn:            "id",
			SQLColumn:           "body",
			ExposurePrefix:      "cat",
			ExposureURLTemplate: "https://example.com/item/{{.ID}}",
		}
	}
	base := func(cat *config.SQLCatalog) *config.Config {
		return &config.Config{
			Manifest: config.Manifest{File: "m.json"},
			Source:   []config.SourceEntry{{Name: "pkg", Type: "mysql", MySQL: cat}},
			Sink:     config.Sink{Git: validGitSink()},
		}
	}
	if err := base(full()).Validate(); err != nil {
		t.Fatalf("full: %v", err)
	}
	if err := base(nil).Validate(); err == nil || !strings.Contains(err.Error(), "mysql is required") {
		t.Fatalf("nil: got %v", err)
	}

	cases := []struct {
		mut  func(*config.SQLCatalog)
		want string
	}{
		{func(c *config.SQLCatalog) { c.Host = "" }, "host is required"},
		{func(c *config.SQLCatalog) { c.User = "" }, "user is required"},
		{func(c *config.SQLCatalog) { c.Database = "" }, "database is required"},
		{func(c *config.SQLCatalog) { c.IDColumn = "" }, "id_column is required"},
		{func(c *config.SQLCatalog) { c.SQLColumn = "" }, "sql_column is required"},
		{func(c *config.SQLCatalog) { c.ExposurePrefix = "" }, "exposure_prefix is required"},
		{func(c *config.SQLCatalog) { c.ExposureURLTemplate = "" }, "exposure_url_template is required"},
		{func(c *config.SQLCatalog) { c.Table = ""; c.Query = "" }, "query or"},
		{func(c *config.SQLCatalog) { c.Query = "SELECT 1" }, "mutually exclusive"},
	}
	for _, tc := range cases {
		cat := full()
		tc.mut(cat)
		err := base(cat).Validate()
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s: got %v", tc.want, err)
		}
	}

	qOnly := full()
	qOnly.Table = ""
	qOnly.Query = "SELECT id, body FROM catalog"
	if err := base(qOnly).Validate(); err != nil {
		t.Fatalf("query only: %v", err)
	}
}

func TestUnitSQLCatalogSharedSchema(t *testing.T) {
	body := `
manifest:
  file: m.json
source:
  - name: cat_pkg
    type: %s
    %s:
      host: db.example.com
      port: 10000
      user: readonly
      password: ""
      database: scripts
      table: catalog
      id_column: id
      sql_column: body
      description_column: description
      extra_filters: "status = 'active'"
      exposure_prefix: cat
      exposure_url_template: "https://example.com/item/{{.ID}}"
` + validSinkYAML()
	for _, typ := range []string{"hive", "mysql"} {
		t.Run(typ, func(t *testing.T) {
			cfg, err := config.Load(writeTempYAML(t, fmt.Sprintf(body, typ, typ)))
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if err := cfg.Validate(); err != nil {
				t.Fatalf("Validate: %v", err)
			}
			var cat *config.SQLCatalog
			if typ == "hive" {
				cat = cfg.Source[0].Hive
			} else {
				cat = cfg.Source[0].MySQL
			}
			if cat == nil || cat.Database != "scripts" || cat.SQLColumn != "body" {
				t.Fatalf("%s catalog: %+v", typ, cat)
			}
		})
	}
}

func writeTempYAML(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func validGitSink() config.GitSink {
	return config.GitSink{
		RepoURL:       "https://git.example.com/org/dbt.git",
		User:          "bot",
		Token:         "tok",
		LocalPath:     "/tmp/dbt_repo",
		ExposuresPath: "dbt/models/exposures",
		Branch:        "feature/sync",
		BaseBranch:    "master",
	}
}

func validSinkYAML() string {
	return `
sink:
  git:
    repo_url: "https://git.example.com/org/dbt.git"
    user: bot
    token: tok
    local_path: /tmp/dbt_repo
    exposures_path: dbt/models/exposures
    branch: feature/sync
    base_branch: master
`
}

func validQlikYAML() string {
	return `
default_owner:
  name: analytics
  email: "@analytics"
manifest:
  file: app/manifest/testdata/manifest.mini.json
  filter_name: starrocks
parse:
  excluded_catalogs: ["main"]
source:
  - name: bi_qlik_sense
    type: qlik
    qlik:
      host: qlik.example.com
      user: "DOMAIN\\user"
      password: ""
      datasource: starrocks
      stream_exclude: ["99-Archive", "92-TMP"]
      stream_include: []
      exposure_url_template: "https://qlik.example.com/sense/app/{{.AppID}}"
` + validSinkYAML()
}

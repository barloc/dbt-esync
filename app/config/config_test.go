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
	if cfg.Source.Type != "qlik" {
		t.Fatalf("type: got %q", cfg.Source.Type)
	}
	if cfg.Source.Qlik == nil {
		t.Fatal("expected source.qlik")
	}
}

func TestUnitAcceptStreamLists(t *testing.T) {
	cfg, err := config.Load(writeTempYAML(t, `
manifest:
  file: m.json
source:
  type: qlik
  qlik:
    host: qlik.example.com
    stream_include: ["Prod", "Shared"]
    stream_exclude: ["99-Archive", "92-TMP"]
sink:
  git:
    repo_url: "https://git.example.com/org/dbt.git"
    exposures_path: dbt/models/exposures
`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	gotIn := cfg.Source.Qlik.StreamInclude
	if len(gotIn) != 2 || gotIn[0] != "Prod" || gotIn[1] != "Shared" {
		t.Fatalf("stream_include: got %v", gotIn)
	}
	gotEx := cfg.Source.Qlik.StreamExclude
	if len(gotEx) != 2 || gotEx[0] != "99-Archive" || gotEx[1] != "92-TMP" {
		t.Fatalf("stream_exclude: got %v", gotEx)
	}
}

func TestUnitValidateRejectsEmptyType(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Manifest: config.Manifest{File: "m.json"},
		Sink:     config.Sink{Git: config.GitSink{RepoURL: "https://git.example/r.git", ExposuresPath: "e"}},
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "source.type is required") {
		t.Fatalf("got %v", err)
	}
}

func TestUnitValidateRejectsUnknownType(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Manifest: config.Manifest{File: "m.json"},
		Source:   config.Source{Type: "nope"},
		Sink:     config.Sink{Git: config.GitSink{RepoURL: "https://git.example/r.git", ExposuresPath: "e"}},
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
		Source:   config.Source{Type: "qlik", Qlik: &config.QlikConfig{Host: "q"}},
		Sink:     config.Sink{Git: config.GitSink{RepoURL: "https://git.example/r.git", ExposuresPath: "e"}},
	}
	if err := both.Validate(); err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("both: got %v", err)
	}

	neither := &config.Config{
		Source: config.Source{Type: "qlik", Qlik: &config.QlikConfig{Host: "q"}},
		Sink:   config.Sink{Git: config.GitSink{RepoURL: "https://git.example/r.git", ExposuresPath: "e"}},
	}
	if err := neither.Validate(); err == nil || !strings.Contains(err.Error(), "manifest.url or manifest.file") {
		t.Fatalf("neither: got %v", err)
	}
}

func TestUnitValidateRequiresSourceBlock(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Manifest: config.Manifest{File: "m.json"},
		Source:   config.Source{Type: "qlik"},
		Sink:     config.Sink{Git: config.GitSink{RepoURL: "https://git.example/r.git", ExposuresPath: "e"}},
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "source.qlik is required") {
		t.Fatalf("got %v", err)
	}
}

func TestUnitValidateRequiresSinkGit(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Manifest: config.Manifest{File: "m.json"},
		Source:   config.Source{Type: "qlik", Qlik: &config.QlikConfig{Host: "q"}},
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "sink.git.repo_url") {
		t.Fatalf("got %v", err)
	}
}

func TestUnitValidateRequiresExposuresPath(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Manifest: config.Manifest{File: "m.json"},
		Source:   config.Source{Type: "qlik", Qlik: &config.QlikConfig{Host: "q"}},
		Sink:     config.Sink{Git: config.GitSink{RepoURL: "https://git.example/r.git"}},
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "sink.git.exposures_path") {
		t.Fatalf("got %v", err)
	}
}

func TestUnitSQLCatalogSharedSchema(t *testing.T) {
	body := `
manifest:
  file: m.json
source:
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
sink:
  git:
    repo_url: "https://git.example.com/org/dbt.git"
    exposures_path: dbt/models/exposures
`
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
				cat = cfg.Source.Hive
			} else {
				cat = cfg.Source.MySQL
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

func validQlikYAML() string {
	return `
save_path: bi_qlik_sense
default_owner:
  name: analytics
  email: "@analytics"
manifest:
  file: app/manifest/testdata/manifest.mini.json
  filter_name: starrocks
parse:
  excluded_catalogs: ["main"]
source:
  type: qlik
  qlik:
    host: qlik.example.com
    user: "DOMAIN\\user"
    password: ""
    datasource: starrocks
    stream_exclude: ["99-Archive", "92-TMP"]
    stream_include: []
    exposure_url_template: "https://qlik.example.com/sense/app/{{.AppID}}"
sink:
  git:
    repo_url: "https://git.example.com/org/dbt.git"
    exposures_path: dbt/models/exposures
`
}

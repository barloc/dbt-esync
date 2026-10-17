package hive_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/barloc/dbt-esync/app/config"
	"github.com/barloc/dbt-esync/app/manifest"
	"github.com/barloc/dbt-esync/app/parse"
	hivesrc "github.com/barloc/dbt-esync/app/source/hive"
)

func TestUnitCollectBuildsExposures(t *testing.T) {
	t.Parallel()

	db := &fakeDB{cols: []string{"id", "script", "description"}, rows: [][]any{
		{"1", "SELECT * FROM marts.orders_day", "orders script"},
		{"2", "SELECT 1", "no tables"},
	}}
	src := newSource(db, hivesrc.Options{
		Owner: config.DefaultOwner{Name: "analytics", Email: "@analytics"},
		Parse: parse.Options{ExcludedCatalogs: []string{"main"}},
	})

	files, err := src.Collect(context.Background(), loadFinder(t))
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(files) != 1 || files[0].Name != "hive" {
		t.Fatalf("files: %+v", files)
	}
	if len(files[0].Exposures) != 1 {
		t.Fatalf("exposures: %+v", files[0].Exposures)
	}
	exp := files[0].Exposures[0]
	if exp.Name != "hive_1" {
		t.Fatalf("name: %q", exp.Name)
	}
	if exp.Type != "application" || exp.Label != "hive: 1" {
		t.Fatalf("type/label: %+v", exp)
	}
	if exp.URL != "https://hive.example.com/script/1" {
		t.Fatalf("url: %q", exp.URL)
	}
	if exp.Description != "orders script" {
		t.Fatalf("description: %q", exp.Description)
	}
	if exp.OwnerName != "analytics" || exp.OwnerEmail != "@analytics" {
		t.Fatalf("owner: %+v", exp)
	}
	if len(exp.DependsOn) != 1 || exp.DependsOn[0] != "ref('orders_day')" {
		t.Fatalf("depends_on: %v", exp.DependsOn)
	}
	if !strings.Contains(db.lastQuery, "`scripts`.`hive_scripts`") {
		t.Fatalf("query: %q", db.lastQuery)
	}
}

func TestUnitCollectSkipsSQLWithoutTables(t *testing.T) {
	t.Parallel()

	db := &fakeDB{cols: []string{"id", "script", "description"}, rows: [][]any{
		{"1", "SELECT 1", ""},
	}}
	src := newSource(db, hivesrc.Options{})

	files, err := src.Collect(context.Background(), loadFinder(t))
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if files != nil {
		t.Fatalf("got %v, want nil", files)
	}
}

func TestUnitCollectNotFoundInDescription(t *testing.T) {
	t.Parallel()

	db := &fakeDB{cols: []string{"id", "script", "description"}, rows: [][]any{
		{"9", "SELECT * FROM schema_a.missing_table", "NULL"},
	}}
	src := newSource(db, hivesrc.Options{
		Owner: config.DefaultOwner{Name: "a", Email: "e"},
	})

	files, err := src.Collect(context.Background(), loadFinder(t))
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	exp := files[0].Exposures[0]
	if len(exp.DependsOn) != 0 {
		t.Fatalf("depends_on: %v", exp.DependsOn)
	}
	if !strings.Contains(exp.Description, "SQL catalog query") {
		t.Fatalf("description default: %q", exp.Description)
	}
	if !strings.Contains(exp.Description, "Tables not in dbt:") {
		t.Fatalf("description: %q", exp.Description)
	}
}

func TestUnitCollectEmptyResult(t *testing.T) {
	t.Parallel()

	db := &fakeDB{cols: []string{"id", "script", "description"}, rows: nil}
	src := newSource(db, hivesrc.Options{})

	files, err := src.Collect(context.Background(), loadFinder(t))
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if files != nil {
		t.Fatalf("got %v, want nil", files)
	}
}

func TestUnitCollectQueryError(t *testing.T) {
	t.Parallel()

	db := &fakeDB{err: errors.New("boom")}
	src := newSource(db, hivesrc.Options{})

	_, err := src.Collect(context.Background(), loadFinder(t))
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("got %v", err)
	}
}

func TestUnitCollectBadURLTemplate(t *testing.T) {
	t.Parallel()

	db := &fakeDB{cols: []string{"id", "script", "description"}, rows: [][]any{
		{"1", "SELECT * FROM marts.orders_day", "d"},
	}}
	cfg := validCatalog()
	cfg.ExposureURLTemplate = "{{.ID"
	src := hivesrc.New(cfg, hivesrc.Options{DB: db})

	_, err := src.Collect(context.Background(), loadFinder(t))
	if err == nil || !strings.Contains(err.Error(), "exposure_url_template") {
		t.Fatalf("got %v", err)
	}
}

func TestUnitCollectUsesCustomQuery(t *testing.T) {
	t.Parallel()

	db := &fakeDB{cols: []string{"id", "script"}, rows: [][]any{
		{"1", "SELECT * FROM marts.orders_day"},
	}}
	cfg := validCatalog()
	cfg.Table = ""
	cfg.Query = "SELECT id, script FROM hive_scripts WHERE active = 1"
	cfg.DescriptionColumn = ""
	src := hivesrc.New(cfg, hivesrc.Options{DB: db, Parse: parse.Options{ExcludedCatalogs: []string{"main"}}})

	files, err := src.Collect(context.Background(), loadFinder(t))
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("files: %+v", files)
	}
	if db.lastQuery != cfg.Query {
		t.Fatalf("query: %q", db.lastQuery)
	}
}

func TestUnitCollectSanitizesExposureID(t *testing.T) {
	t.Parallel()

	db := &fakeDB{cols: []string{"id", "script", "description"}, rows: [][]any{
		{"12-ab cd", "SELECT * FROM marts.orders_day", "d"},
	}}
	src := newSource(db, hivesrc.Options{
		Parse: parse.Options{ExcludedCatalogs: []string{"main"}},
	})

	files, err := src.Collect(context.Background(), loadFinder(t))
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	exp := files[0].Exposures[0]
	if exp.Name != "hive_12_ab_cd" {
		t.Fatalf("name: %q", exp.Name)
	}
	if exp.Label != "hive: 12_ab_cd" {
		t.Fatalf("label: %q", exp.Label)
	}
	// URL keeps the raw catalog id.
	if exp.URL != "https://hive.example.com/script/12-ab cd" {
		t.Fatalf("url: %q", exp.URL)
	}
}

func TestUnitCollectMissingDescriptionColumnOK(t *testing.T) {
	t.Parallel()

	// description_column configured, but custom query result omits it.
	db := &fakeDB{cols: []string{"id", "script"}, rows: [][]any{
		{"1", "SELECT * FROM marts.orders_day"},
	}}
	cfg := validCatalog()
	cfg.Table = ""
	cfg.Query = "SELECT id, script FROM hive_scripts"
	src := hivesrc.New(cfg, hivesrc.Options{
		DB:    db,
		Parse: parse.Options{ExcludedCatalogs: []string{"main"}},
	})

	files, err := src.Collect(context.Background(), loadFinder(t))
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if files[0].Exposures[0].Description != "SQL catalog query" {
		t.Fatalf("description: %q", files[0].Exposures[0].Description)
	}
}

func newSource(db hivesrc.Querier, opt hivesrc.Options) *hivesrc.Source {
	opt.DB = db
	return hivesrc.New(validCatalog(), opt)
}

func validCatalog() config.SQLCatalog {
	return config.SQLCatalog{
		Host:                "hive.example.com",
		Port:                10000,
		User:                "readonly",
		Database:            "scripts",
		Table:               "hive_scripts",
		IDColumn:            "id",
		SQLColumn:           "script",
		DescriptionColumn:   "description",
		ExposurePrefix:      "hive",
		ExposureURLTemplate: "https://hive.example.com/script/{{.ID}}",
	}
}

func loadFinder(t *testing.T) *manifest.Finder {
	t.Helper()
	path := filepath.Join("..", "..", "manifest", "testdata", "manifest.mini.json")
	f, err := manifest.Load(context.Background(), "", path, []string{"main"})
	if err != nil {
		t.Fatalf("manifest.Load: %v", err)
	}
	return f
}

type fakeDB struct {
	cols      []string
	rows      [][]any
	err       error
	lastQuery string
}

func (f *fakeDB) QueryContext(_ context.Context, query string, _ ...any) (hivesrc.RowScanner, error) {
	f.lastQuery = query
	if f.err != nil {
		return nil, f.err
	}
	return &fakeRows{cols: f.cols, rows: f.rows}, nil
}

func (f *fakeDB) Close() error { return nil }

type fakeRows struct {
	cols []string
	rows [][]any
	i    int
	err  error
}

func (r *fakeRows) Next() bool {
	if r.i >= len(r.rows) {
		return false
	}
	r.i++
	return true
}

func (r *fakeRows) Scan(dest ...any) error {
	row := r.rows[r.i-1]
	if len(dest) != len(row) {
		return errors.New("scan arity")
	}
	for i, d := range dest {
		ptr, ok := d.(*any)
		if !ok {
			return errors.New("scan dest")
		}
		*ptr = row[i]
	}
	return nil
}

func (r *fakeRows) Columns() ([]string, error) { return r.cols, nil }
func (r *fakeRows) Err() error                 { return r.err }
func (r *fakeRows) Close() error               { return nil }

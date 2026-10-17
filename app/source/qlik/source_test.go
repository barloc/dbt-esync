package qlik_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/barloc/dbt-esync/app/config"
	"github.com/barloc/dbt-esync/app/manifest"
	"github.com/barloc/dbt-esync/app/parse"
	"github.com/barloc/dbt-esync/app/source/qlik"
)

func TestUnitCollectBuildsExposures(t *testing.T) {
	t.Parallel()

	script := readQVS(t, "multi_connect.qvs")
	src := qlik.New(validCfg(), qlik.Options{
		PackageName: "bi_qlik",
		Owner:       config.DefaultOwner{Name: "analytics", Email: "@analytics"},
		Parse:       parse.Options{ExcludedCatalogs: []string{"main"}},
		Catalog: fakeCatalog{apps: []qlik.App{
			{ID: "app-1", Name: "Orders", Stream: "Prod"},
			{ID: "app-2", Name: "Archived", Stream: "99-Archive"},
		}},
		Scripts: fakeScripts{byID: map[string]string{"app-1": script, "app-2": script}},
		Tags: fakeTags{byID: map[string][]parse.Tag{
			"app-1": {{Name: "high"}, {Name: "finance"}},
		}},
	})

	files, err := src.Collect(context.Background(), loadFinder(t))
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(files) != 1 || files[0].Name != "qlik" {
		t.Fatalf("files: %+v", files)
	}
	if len(files[0].Exposures) != 1 {
		t.Fatalf("exposures: %+v", files[0].Exposures)
	}
	exp := files[0].Exposures[0]
	if exp.Name != "bi_qlik_app_1" {
		t.Fatalf("name: %q", exp.Name)
	}
	if exp.Type != "dashboard" || exp.Label != "QLIK: Orders" {
		t.Fatalf("type/label: %+v", exp)
	}
	if exp.URL != "https://qlik.example.com/sense/app/app-1" {
		t.Fatalf("url: %q", exp.URL)
	}
	if exp.Maturity != "high" {
		t.Fatalf("maturity: %q", exp.Maturity)
	}
	if len(exp.Tags) != 1 || exp.Tags[0] != "finance" {
		t.Fatalf("tags: %v", exp.Tags)
	}
	// starrocks datasource slice from multi_connect → warehouse_a.events_raw (not in manifest)
	if len(exp.DependsOn) != 0 {
		t.Fatalf("depends_on: %v", exp.DependsOn)
	}
	if !strings.Contains(exp.Description, "Tables not in dbt:") {
		t.Fatalf("description: %q", exp.Description)
	}
}

func TestUnitCollectStreamExclude(t *testing.T) {
	t.Parallel()

	if !qlik.StreamAllowedForTest("Prod", nil, []string{"99-Archive"}) {
		t.Fatal("Prod should be allowed")
	}
	if qlik.StreamAllowedForTest("99-Archive", nil, []string{"99-Archive"}) {
		t.Fatal("excluded stream")
	}
	if qlik.StreamAllowedForTest("Other", []string{"Prod"}, nil) {
		t.Fatal("not in include")
	}
	if !qlik.StreamAllowedForTest("Prod", []string{"Prod"}, []string{"99-Archive"}) {
		t.Fatal("include wins")
	}
}

func TestUnitCollectDatasourceFilter(t *testing.T) {
	t.Parallel()

	script := readQVS(t, "multi_connect.qvs")
	cfg := validCfg()
	cfg.Datasource = "starrocks"
	cfg.StreamExclude = nil
	src := qlik.New(cfg, qlik.Options{
		PackageName: "pkg",
		Parse:       parse.Options{ExcludedCatalogs: []string{"main"}},
		Catalog:     fakeCatalog{apps: []qlik.App{{ID: "a1", Name: "A", Stream: "Prod"}}},
		Scripts:     fakeScripts{byID: map[string]string{"a1": script}},
	})

	files, err := src.Collect(context.Background(), loadFinder(t))
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("files: %+v", files)
	}
}

func TestUnitCollectSkipsEmptyFilter(t *testing.T) {
	t.Parallel()

	cfg := validCfg()
	cfg.Datasource = "nosuch"
	src := qlik.New(cfg, qlik.Options{
		Parse:   parse.Options{},
		Catalog: fakeCatalog{apps: []qlik.App{{ID: "a1", Name: "A", Stream: "Prod"}}},
		Scripts: fakeScripts{byID: map[string]string{"a1": "LIB CONNECT TO 'vertica';\nSQL SELECT 1;"}},
	})

	files, err := src.Collect(context.Background(), loadFinder(t))
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if files != nil {
		t.Fatalf("got %v, want nil", files)
	}
}

func TestUnitCollectOrdersDayRef(t *testing.T) {
	t.Parallel()

	script := "LIB CONNECT TO 'starrocks';\nSQL SELECT * FROM marts.orders_day;"
	src := qlik.New(validCfg(), qlik.Options{
		PackageName: "pkg",
		Owner:       config.DefaultOwner{Name: "n", Email: "e"},
		Parse:       parse.Options{ExcludedCatalogs: []string{"main"}},
		Catalog:     fakeCatalog{apps: []qlik.App{{ID: "x", Name: "X", Stream: "Prod"}}},
		Scripts:     fakeScripts{byID: map[string]string{"x": script}},
	})

	files, err := src.Collect(context.Background(), loadFinder(t))
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	exp := files[0].Exposures[0]
	if len(exp.DependsOn) != 1 || exp.DependsOn[0] != "ref('orders_day')" {
		t.Fatalf("depends_on: %v", exp.DependsOn)
	}
}

func TestUnitCollectBadURLTemplate(t *testing.T) {
	t.Parallel()

	cfg := validCfg()
	cfg.ExposureURLTemplate = "{{.AppID"
	src := qlik.New(cfg, qlik.Options{
		Catalog: fakeCatalog{apps: []qlik.App{{ID: "x", Name: "X", Stream: "Prod"}}},
		Scripts: fakeScripts{byID: map[string]string{"x": "LIB CONNECT TO 'starrocks';\nSQL SELECT * FROM marts.orders_day;"}},
		Parse:   parse.Options{ExcludedCatalogs: []string{"main"}},
	})
	_, err := src.Collect(context.Background(), loadFinder(t))
	if err == nil || !strings.Contains(err.Error(), "exposure_url_template") {
		t.Fatalf("got %v", err)
	}
}

func TestUnitDefaultClientRequiresQRC(t *testing.T) {
	t.Parallel()

	src := qlik.New(validCfg(), qlik.Options{})
	_, err := src.Collect(context.Background(), loadFinder(t))
	if err == nil || !strings.Contains(err.Error(), "qrc.host") {
		t.Fatalf("got %v", err)
	}
}

func validCfg() config.QlikConfig {
	return config.QlikConfig{
		Host:                "qlik.example.com",
		User:                `DOMAIN\user`,
		Datasource:          "starrocks",
		StreamExclude:       []string{"99-Archive"},
		ExposureURLTemplate: "https://qlik.example.com/sense/app/{{.AppID}}",
	}
}

func readQVS(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("..", "..", "parse", "testdata", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
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

type fakeCatalog struct{ apps []qlik.App }

func (f fakeCatalog) ListApps(context.Context) ([]qlik.App, error) { return f.apps, nil }

type fakeScripts struct{ byID map[string]string }

func (f fakeScripts) LoadScript(_ context.Context, appID string) (string, error) {
	return f.byID[appID], nil
}

type fakeTags struct{ byID map[string][]parse.Tag }

func (f fakeTags) AppTags(_ context.Context, appID string) ([]parse.Tag, error) {
	return f.byID[appID], nil
}

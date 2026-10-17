package growthbook_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/barloc/dbt-esync/app/config"
	"github.com/barloc/dbt-esync/app/manifest"
	"github.com/barloc/dbt-esync/app/parse"
	"github.com/barloc/dbt-esync/app/source/growthbook"
)

func TestUnitCollectBuildsExposures(t *testing.T) {
	t.Parallel()

	srv := fixtureServer(t, "datasources_mixed.json", "Bearer test-key")
	src := newSource(srv.URL, "test-key", growthbook.Options{
		Owner: config.DefaultOwner{Name: "analytics", Email: "@analytics"},
		Parse: parse.Options{ExcludedCatalogs: []string{"main"}},
	})

	files, err := src.Collect(context.Background(), loadFinder(t))
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(files) != 1 || files[0].Name != "growthbook" {
		t.Fatalf("files: %+v", files)
	}
	if len(files[0].Exposures) != 1 {
		t.Fatalf("exposures: %+v", files[0].Exposures)
	}
	exp := files[0].Exposures[0]
	if exp.Name != "growthbook_ds_sr_q1" {
		t.Fatalf("name: %q", exp.Name)
	}
	if exp.Type != "application" || exp.Label != "GROWTHBOOK: q1" {
		t.Fatalf("type/label: %+v", exp)
	}
	if exp.URL != "https://growthbook.example.com/datasources/ds_sr" {
		t.Fatalf("url: %q", exp.URL)
	}
	if exp.Description != "orders query" {
		t.Fatalf("description: %q", exp.Description)
	}
	if exp.OwnerName != "analytics" || exp.OwnerEmail != "@analytics" {
		t.Fatalf("owner: %+v", exp)
	}
	if len(exp.DependsOn) != 1 || exp.DependsOn[0] != "ref('orders_day')" {
		t.Fatalf("depends_on: %v", exp.DependsOn)
	}
}

func TestUnitCollectSkipsWrongDatasourceType(t *testing.T) {
	t.Parallel()

	srv := fixtureServer(t, "datasources_wrong_type.json", "")
	src := newSource(srv.URL, "k", growthbook.Options{})

	files, err := src.Collect(context.Background(), loadFinder(t))
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if files != nil {
		t.Fatalf("got %v, want nil", files)
	}
}

func TestUnitCollectSkipsSQLWithoutTables(t *testing.T) {
	t.Parallel()

	srv := fixtureServer(t, "datasources_no_tables.json", "")
	src := newSource(srv.URL, "k", growthbook.Options{})

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

	srv := fixtureServer(t, "datasources_not_found.json", "")
	src := newSource(srv.URL, "k", growthbook.Options{
		Owner: config.DefaultOwner{Name: "a", Email: "e"},
	})

	files, err := src.Collect(context.Background(), loadFinder(t))
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	exp := files[0].Exposures[0]
	if !strings.Contains(exp.Description, "Tables not in dbt: schema_a.missing_table") {
		t.Fatalf("description: %q", exp.Description)
	}
	if !strings.Contains(exp.Description, "GrowthBook assignment query") {
		t.Fatalf("expected default description, got %q", exp.Description)
	}
	if len(exp.DependsOn) != 0 {
		t.Fatalf("depends_on: %v", exp.DependsOn)
	}
}

func TestUnitCollectEmptyResult(t *testing.T) {
	t.Parallel()

	srv := fixtureServer(t, "datasources_empty.json", "")
	src := newSource(srv.URL, "k", growthbook.Options{})

	files, err := src.Collect(context.Background(), loadFinder(t))
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if files != nil {
		t.Fatalf("got %v, want nil", files)
	}
}

func TestUnitCollectPaginates(t *testing.T) {
	t.Parallel()

	page0 := readFixture(t, "datasources_page0.json")
	page1 := readFixture(t, "datasources_page1.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("offset") {
		case "0":
			_, _ = w.Write(page0)
		case "1":
			_, _ = w.Write(page1)
		default:
			_, _ = w.Write([]byte(`{"count":2,"dataSources":[]}`))
		}
	}))
	t.Cleanup(srv.Close)

	src := newSource(srv.URL, "k", growthbook.Options{
		Owner: config.DefaultOwner{Name: "a", Email: "e"},
	})

	files, err := src.Collect(context.Background(), loadFinder(t))
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(files[0].Exposures) != 2 {
		t.Fatalf("exposures: %+v", files[0].Exposures)
	}
}

func TestUnitCollectHTTPError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)

	src := newSource(srv.URL, "k", growthbook.Options{})
	_, err := src.Collect(context.Background(), loadFinder(t))
	if err == nil || !strings.Contains(err.Error(), "status 403") {
		t.Fatalf("got %v", err)
	}
}

func TestUnitCollectBadJSON(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("{not-json"))
	}))
	t.Cleanup(srv.Close)

	src := newSource(srv.URL, "k", growthbook.Options{})
	_, err := src.Collect(context.Background(), loadFinder(t))
	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("got %v", err)
	}
}

func newSource(host, apiKey string, opt growthbook.Options) *growthbook.Source {
	return growthbook.New(config.GrowthBookConfig{
		Host:         host,
		APIKey:       apiKey,
		FilterSource: "starrocks",
		ExposureURL:  "https://growthbook.example.com/datasources",
	}, opt)
}

func fixtureServer(t *testing.T, name, wantAuth string) *httptest.Server {
	t.Helper()
	body := readFixture(t, name)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if wantAuth != "" && r.Header.Get("Authorization") != wantAuth {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return raw
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

package superset_test

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
	"github.com/barloc/dbt-esync/app/source/superset"
)

func TestUnitCollectLoginAuthBuildsDashboard(t *testing.T) {
	t.Parallel()

	var sawLogin bool
	srv := fixtureServer(t, "Bearer jwt-from-login", func(r *http.Request) {
		if r.URL.Path == "/api/v1/security/login" {
			sawLogin = true
		}
	}, nil)
	src := newSource(srv.URL, config.SupersetConfig{
		Username:             "bot",
		Password:             "secret",
		DatabaseNameContains: "starrocks",
		HostToDisplay:        "https://superset.example.com",
	}, superset.Options{
		PackageName: "bi_superset",
		Owner:       config.DefaultOwner{Name: "analytics", Email: "@analytics"},
		Parse:       parse.Options{ExcludedCatalogs: []string{"main"}},
	})

	files, err := src.Collect(context.Background(), loadFinder(t))
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if !sawLogin {
		t.Fatal("expected login call")
	}
	if len(files) != 1 || files[0].Name != "superset" {
		t.Fatalf("files: %+v", files)
	}
	if len(files[0].Exposures) != 1 {
		t.Fatalf("exposures: %+v", files[0].Exposures)
	}
	exp := files[0].Exposures[0]
	if exp.Name != "bi_superset_dashboard_10" {
		t.Fatalf("name: %q", exp.Name)
	}
	if exp.Type != "dashboard" || exp.Label != "SUPERSET: Orders Dash" {
		t.Fatalf("type/label: %+v", exp)
	}
	if exp.URL != "https://superset.example.com/superset/dashboard/10/" {
		t.Fatalf("url: %q", exp.URL)
	}
	if !strings.Contains(exp.Description, "* [orders_virt](https://superset.example.com/explore/?datasource_type=table&datasource_id=53)") {
		t.Fatalf("description: %q", exp.Description)
	}
	if exp.OwnerName != "analytics" || exp.OwnerEmail != "@analytics" {
		t.Fatalf("owner (default): %+v", exp)
	}
	if len(exp.DependsOn) != 1 || exp.DependsOn[0] != "ref('orders_day')" {
		t.Fatalf("depends_on: %v", exp.DependsOn)
	}
}

func TestUnitCollectAPIKeyAuthSkipsLogin(t *testing.T) {
	t.Parallel()

	var sawLogin bool
	srv := fixtureServer(t, "Bearer sst_test", func(r *http.Request) {
		if r.URL.Path == "/api/v1/security/login" {
			sawLogin = true
		}
	}, nil)
	src := newSource(srv.URL, config.SupersetConfig{
		APIKey:               "sst_test",
		DatabaseNameContains: "starrocks",
		HostToDisplay:        "https://superset.example.com",
	}, superset.Options{
		PackageName: "bi_superset",
		Owner:       config.DefaultOwner{Name: "analytics", Email: "@analytics"},
	})

	files, err := src.Collect(context.Background(), loadFinder(t))
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if sawLogin {
		t.Fatal("api_key mode must not call /security/login")
	}
	if len(files) != 1 || len(files[0].Exposures) != 1 {
		t.Fatalf("files: %+v", files)
	}
}

func TestUnitCollectUseAPIOwner(t *testing.T) {
	t.Parallel()

	srv := fixtureServer(t, "", nil, nil)
	src := newSource(srv.URL, config.SupersetConfig{
		APIKey:               "k",
		DatabaseNameContains: "starrocks",
		HostToDisplay:        "https://superset.example.com",
		UseAPIOwner:          true,
	}, superset.Options{
		PackageName: "bi_superset",
		Owner:       config.DefaultOwner{Name: "analytics", Email: "@analytics"},
	})

	files, err := src.Collect(context.Background(), loadFinder(t))
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	exp := files[0].Exposures[0]
	if exp.OwnerName != "Alice Admin" || exp.OwnerEmail != "alice@example.com" {
		t.Fatalf("owner: %+v", exp)
	}
}

func TestUnitCollectFiltersByDatabaseName(t *testing.T) {
	t.Parallel()

	srv := fixtureServer(t, "", nil, nil)
	src := newSource(srv.URL, config.SupersetConfig{
		APIKey:               "k",
		DatabaseNameContains: "starrocks",
		HostToDisplay:        "https://superset.example.com",
	}, superset.Options{PackageName: "bi_superset"})

	files, err := src.Collect(context.Background(), loadFinder(t))
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	// dashboard 11 only has postgres datasets → skipped
	if len(files) != 1 || len(files[0].Exposures) != 1 || files[0].Exposures[0].Name != "bi_superset_dashboard_10" {
		t.Fatalf("files: %+v", files)
	}
}

func TestUnitCollectPhysicalDataset(t *testing.T) {
	t.Parallel()

	overrides := map[string][]byte{
		"/api/v1/dashboard/": readFixture(t, "dashboards.json"),
		"/api/v1/dashboard/10/datasets": []byte(`{
  "result": [{
    "id": 54,
    "name": "orders_phys",
    "table_name": "orders_day",
    "schema": "marts",
    "sql": "",
    "database": {"id": 1, "name": "starrocks_prod"}
  }]
}`),
		"/api/v1/dashboard/11/datasets": readFixture(t, "dashboard_11_datasets.json"),
	}
	// rewrite dashboards list to only id 10 for a cleaner assert — keep both, 11 filtered by db
	srv := fixtureServer(t, "", nil, overrides)
	src := newSource(srv.URL, config.SupersetConfig{
		APIKey:               "k",
		DatabaseNameContains: "starrocks",
		HostToDisplay:        "https://superset.example.com",
	}, superset.Options{
		PackageName: "bi_superset",
		Parse:       parse.Options{ExcludedCatalogs: []string{"main"}},
	})

	files, err := src.Collect(context.Background(), loadFinder(t))
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(files) != 1 || len(files[0].Exposures) != 1 {
		t.Fatalf("files: %+v", files)
	}
	if len(files[0].Exposures[0].DependsOn) != 1 || files[0].Exposures[0].DependsOn[0] != "ref('orders_day')" {
		t.Fatalf("depends_on: %v", files[0].Exposures[0].DependsOn)
	}
}

func TestUnitCollectSkipsPhysicalWithoutSchema(t *testing.T) {
	t.Parallel()

	overrides := map[string][]byte{
		"/api/v1/dashboard/10/datasets": []byte(`{
  "result": [{
    "id": 80,
    "name": "bare_table",
    "table_name": "orders_day",
    "schema": "",
    "sql": "",
    "database": {"id": 1, "name": "starrocks_prod"}
  }]
}`),
		"/api/v1/dashboard/11/datasets": readFixture(t, "dashboard_11_datasets.json"),
	}
	srv := fixtureServer(t, "", nil, overrides)
	src := newSource(srv.URL, config.SupersetConfig{
		APIKey:               "k",
		DatabaseNameContains: "starrocks",
	}, superset.Options{PackageName: "bi_superset"})

	files, err := src.Collect(context.Background(), loadFinder(t))
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if files != nil {
		t.Fatalf("want nil (unparsable physical), got %+v", files)
	}
}

func TestUnitCollectEmpty(t *testing.T) {
	t.Parallel()

	srv := fixtureServer(t, "", nil, map[string][]byte{
		"/api/v1/dashboard/": readFixture(t, "dashboards_empty.json"),
	})
	src := newSource(srv.URL, config.SupersetConfig{
		APIKey:               "k",
		DatabaseNameContains: "starrocks",
	}, superset.Options{PackageName: "bi_superset"})

	files, err := src.Collect(context.Background(), loadFinder(t))
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if files != nil {
		t.Fatalf("want nil, got %+v", files)
	}
}

func TestUnitCollectHTTPError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	src := newSource(srv.URL, config.SupersetConfig{
		APIKey:               "bad",
		DatabaseNameContains: "starrocks",
	}, superset.Options{PackageName: "bi_superset"})

	_, err := src.Collect(context.Background(), loadFinder(t))
	if err == nil || !strings.Contains(err.Error(), "status 401") {
		t.Fatalf("got %v", err)
	}
}

func TestUnitCollectLoginHTTPError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/security/login" {
			http.Error(w, "nope", http.StatusUnauthorized)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	src := newSource(srv.URL, config.SupersetConfig{
		Username:             "bot",
		Password:             "bad",
		DatabaseNameContains: "starrocks",
	}, superset.Options{PackageName: "bi_superset"})

	_, err := src.Collect(context.Background(), loadFinder(t))
	if err == nil || !strings.Contains(err.Error(), "login status 401") {
		t.Fatalf("got %v", err)
	}
}

func newSource(host string, cfg config.SupersetConfig, opt superset.Options) *superset.Source {
	cfg.Host = host
	return superset.New(cfg, opt)
}

func fixtureServer(t *testing.T, wantAuth string, onRequest func(*http.Request), overrides map[string][]byte) *httptest.Server {
	t.Helper()
	routes := map[string][]byte{
		"/api/v1/security/login":        readFixture(t, "login.json"),
		"/api/v1/database/":             readFixture(t, "databases.json"),
		"/api/v1/dashboard/":            readFixture(t, "dashboards.json"),
		"/api/v1/dashboard/10/datasets": readFixture(t, "dashboard_10_datasets.json"),
		"/api/v1/dashboard/11/datasets": readFixture(t, "dashboard_11_datasets.json"),
	}
	for k, v := range overrides {
		routes[k] = v
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if onRequest != nil {
			onRequest(r)
		}
		path := r.URL.Path
		if path == "/api/v1/security/login" {
			if r.Method != http.MethodPost {
				http.Error(w, "method", http.StatusMethodNotAllowed)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(routes[path])
			return
		}
		if wantAuth != "" && r.Header.Get("Authorization") != wantAuth {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		body, ok := routes[path]
		if !ok {
			http.NotFound(w, r)
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

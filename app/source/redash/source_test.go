package redash_test

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
	"github.com/barloc/dbt-esync/app/source/redash"
)

func TestUnitCollectBuildsDashboardAndAlert(t *testing.T) {
	t.Parallel()

	srv := fixtureServer(t, "Key test-key", nil)
	src := newSource(srv.URL, config.RedashConfig{
		APIKey:                 "test-key",
		DatasourceNameContains: "starrocks",
		EntityTypes:            []string{"dashboard", "alerts"},
		HostToDisplay:          "https://redash.example.com",
	}, redash.Options{
		PackageName: "bi_redash",
		Owner:       config.DefaultOwner{Name: "analytics", Email: "@analytics"},
		Parse:       parse.Options{ExcludedCatalogs: []string{"main"}},
	})

	files, err := src.Collect(context.Background(), loadFinder(t))
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("files: %+v", files)
	}
	if files[0].Name != "redash_dashboard" || files[1].Name != "redash_alerts" {
		t.Fatalf("file names: %q %q", files[0].Name, files[1].Name)
	}
	if len(files[0].Exposures) != 1 {
		t.Fatalf("dash exposures: %+v", files[0].Exposures)
	}
	dash := files[0].Exposures[0]
	if dash.Name != "bi_redash_dashboard_10" {
		t.Fatalf("dash name: %q", dash.Name)
	}
	if dash.Type != "dashboard" || dash.Label != "REDASH: Orders Dash" {
		t.Fatalf("dash type/label: %+v", dash)
	}
	if dash.URL != "https://redash.example.com/dashboards/10" {
		t.Fatalf("dash url: %q", dash.URL)
	}
	if !strings.Contains(dash.Description, "* [orders_q](https://redash.example.com/queries/53)") {
		t.Fatalf("dash description: %q", dash.Description)
	}
	if !strings.Contains(dash.Description, "* [orders_q2](https://redash.example.com/queries/54)") {
		t.Fatalf("dash description missing q2: %q", dash.Description)
	}
	if dash.OwnerName != "analytics" || dash.OwnerEmail != "@analytics" {
		t.Fatalf("dash owner (default): %+v", dash)
	}
	if len(dash.DependsOn) != 1 || dash.DependsOn[0] != "ref('orders_day')" {
		t.Fatalf("dash depends_on: %v", dash.DependsOn)
	}

	if len(files[1].Exposures) != 1 {
		t.Fatalf("alert exposures: %+v", files[1].Exposures)
	}
	alert := files[1].Exposures[0]
	if alert.Name != "bi_redash_alert_7" {
		t.Fatalf("alert name: %q", alert.Name)
	}
	if alert.Label != "REDASH ALERTS: Orders Alert" {
		t.Fatalf("alert label: %q", alert.Label)
	}
	if alert.URL != "https://redash.example.com/alerts/7" {
		t.Fatalf("alert url: %q", alert.URL)
	}
	if alert.OwnerName != "analytics" {
		t.Fatalf("alert owner: %+v", alert)
	}
}

func TestUnitCollectUseAPIOwner(t *testing.T) {
	t.Parallel()

	srv := fixtureServer(t, "", nil)
	src := newSource(srv.URL, config.RedashConfig{
		APIKey:                 "k",
		DatasourceNameContains: "starrocks",
		EntityTypes:            []string{"dashboard"},
		HostToDisplay:          "https://redash.example.com",
		UseAPIOwner:            true,
	}, redash.Options{
		PackageName: "bi_redash",
		Owner:       config.DefaultOwner{Name: "analytics", Email: "@analytics"},
	})

	files, err := src.Collect(context.Background(), loadFinder(t))
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	exp := files[0].Exposures[0]
	if exp.OwnerName != "Redash Alice" || exp.OwnerEmail != "alice@example.com" {
		t.Fatalf("owner: %+v", exp)
	}
}

func TestUnitCollectUseAPIOwnerPartialFallback(t *testing.T) {
	t.Parallel()

	overrides := map[string][]byte{
		"/api/dashboards/10": []byte(`{
			"id": 10,
			"name": "Orders Dash",
			"user": {"name": "Only Name", "email": ""},
			"widgets": [{"visualization": {"query": {"id": 53, "name": "orders_q", "data_source_id": 1}}}]
		}`),
	}
	srv := fixtureServer(t, "", overrides)
	src := newSource(srv.URL, config.RedashConfig{
		APIKey:                 "k",
		DatasourceNameContains: "starrocks",
		EntityTypes:            []string{"dashboard"},
		HostToDisplay:          "https://redash.example.com",
		UseAPIOwner:            true,
	}, redash.Options{
		PackageName: "bi_redash",
		Owner:       config.DefaultOwner{Name: "analytics", Email: "@analytics"},
	})

	files, err := src.Collect(context.Background(), loadFinder(t))
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	exp := files[0].Exposures[0]
	if exp.OwnerName != "Only Name" || exp.OwnerEmail != "@analytics" {
		t.Fatalf("owner: %+v", exp)
	}
}

func TestUnitCollectDedupsDashboardQuerySQL(t *testing.T) {
	t.Parallel()

	overrides := map[string][]byte{
		"/api/dashboards": []byte(`{"results":[{"id":10,"name":"Dup"}]}`),
		"/api/dashboards/10": []byte(`{
			"id": 10,
			"name": "Dup",
			"user": {"name": "A", "email": "a@e"},
			"widgets": [
				{"visualization": {"query": {"id": 53, "name": "orders_q", "data_source_id": 1}}},
				{"visualization": {"query": {"id": 53, "name": "orders_q", "data_source_id": 1}}}
			]
		}`),
		"/api/alerts": []byte(`[]`),
	}
	srv := fixtureServer(t, "", overrides)
	src := newSource(srv.URL, config.RedashConfig{
		APIKey:                 "k",
		DatasourceNameContains: "starrocks",
		EntityTypes:            []string{"dashboard"},
		HostToDisplay:          "https://redash.example.com",
	}, redash.Options{PackageName: "bi_redash"})

	files, err := src.Collect(context.Background(), loadFinder(t))
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	desc := files[0].Exposures[0].Description
	if strings.Count(desc, "orders_q") != 1 {
		t.Fatalf("expected one link, got %q", desc)
	}
}

func TestUnitCollectFiltersDatasource(t *testing.T) {
	t.Parallel()

	srv := fixtureServer(t, "", nil)
	src := newSource(srv.URL, config.RedashConfig{
		APIKey:                 "k",
		DatasourceNameContains: "starrocks",
		EntityTypes:            []string{"dashboard"},
		HostToDisplay:          "https://redash.example.com",
	}, redash.Options{PackageName: "bi_redash"})

	files, err := src.Collect(context.Background(), loadFinder(t))
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(files) != 1 || len(files[0].Exposures) != 1 || files[0].Exposures[0].Name != "bi_redash_dashboard_10" {
		t.Fatalf("got %+v", files)
	}
}

func TestUnitCollectEntityTypesAlertsOnly(t *testing.T) {
	t.Parallel()

	srv := fixtureServer(t, "", nil)
	src := newSource(srv.URL, config.RedashConfig{
		APIKey:                 "k",
		DatasourceNameContains: "starrocks",
		EntityTypes:            []string{"alerts"},
		HostToDisplay:          "https://redash.example.com",
	}, redash.Options{PackageName: "bi_redash"})

	files, err := src.Collect(context.Background(), loadFinder(t))
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(files) != 1 || files[0].Name != "redash_alerts" {
		t.Fatalf("files: %+v", files)
	}
	if len(files[0].Exposures) != 1 || files[0].Exposures[0].Name != "bi_redash_alert_7" {
		t.Fatalf("exposures: %+v", files[0].Exposures)
	}
}

func TestUnitCollectSkipsSQLWithoutTables(t *testing.T) {
	t.Parallel()

	overrides := map[string][]byte{
		"/api/dashboards":    []byte(`{"results":[{"id":20,"name":"No Tables"}]}`),
		"/api/dashboards/20": []byte(`{
			"id": 20,
			"name": "No Tables",
			"user": {"name": "A", "email": "a@e"},
			"widgets": [{"visualization": {"query": {"id": 54, "name": "lit", "data_source_id": 1}}}]
		}`),
		"/api/alerts": []byte(`[]`),
	}
	srv := fixtureServer(t, "", overrides)
	src := newSource(srv.URL, config.RedashConfig{
		APIKey:                 "k",
		DatasourceNameContains: "starrocks",
		EntityTypes:            []string{"dashboard"},
		HostToDisplay:          "https://redash.example.com",
	}, redash.Options{PackageName: "bi_redash"})

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

	overrides := map[string][]byte{
		"/api/alerts": readFixture(t, "alerts_not_found.json"),
	}
	srv := fixtureServer(t, "", overrides)
	src := newSource(srv.URL, config.RedashConfig{
		APIKey:                 "k",
		DatasourceNameContains: "starrocks",
		EntityTypes:            []string{"alerts"},
		HostToDisplay:          "https://redash.example.com",
	}, redash.Options{
		PackageName: "bi_redash",
		Owner:       config.DefaultOwner{Name: "a", Email: "e"},
	})

	files, err := src.Collect(context.Background(), loadFinder(t))
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	exp := files[0].Exposures[0]
	if !strings.Contains(exp.Description, "Tables not in dbt: schema_a.missing_table") {
		t.Fatalf("description: %q", exp.Description)
	}
}

func TestUnitCollectHTTPError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)

	src := newSource(srv.URL, config.RedashConfig{
		APIKey:                 "k",
		DatasourceNameContains: "starrocks",
	}, redash.Options{PackageName: "bi_redash"})
	_, err := src.Collect(context.Background(), loadFinder(t))
	if err == nil || !strings.Contains(err.Error(), "status 403") {
		t.Fatalf("got %v", err)
	}
}

func TestUnitCollectAuthHeader(t *testing.T) {
	t.Parallel()

	srv := fixtureServer(t, "Key secret", nil)
	src := newSource(srv.URL, config.RedashConfig{
		APIKey:                 "secret",
		DatasourceNameContains: "starrocks",
		EntityTypes:            []string{"alerts"},
		HostToDisplay:          "https://redash.example.com",
	}, redash.Options{PackageName: "bi_redash"})

	_, err := src.Collect(context.Background(), loadFinder(t))
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
}

func newSource(host string, cfg config.RedashConfig, opt redash.Options) *redash.Source {
	cfg.Host = host
	return redash.New(cfg, opt)
}

func fixtureServer(t *testing.T, wantAuth string, overrides map[string][]byte) *httptest.Server {
	t.Helper()
	routes := map[string][]byte{
		"/api/data_sources":  readFixture(t, "data_sources.json"),
		"/api/dashboards":    readFixture(t, "dashboards_page1.json"),
		"/api/dashboards/10": readFixture(t, "dashboard_10.json"),
		"/api/dashboards/11": readFixture(t, "dashboard_11.json"),
		"/api/queries/53":    readFixture(t, "query_53.json"),
		"/api/queries/54":    readFixture(t, "query_54.json"),
		"/api/queries/70":    readFixture(t, "query_70.json"),
		"/api/queries/71":    readFixture(t, "query_71.json"),
		"/api/queries/99":    readFixture(t, "query_99.json"),
		"/api/alerts":        readFixture(t, "alerts.json"),
	}
	for k, v := range overrides {
		routes[k] = v
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if wantAuth != "" && r.Header.Get("Authorization") != wantAuth {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		path := r.URL.Path
		// list dashboards carries query string; match path only
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

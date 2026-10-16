package manifest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestUnitRefsFromMiniManifest(t *testing.T) {
	f := loadMini(t, []string{"main"})
	got := f.Refs([]string{
		"marts.orders_day",
		"marketing.googleads_devices",
		"lakehouse.schema_a.events_raw",
		"schema_b.clicks",
		"unknown.table",
		"",
	})
	want := Lookup{
		Found: []string{
			"ref('events_raw')",
			"ref('orders_day')",
			"source('raw', 'clicks')",
			"source('team_marketing', 'googleads_devices')",
		},
		NotFound: []string{"unknown.table"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestUnitRefsThreePartExact(t *testing.T) {
	f := &Finder{mapping: map[string]string{
		"project.marts.help_center_quiz": "ref('help_center_quiz')",
	}}
	got := f.Refs([]string{"project.marts.help_center_quiz"})
	want := Lookup{Found: []string{"ref('help_center_quiz')"}, NotFound: []string{}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestUnitRefsThreePartFallbackTwoPart(t *testing.T) {
	f := &Finder{mapping: map[string]string{
		"marts.help_center_quiz": "ref('help_center_quiz')",
	}}
	got := f.Refs([]string{"project.marts.help_center_quiz"})
	want := Lookup{Found: []string{"ref('help_center_quiz')"}, NotFound: []string{}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestUnitRefsTwoPartDualKey(t *testing.T) {
	f := &Finder{mapping: map[string]string{
		"project.marts.help_center_quiz": "ref('help_center_quiz')",
		"marts.help_center_quiz":         "ref('help_center_quiz')",
	}}
	got := f.Refs([]string{"marts.help_center_quiz"})
	want := Lookup{Found: []string{"ref('help_center_quiz')"}, NotFound: []string{}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestUnitRefsTwoPartNotFound(t *testing.T) {
	f := &Finder{mapping: map[string]string{}}
	got := f.Refs([]string{"marts.unknown_table"})
	want := Lookup{Found: []string{}, NotFound: []string{"marts.unknown_table"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestUnitExcludedCatalogTwoPartOnly(t *testing.T) {
	f := loadMini(t, []string{"main"})
	// main.marts.orders_day must not be a key; marts.orders_day must.
	if _, ok := f.mapping["main.marts.orders_day"]; ok {
		t.Fatal("expected no 3-part key for excluded catalog main")
	}
	if f.mapping["marts.orders_day"] != "ref('orders_day')" {
		t.Fatalf("marts.orders_day = %q", f.mapping["marts.orders_day"])
	}
	// lakehouse is not excluded → dual keys
	if f.mapping["lakehouse.schema_a.events_raw"] != "ref('events_raw')" {
		t.Fatalf("3-part lakehouse key missing: %v", f.mapping)
	}
	if f.mapping["schema_a.events_raw"] != "ref('events_raw')" {
		t.Fatalf("2-part lakehouse short key missing: %v", f.mapping)
	}
}

func TestUnitSourcesWinShortKeyCollision(t *testing.T) {
	raw := []byte(`{
  "nodes": {
    "model.demo.shared_tbl": {
      "resource_type": "model",
      "name": "shared_tbl_model",
      "database": "lakehouse",
      "alias": "shared_tbl",
      "config": {"schema": "schema_a", "alias": "shared_tbl"}
    }
  },
  "sources": {
    "source.demo.raw.shared_tbl": {
      "source_name": "raw",
      "name": "shared_tbl",
      "database": "lakehouse",
      "schema": "schema_a"
    }
  }
}`)
	dir := t.TempDir()
	path := filepath.Join(dir, "collision.json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Load(context.Background(), "", path, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := "source('raw', 'shared_tbl')"
	if f.mapping["schema_a.shared_tbl"] != want {
		t.Fatalf("short key = %q want %q", f.mapping["schema_a.shared_tbl"], want)
	}
	if f.mapping["lakehouse.schema_a.shared_tbl"] != want {
		t.Fatalf("full key = %q want %q", f.mapping["lakehouse.schema_a.shared_tbl"], want)
	}
	got := f.Refs([]string{"schema_a.shared_tbl"})
	if len(got.Found) != 1 || got.Found[0] != want {
		t.Fatalf("Refs = %+v", got)
	}
}

func TestUnitCaseInsensitiveLookup(t *testing.T) {
	f := loadMini(t, []string{"main"})
	got := f.Refs([]string{"Marts.Orders_Day"})
	want := Lookup{Found: []string{"ref('orders_day')"}, NotFound: []string{}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestUnitLoadFromFile(t *testing.T) {
	f, err := Load(context.Background(), "", filepath.Join("testdata", "manifest.mini.json"), []string{"main"})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := f.Refs([]string{"marts.orders_day"})
	if len(got.Found) != 1 || got.Found[0] != "ref('orders_day')" {
		t.Fatalf("got %+v", got)
	}
}

func TestUnitLoadFromURL(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "manifest.mini.json"))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
	}))
	t.Cleanup(srv.Close)

	f, err := Load(context.Background(), srv.URL, "", []string{"main"})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := f.Refs([]string{"marketing.googleads_devices"})
	if len(got.Found) != 1 || got.Found[0] != "source('team_marketing', 'googleads_devices')" {
		t.Fatalf("got %+v", got)
	}
}

func TestUnitLoadRequiresURLOrFile(t *testing.T) {
	_, err := Load(context.Background(), "", "", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	_, err = Load(context.Background(), "http://example", "file.json", nil)
	if err == nil {
		t.Fatal("expected mutual exclusion error")
	}
}

func TestUnitLoadURLBadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	_, err := Load(context.Background(), srv.URL, "", nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func loadMini(t *testing.T, excluded []string) *Finder {
	t.Helper()
	path := filepath.Join("testdata", "manifest.mini.json")
	f, err := Load(context.Background(), "", path, excluded)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return f
}

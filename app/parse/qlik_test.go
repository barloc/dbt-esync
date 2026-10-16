package parse

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func readTestdata(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestUnitFilterQlikMultiConnect(t *testing.T) {
	body := readTestdata(t, "multi_connect.qvs")

	vertica := FilterQlikScript(body, "vertica")
	gotV := Tables(vertica, Options{})
	wantV := []string{"marts.billing_day", "marts.users_dim"}
	if !reflect.DeepEqual(gotV, wantV) {
		t.Fatalf("vertica: got %v want %v\nfiltered:\n%s", gotV, wantV, vertica)
	}

	sr := FilterQlikScript(body, "starrocks")
	gotSR := Tables(sr, Options{})
	wantSR := []string{"warehouse_a.events_raw"}
	if !reflect.DeepEqual(gotSR, wantSR) {
		t.Fatalf("starrocks: got %v want %v\nfiltered:\n%s", gotSR, wantSR, sr)
	}
}

func TestUnitFilterQlikCommentedConnect(t *testing.T) {
	body := readTestdata(t, "commented_connect.qvs")
	filtered := FilterQlikScript(body, "starrocks")
	if strings.Contains(strings.ToLower(filtered), "should_skip") {
		t.Fatalf("commented section leaked:\n%s", filtered)
	}
	got := Tables(filtered, Options{})
	want := []string{"schema_alpha.metrics_day"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}

	empty := FilterQlikScript(body, "vertica")
	if empty != "" {
		t.Fatalf("expected empty for vertica (commented only), got %q", empty)
	}
}

func TestUnitFilterQlikSubCallConnect(t *testing.T) {
	body := readTestdata(t, "sub_call_connect.qvs")
	filtered := FilterQlikScript(body, "starrocks")
	got := Tables(filtered, Options{ExcludedCatalogs: []string{"main"}})
	want := []string{"analytic_demo.sample_table", "marts.orders_fact"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v\nfiltered:\n%s", got, want, filtered)
	}
}

func TestUnitFilterQlikSubstringDatasource(t *testing.T) {
	body := `
LIB CONNECT TO 'starrocks_warehouse';
SQL SELECT * FROM marts.billing_day;
`
	filtered := FilterQlikScript(body, "starrocks")
	got := Tables(filtered, Options{})
	want := []string{"marts.billing_day"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestUnitFilterQlikCaseInsensitiveConnect(t *testing.T) {
	body := `
LIB Connect To 'StarRocks';
SQL SELECT * FROM core.events_log;
`
	filtered := FilterQlikScript(body, "starrocks")
	got := Tables(filtered, Options{})
	want := []string{"core.events_log"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

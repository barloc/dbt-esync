package parse

import (
	"reflect"
	"testing"
)

func TestUnitTablesBasicSQL(t *testing.T) {
	opts := Options{ExcludedCatalogs: []string{"main"}}
	got := Tables(`select * from marts.billing_day s
join schema_alpha.dict_country c on 1=1`, opts)
	want := []string{"marts.billing_day", "schema_alpha.dict_country"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestUnitTablesExcludedCatalog(t *testing.T) {
	opts := Options{ExcludedCatalogs: []string{"main"}}
	got := Tables(`select * from "main"."analytic_demo"."sample_table"`, opts)
	want := []string{"analytic_demo.sample_table"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestUnitTablesKeepOtherCatalog(t *testing.T) {
	opts := Options{ExcludedCatalogs: []string{"main"}}
	got := Tables(`select * from lakehouse.schema_alpha.events_raw`, opts)
	want := []string{"lakehouse.schema_alpha.events_raw"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestUnitTablesPersonNameDropped(t *testing.T) {
	opts := Options{}
	got := Tables(`select * from A.Bcd; select * from marts.users`, opts)
	want := []string{"marts.users"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestUnitTablesSkipExtractAndImport(t *testing.T) {
	opts := Options{}
	got := Tables(`extract(day from ts); from schema_a.table_x import foo`, opts)
	if len(got) != 0 {
		t.Fatalf("expected empty, got %v", got)
	}
}

func TestUnitTablesSchemaAllowlist(t *testing.T) {
	opts := Options{SchemaAllowlist: []string{"marts"}}
	got := Tables(`select * from marts.billing_day; select * from other_schema.x_y`, opts)
	want := []string{"marts.billing_day"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestUnitTablesRejectShortSchema(t *testing.T) {
	opts := Options{}
	got := Tables(`select * from ab.users`, opts)
	if len(got) != 0 {
		t.Fatalf("expected empty, got %v", got)
	}
}

func TestUnitTablesInlineAndPeek(t *testing.T) {
	filtered := FilterQlikScript(readTestdata(t, "inline_peek.qvs"), "vertica")
	got := Tables(filtered, Options{ExcludedCatalogs: []string{"main"}})
	// FROM/JOIN first (users_dim), then INLINE source_table rows, then Peek dedup.
	want := []string{
		"marts.users_dim",
		"marts.billing_day",
		"schema_alpha.dict_country",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestUnitTablesLetLiteralVar(t *testing.T) {
	text := `
LET vSrc = 'marts.billing_day';
SQL SELECT * FROM $(vSrc);
`
	got := Tables(text, Options{})
	want := []string{"marts.billing_day"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestUnitTablesDedupPreservesOrder(t *testing.T) {
	opts := Options{}
	got := Tables(`select * from marts.a_b; select * from marts.a_b; select * from marts.c_d`, opts)
	want := []string{"marts.a_b", "marts.c_d"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

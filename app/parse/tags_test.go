package parse

import (
	"reflect"
	"testing"
)

func TestUnitTagsMaturityAndRest(t *testing.T) {
	maturity, rest := Tags([]Tag{
		{Name: "finance"},
		{Name: "High"},
		{Name: "weekly"},
	})
	if maturity != "high" {
		t.Fatalf("maturity=%q want high", maturity)
	}
	want := []string{"finance", "weekly"}
	if !reflect.DeepEqual(rest, want) {
		t.Fatalf("rest=%v want %v", rest, want)
	}
}

func TestUnitTagsNoMaturity(t *testing.T) {
	maturity, rest := Tags([]Tag{{Name: "ops"}})
	if maturity != "" {
		t.Fatalf("maturity=%q want empty", maturity)
	}
	if !reflect.DeepEqual(rest, []string{"ops"}) {
		t.Fatalf("rest=%v", rest)
	}
}

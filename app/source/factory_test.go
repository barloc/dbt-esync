package source_test

import (
	"context"
	"errors"
	"testing"

	"github.com/barloc/dbt-esync/app/manifest"
	"github.com/barloc/dbt-esync/app/source"
)

func TestUnitNewKnownTypes(t *testing.T) {
	t.Parallel()

	for _, typ := range source.KnownTypes {
		t.Run(typ, func(t *testing.T) {
			t.Parallel()

			src, err := source.New(typ)
			if err != nil {
				t.Fatalf("New(%q): %v", typ, err)
			}
			_, err = src.Collect(context.Background(), &manifest.Finder{})
			if !errors.Is(err, source.ErrNotImplemented) {
				t.Fatalf("Collect(%q): got %v, want ErrNotImplemented", typ, err)
			}
		})
	}
}

func TestUnitNewUnknownType(t *testing.T) {
	t.Parallel()

	_, err := source.New("nope")
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
}

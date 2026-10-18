package source_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/barloc/dbt-esync/app/config"
	"github.com/barloc/dbt-esync/app/manifest"
	"github.com/barloc/dbt-esync/app/source"
)

func TestUnitNewStubsNotImplemented(t *testing.T) {
	t.Parallel()

	for _, typ := range source.KnownTypes {
		if typ == "growthbook" || typ == "mysql" || typ == "redash" || typ == "hive" || typ == "gitscan" || typ == "qlik" || typ == "superset" {
			continue
		}
		t.Run(typ, func(t *testing.T) {
			t.Parallel()

			src, err := source.New(config.SourceEntry{Name: "pkg", Type: typ}, source.Deps{})
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

func TestUnitNewGrowthBookRequiresConfig(t *testing.T) {
	t.Parallel()

	_, err := source.New(config.SourceEntry{Name: "pkg", Type: "growthbook"}, source.Deps{})
	if err == nil || !strings.Contains(err.Error(), "growthbook config") {
		t.Fatalf("got %v", err)
	}
}

func TestUnitNewMySQLRequiresConfig(t *testing.T) {
	t.Parallel()

	_, err := source.New(config.SourceEntry{Name: "pkg", Type: "mysql"}, source.Deps{})
	if err == nil || !strings.Contains(err.Error(), "mysql config") {
		t.Fatalf("got %v", err)
	}
}

func TestUnitNewRedashRequiresConfig(t *testing.T) {
	t.Parallel()

	_, err := source.New(config.SourceEntry{Name: "pkg", Type: "redash"}, source.Deps{})
	if err == nil || !strings.Contains(err.Error(), "redash config") {
		t.Fatalf("got %v", err)
	}
}

func TestUnitNewHiveRequiresConfig(t *testing.T) {
	t.Parallel()

	_, err := source.New(config.SourceEntry{Name: "pkg", Type: "hive"}, source.Deps{})
	if err == nil || !strings.Contains(err.Error(), "hive config") {
		t.Fatalf("got %v", err)
	}
}

func TestUnitNewGitscanRequiresConfig(t *testing.T) {
	t.Parallel()

	_, err := source.New(config.SourceEntry{Name: "pkg", Type: "gitscan"}, source.Deps{})
	if err == nil || !strings.Contains(err.Error(), "gitscan config") {
		t.Fatalf("got %v", err)
	}
}

func TestUnitNewQlikRequiresConfig(t *testing.T) {
	t.Parallel()

	_, err := source.New(config.SourceEntry{Name: "pkg", Type: "qlik"}, source.Deps{})
	if err == nil || !strings.Contains(err.Error(), "qlik config") {
		t.Fatalf("got %v", err)
	}
}

func TestUnitNewSupersetRequiresConfig(t *testing.T) {
	t.Parallel()

	_, err := source.New(config.SourceEntry{Name: "pkg", Type: "superset"}, source.Deps{})
	if err == nil || !strings.Contains(err.Error(), "superset config") {
		t.Fatalf("got %v", err)
	}
}

func TestUnitNewUnknownType(t *testing.T) {
	t.Parallel()

	_, err := source.New(config.SourceEntry{Name: "pkg", Type: "nope"}, source.Deps{})
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
}

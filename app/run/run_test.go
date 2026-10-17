package run

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/barloc/dbt-esync/app/config"
	"github.com/barloc/dbt-esync/app/exposure"
	"github.com/barloc/dbt-esync/app/manifest"
	"github.com/barloc/dbt-esync/app/source"
)

func TestUnitCollectAllBestEffort(t *testing.T) {
	orig := newSource
	t.Cleanup(func() { newSource = orig })

	newSource = func(e config.SourceEntry, _ source.Deps) (source.Source, error) {
		switch e.Type {
		case "ok":
			return stubSource{files: []exposure.File{{Name: "a"}}}, nil
		case "fail":
			return stubSource{err: errors.New("boom")}, nil
		case "empty":
			return stubSource{}, nil
		default:
			return nil, errors.New("unknown")
		}
	}

	entries := []config.SourceEntry{
		{Name: "pkg_ok", Type: "ok"},
		{Name: "pkg_fail", Type: "fail"},
		{Name: "pkg_empty", Type: "empty"},
	}
	pkgs, errs := collectAll(context.Background(), &manifest.Finder{}, entries, source.Deps{})
	if len(pkgs) != 1 || pkgs[0].Name != "pkg_ok" {
		t.Fatalf("packages: %+v", pkgs)
	}
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "pkg_fail") {
		t.Fatalf("errs: %v", errs)
	}
}

func TestUnitWithCollectErrsPreservesBoth(t *testing.T) {
	t.Parallel()
	c1 := errors.New("collect")
	s1 := errors.New("sync")
	err := withCollectErrs([]error{c1}, s1)
	if err == nil || !strings.Contains(err.Error(), "collect") || !strings.Contains(err.Error(), "sync") {
		t.Fatalf("got %v", err)
	}
	if withCollectErrs(nil, nil) != nil {
		t.Fatal("expected nil")
	}
	if withCollectErrs(nil, s1) != s1 {
		t.Fatalf("got %v", withCollectErrs(nil, s1))
	}
}

type stubSource struct {
	files []exposure.File
	err   error
}

func (s stubSource) Collect(context.Context, *manifest.Finder) ([]exposure.File, error) {
	return s.files, s.err
}

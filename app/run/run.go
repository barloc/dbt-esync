package run

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/barloc/dbt-esync/app/config"
	"github.com/barloc/dbt-esync/app/manifest"
	"github.com/barloc/dbt-esync/app/parse"
	sinkgit "github.com/barloc/dbt-esync/app/sink/git"
	"github.com/barloc/dbt-esync/app/sink/vcs"
	"github.com/barloc/dbt-esync/app/source"
)

// Run loads config, collects exposures from all sources (best-effort), pushes
// successful packages to the sink repo, and opens an MR/PR when enabled.
// If any Collect fails, successful packages are still synced, then a joined
// error is returned.
func Run(configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}

	ctx := context.Background()
	finder, err := manifest.Load(ctx, cfg.Manifest.URL, cfg.Manifest.File, cfg.Parse.ExcludedCatalogs)
	if err != nil {
		return err
	}

	deps := source.Deps{
		Owner:      cfg.DefaultOwner,
		FilterName: cfg.Manifest.FilterName,
		Parse: parse.Options{
			ExcludedCatalogs: cfg.Parse.ExcludedCatalogs,
			SchemaAllowlist:  cfg.Parse.SchemaAllowlist,
		},
	}
	packages, collectErrs := collectAll(ctx, finder, cfg.Source, deps)

	if len(packages) > 0 {
		pushed, err := sinkgit.Sync(ctx, cfg.Sink.Git, cfg.Sink.Git.BaseBranch, packages)
		if err != nil {
			return withCollectErrs(collectErrs, err)
		}
		if pushed && cfg.Sink.MR.Enabled {
			if err := vcs.Open(ctx, cfg.Sink.MR, cfg.Sink.Git, packageNames(packages)); err != nil {
				return withCollectErrs(collectErrs, err)
			}
		}
	}

	return withCollectErrs(collectErrs, nil)
}

// withCollectErrs returns collectErrs joined with err (if any). Collect failures
// are never dropped when a later step also fails.
func withCollectErrs(collectErrs []error, err error) error {
	if err == nil {
		if len(collectErrs) == 0 {
			return nil
		}
		return errors.Join(collectErrs...)
	}
	if len(collectErrs) == 0 {
		return err
	}
	return errors.Join(append(append([]error{}, collectErrs...), err)...)
}

// newSource builds a collector; overridden in tests.
var newSource = source.New

func collectAll(ctx context.Context, finder *manifest.Finder, entries []config.SourceEntry, deps source.Deps) ([]sinkgit.Package, []error) {
	var packages []sinkgit.Package
	var errs []error
	for i := range entries {
		e := &entries[i]
		src, err := newSource(*e, deps)
		if err != nil {
			errs = append(errs, fmt.Errorf("source %q (%s): %w", e.Name, e.Type, err))
			continue
		}
		files, err := src.Collect(ctx, finder)
		if err != nil {
			errs = append(errs, fmt.Errorf("source %q (%s): %w", e.Name, e.Type, err))
			continue
		}
		if len(files) == 0 {
			continue
		}
		packages = append(packages, sinkgit.Package{Name: e.Name, Files: files})
	}
	return packages, errs
}

func packageNames(packages []sinkgit.Package) string {
	names := make([]string, len(packages))
	for i := range packages {
		names[i] = packages[i].Name
	}
	return strings.Join(names, ", ")
}

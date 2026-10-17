package run

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
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
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	slog.Info("load config", "path", configPath)
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	slog.Info("config ok", "sources", len(cfg.Source), "sink", cfg.Sink.Git.RepoURL, "exposures_path", cfg.Sink.Git.ExposuresPath)

	ctx := context.Background()
	slog.Info("load manifest", "url", cfg.Manifest.URL, "file", cfg.Manifest.File, "filter_name", cfg.Manifest.FilterName)
	finder, err := manifest.Load(ctx, cfg.Manifest.URL, cfg.Manifest.File, cfg.Parse.ExcludedCatalogs)
	if err != nil {
		return err
	}
	slog.Info("manifest loaded")

	deps := source.Deps{
		Owner:      cfg.DefaultOwner,
		FilterName: cfg.Manifest.FilterName,
		Parse: parse.Options{
			ExcludedCatalogs: cfg.Parse.ExcludedCatalogs,
			SchemaAllowlist:  cfg.Parse.SchemaAllowlist,
		},
	}
	packages, collectErrs := collectAll(ctx, finder, cfg.Source, deps)
	slog.Info("collect finished", "packages", len(packages), "errors", len(collectErrs))

	if len(packages) > 0 {
		slog.Info("sink sync start", "branch", cfg.Sink.Git.Branch, "packages", packageNames(packages))
		pushed, err := sinkgit.Sync(ctx, cfg.Sink.Git, cfg.Sink.Git.BaseBranch, packages)
		if err != nil {
			return withCollectErrs(collectErrs, err)
		}
		slog.Info("sink sync done", "pushed", pushed)
		if pushed && cfg.Sink.MR.Enabled {
			slog.Info("open MR/PR", "provider", cfg.Sink.MR.Provider, "host", cfg.Sink.MR.Host)
			if err := vcs.Open(ctx, cfg.Sink.MR, cfg.Sink.Git, packageNames(packages)); err != nil {
				return withCollectErrs(collectErrs, err)
			}
			slog.Info("MR/PR opened")
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
		slog.Info("collect start", "source", e.Name, "type", e.Type, "i", i+1, "of", len(entries))
		src, err := newSource(*e, deps)
		if err != nil {
			slog.Error("collect init failed", "source", e.Name, "type", e.Type, "err", err)
			errs = append(errs, fmt.Errorf("source %q (%s): %w", e.Name, e.Type, err))
			continue
		}
		files, err := src.Collect(ctx, finder)
		if err != nil {
			slog.Error("collect failed", "source", e.Name, "type", e.Type, "err", err)
			errs = append(errs, fmt.Errorf("source %q (%s): %w", e.Name, e.Type, err))
			continue
		}
		nExp := 0
		for _, f := range files {
			nExp += len(f.Exposures)
		}
		if len(files) == 0 {
			slog.Info("collect empty", "source", e.Name, "type", e.Type)
			continue
		}
		slog.Info("collect ok", "source", e.Name, "type", e.Type, "files", len(files), "exposures", nExp)
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

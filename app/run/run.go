package run

import (
	"context"
	"fmt"

	"github.com/barloc/dbt-esync/app/config"
	"github.com/barloc/dbt-esync/app/manifest"
	sinkgit "github.com/barloc/dbt-esync/app/sink/git"
	"github.com/barloc/dbt-esync/app/sink/vcs"
	"github.com/barloc/dbt-esync/app/source"
)

// Run loads config, collects exposures, pushes them to the sink repo, and opens an MR/PR when enabled.
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

	src, err := source.New(cfg.Source.Type)
	if err != nil {
		return err
	}

	files, err := src.Collect(ctx, finder)
	if err != nil {
		return fmt.Errorf("source %q: %w", cfg.Source.Type, err)
	}

	pushed, err := sinkgit.Sync(ctx, cfg.Sink.Git, cfg.Sink.Git.BaseBranch, cfg.SavePath, files)
	if err != nil {
		return err
	}
	if pushed && cfg.Sink.MR.Enabled {
		if err := vcs.Open(ctx, cfg.Sink.MR, cfg.Sink.Git, cfg.SavePath); err != nil {
			return err
		}
	}
	return nil
}

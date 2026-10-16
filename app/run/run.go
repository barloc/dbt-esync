package run

import (
	"context"
	"fmt"

	"github.com/barloc/dbt-esync/app/config"
	"github.com/barloc/dbt-esync/app/manifest"
	"github.com/barloc/dbt-esync/app/source"
)

// Run loads config, constructs the Source, and calls Collect.
func Run(configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}

	src, err := source.New(cfg.Source.Type)
	if err != nil {
		return err
	}

	_, err = src.Collect(context.Background(), &manifest.Finder{})
	if err != nil {
		return fmt.Errorf("source %q: %w", cfg.Source.Type, err)
	}
	return nil
}

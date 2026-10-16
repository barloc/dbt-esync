package run

import (
	"context"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/barloc/dbt-esync/app/manifest"
	"github.com/barloc/dbt-esync/app/source"
)

// minimalConfig is the T1 subset of YAML (full schema in T2).
type minimalConfig struct {
	Source struct {
		Type string `yaml:"type"`
	} `yaml:"source"`
}

// Run loads config, constructs the Source, and calls Collect.
func Run(configPath string) error {
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	var cfg minimalConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}
	if cfg.Source.Type == "" {
		return fmt.Errorf("config: source.type is required")
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

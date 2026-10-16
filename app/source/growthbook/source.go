package growthbook

import (
	"context"

	"github.com/barloc/dbt-esync/app/exposure"
	"github.com/barloc/dbt-esync/app/manifest"
	"github.com/barloc/dbt-esync/app/source/stuberr"
)

// Source is a GrowthBook API stub (S1).
type Source struct{}

// New returns a GrowthBook source stub.
func New() *Source {
	return &Source{}
}

// Collect implements source.Source.
func (s *Source) Collect(ctx context.Context, m *manifest.Finder) ([]exposure.File, error) {
	return nil, stuberr.ErrNotImplemented
}

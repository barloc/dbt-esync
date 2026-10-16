package redash

import (
	"context"

	"github.com/barloc/dbt-esync/app/exposure"
	"github.com/barloc/dbt-esync/app/manifest"
	"github.com/barloc/dbt-esync/app/source/stuberr"
)

// Source is a Redash REST stub (S3).
type Source struct{}

// New returns a Redash source stub.
func New() *Source {
	return &Source{}
}

// Collect implements source.Source.
func (s *Source) Collect(ctx context.Context, m *manifest.Finder) ([]exposure.File, error) {
	return nil, stuberr.ErrNotImplemented
}

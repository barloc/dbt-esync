package source

import (
	"context"

	"github.com/barloc/dbt-esync/app/exposure"
	"github.com/barloc/dbt-esync/app/manifest"
)

// Source collects exposures from an external system.
type Source interface {
	Collect(ctx context.Context, m *manifest.Finder) ([]exposure.File, error)
}

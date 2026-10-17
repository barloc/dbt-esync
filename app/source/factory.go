package source

import (
	"fmt"

	"github.com/barloc/dbt-esync/app/config"
	"github.com/barloc/dbt-esync/app/source/gitscan"
	"github.com/barloc/dbt-esync/app/source/growthbook"
	"github.com/barloc/dbt-esync/app/source/hive"
	"github.com/barloc/dbt-esync/app/source/mysql"
	"github.com/barloc/dbt-esync/app/source/qlik"
	"github.com/barloc/dbt-esync/app/source/redash"
)

// KnownTypes lists supported source[].type values (alias of config.KnownSourceTypes).
var KnownTypes = config.KnownSourceTypes

// New returns a Source for the given config entry.
func New(e config.SourceEntry, deps Deps) (Source, error) {
	switch e.Type {
	case "growthbook":
		if e.GrowthBook == nil {
			return nil, fmt.Errorf("growthbook config is required")
		}
		return growthbook.New(*e.GrowthBook, growthbook.Options{
			Owner:      deps.Owner,
			Parse:      deps.Parse,
			HTTPClient: deps.HTTPClient,
		}), nil
	case "mysql":
		if e.MySQL == nil {
			return nil, fmt.Errorf("mysql config is required")
		}
		return mysql.New(*e.MySQL, mysql.Options{
			Owner: deps.Owner,
			Parse: deps.Parse,
		}), nil
	case "redash":
		return redash.New(), nil
	case "hive":
		return hive.New(), nil
	case "gitscan":
		return gitscan.New(), nil
	case "qlik":
		return qlik.New(), nil
	default:
		return nil, fmt.Errorf("unknown source.type %q", e.Type)
	}
}

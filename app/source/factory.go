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
	"github.com/barloc/dbt-esync/app/source/superset"
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
		if e.Redash == nil {
			return nil, fmt.Errorf("redash config is required")
		}
		return redash.New(*e.Redash, redash.Options{
			PackageName: e.Name,
			Owner:       deps.Owner,
			Parse:       deps.Parse,
			HTTPClient:  deps.HTTPClient,
		}), nil
	case "hive":
		if e.Hive == nil {
			return nil, fmt.Errorf("hive config is required")
		}
		return hive.New(*e.Hive, hive.Options{
			Owner: deps.Owner,
			Parse: deps.Parse,
		}), nil
	case "gitscan":
		if e.Gitscan == nil {
			return nil, fmt.Errorf("gitscan config is required")
		}
		return gitscan.New(*e.Gitscan, gitscan.Options{
			Owner:      deps.Owner,
			Parse:      deps.Parse,
			FilterName: deps.FilterName,
		}), nil
	case "qlik":
		if e.Qlik == nil {
			return nil, fmt.Errorf("qlik config is required")
		}
		return qlik.New(*e.Qlik, qlik.Options{
			PackageName: e.Name,
			Owner:       deps.Owner,
			Parse:       deps.Parse,
			HTTPClient:  deps.HTTPClient,
		}), nil
	case "superset":
		if e.Superset == nil {
			return nil, fmt.Errorf("superset config is required")
		}
		return superset.New(*e.Superset, superset.Options{
			PackageName: e.Name,
			Owner:       deps.Owner,
			Parse:       deps.Parse,
			HTTPClient:  deps.HTTPClient,
		}), nil
	default:
		return nil, fmt.Errorf("unknown source.type %q", e.Type)
	}
}

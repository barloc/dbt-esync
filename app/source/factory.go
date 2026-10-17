package source

import (
	"fmt"

	"github.com/barloc/dbt-esync/app/source/gitscan"
	"github.com/barloc/dbt-esync/app/source/growthbook"
	"github.com/barloc/dbt-esync/app/source/hive"
	"github.com/barloc/dbt-esync/app/source/mysql"
	"github.com/barloc/dbt-esync/app/source/qlik"
	"github.com/barloc/dbt-esync/app/source/redash"
)

// KnownTypes lists supported source[].type values.
var KnownTypes = []string{
	"growthbook",
	"mysql",
	"redash",
	"hive",
	"gitscan",
	"qlik",
}

// New returns a Source implementation for the given type (stubs in T1).
func New(typ string) (Source, error) {
	switch typ {
	case "growthbook":
		return growthbook.New(), nil
	case "mysql":
		return mysql.New(), nil
	case "redash":
		return redash.New(), nil
	case "hive":
		return hive.New(), nil
	case "gitscan":
		return gitscan.New(), nil
	case "qlik":
		return qlik.New(), nil
	default:
		return nil, fmt.Errorf("unknown source.type %q", typ)
	}
}

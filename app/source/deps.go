package source

import (
	"net/http"

	"github.com/barloc/dbt-esync/app/config"
	"github.com/barloc/dbt-esync/app/parse"
)

// Deps are shared inputs for source constructors (owner fallback, parse filters, HTTP).
type Deps struct {
	Owner      config.DefaultOwner
	Parse      parse.Options
	FilterName string       // manifest.filter_name (gitscan path_db_tokens)
	HTTPClient *http.Client // nil → http.DefaultClient
}

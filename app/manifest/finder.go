// Package manifest maps warehouse table names to dbt ref()/source() via manifest.json.
package manifest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
)

// Lookup is the result of resolving table names against the dbt manifest.
type Lookup struct {
	Found    []string // sorted unique ref()/source()
	NotFound []string // sorted unique input tables
}

// Finder maps warehouse table names to dbt ref()/source().
type Finder struct {
	mapping map[string]string
}

type manifestJSON struct {
	Nodes   map[string]nodeJSON   `json:"nodes"`
	Sources map[string]sourceJSON `json:"sources"`
}

type nodeJSON struct {
	ResourceType string     `json:"resource_type"`
	Name         string     `json:"name"`
	Database     string     `json:"database"`
	Alias        string     `json:"alias"`
	Config       nodeConfig `json:"config"`
}

type nodeConfig struct {
	Schema string `json:"schema"`
	Alias  string `json:"alias"`
}

type sourceJSON struct {
	SourceName string `json:"source_name"`
	Name       string `json:"name"`
	Database   string `json:"database"`
	Schema     string `json:"schema"`
}

// Load reads manifest.json from file or HTTP(S) URL (exactly one must be set).
// excludedCatalogs lists database names that are omitted from lookup keys (case-insensitive).
func Load(ctx context.Context, url, file string, excludedCatalogs []string) (*Finder, error) {
	switch {
	case url != "" && file != "":
		return nil, fmt.Errorf("manifest: url and file are mutually exclusive")
	case url == "" && file == "":
		return nil, fmt.Errorf("manifest: url or file is required")
	}

	var raw []byte
	var err error
	if file != "" {
		raw, err = os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("manifest: read file: %w", err)
		}
	} else {
		raw, err = fetchURL(ctx, url)
		if err != nil {
			return nil, err
		}
	}

	var data manifestJSON
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("manifest: parse json: %w", err)
	}

	excluded := make(map[string]struct{}, len(excludedCatalogs))
	for _, c := range excludedCatalogs {
		excluded[strings.ToLower(c)] = struct{}{}
	}

	// Separate maps then merge so sources win every key (incl. 2-part dual-keys),
	// matching legacy {**models, **sources}.
	models := map[string]string{}
	indexModels(models, data.Nodes, excluded)
	sources := map[string]string{}
	indexSources(sources, data.Sources, excluded)
	mapping := models
	for k, v := range sources {
		mapping[k] = v
	}
	return &Finder{mapping: mapping}, nil
}

func fetchURL(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("manifest: url request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("manifest: url fetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("manifest: url fetch: status %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("manifest: url read: %w", err)
	}
	return raw, nil
}

func indexModels(mapping map[string]string, nodes map[string]nodeJSON, excluded map[string]struct{}) {
	for _, n := range nodes {
		if n.ResourceType != "model" {
			continue
		}
		alias := n.Config.Alias
		if alias == "" {
			alias = n.Alias
		}
		dbtRef := fmt.Sprintf("ref('%s')", n.Name)
		addMapping(mapping, n.Database, n.Config.Schema, alias, dbtRef, excluded)
	}
}

func indexSources(mapping map[string]string, sources map[string]sourceJSON, excluded map[string]struct{}) {
	for _, s := range sources {
		dbtRef := fmt.Sprintf("source('%s', '%s')", s.SourceName, s.Name)
		addMapping(mapping, s.Database, s.Schema, s.Name, dbtRef, excluded)
	}
}

func addMapping(mapping map[string]string, database, schema, name, dbtRef string, excluded map[string]struct{}) {
	schema = strings.ToLower(schema)
	name = strings.ToLower(name)
	db := strings.ToLower(database)

	key := makeLookupKey(db, schema, name, excluded)
	mapping[key] = dbtRef
	if db != "" {
		if _, skip := excluded[db]; !skip {
			short := schema + "." + name
			if _, ok := mapping[short]; !ok {
				mapping[short] = dbtRef
			}
		}
	}
}

func makeLookupKey(db, schema, name string, excluded map[string]struct{}) string {
	if db != "" {
		if _, skip := excluded[db]; !skip {
			return db + "." + schema + "." + name
		}
	}
	return schema + "." + name
}

// Refs resolves warehouse table names to dbt ref()/source() strings.
func (f *Finder) Refs(tables []string) Lookup {
	foundSet := map[string]struct{}{}
	notFoundSet := map[string]struct{}{}
	for _, table := range tables {
		if table == "" {
			continue
		}
		key := strings.ToLower(table)
		ref := f.lookup(key)
		if ref != "" {
			foundSet[ref] = struct{}{}
		} else {
			notFoundSet[table] = struct{}{}
		}
	}
	return Lookup{
		Found:    sortedKeys(foundSet),
		NotFound: sortedKeys(notFoundSet),
	}
}

func (f *Finder) lookup(table string) string {
	if ref, ok := f.mapping[table]; ok {
		return ref
	}
	parts := strings.Split(table, ".")
	if len(parts) == 3 {
		return f.mapping[parts[1]+"."+parts[2]]
	}
	return ""
}

func sortedKeys(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

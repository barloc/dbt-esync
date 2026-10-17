package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"text/template"

	"github.com/go-sql-driver/mysql"

	"github.com/barloc/dbt-esync/app/config"
	"github.com/barloc/dbt-esync/app/exposure"
	"github.com/barloc/dbt-esync/app/manifest"
	"github.com/barloc/dbt-esync/app/parse"
)

const (
	fileName       = "mysql"
	defaultDesc    = "SQL catalog query"
	defaultPort    = 3306
	notFoundPrefix = "Tables not in dbt: "
)

// RowScanner is the subset of *sql.Rows used by Collect.
type RowScanner interface {
	Next() bool
	Scan(dest ...any) error
	Columns() ([]string, error)
	Err() error
	Close() error
}

// Querier is a DB handle used to fetch catalog rows (injectable for tests).
type Querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (RowScanner, error)
	Close() error
}

// Options configures a MySQL SQL-catalog source.
type Options struct {
	Owner config.DefaultOwner
	Parse parse.Options
	DB    Querier // nil → open from cfg
}

// Source collects exposures from a MySQL SQL catalog.
type Source struct {
	cfg   config.SQLCatalog
	owner config.DefaultOwner
	parse parse.Options
	db    Querier
}

// New returns a MySQL SQL-catalog source.
func New(cfg config.SQLCatalog, opt Options) *Source {
	return &Source{
		cfg:   cfg,
		owner: opt.Owner,
		parse: opt.Parse,
		db:    opt.DB,
	}
}

// Collect implements source.Source.
func (s *Source) Collect(ctx context.Context, m *manifest.Finder) ([]exposure.File, error) {
	db := s.db
	owned := false
	if db == nil {
		opened, err := openDB(ctx, s.cfg)
		if err != nil {
			return nil, err
		}
		db = opened
		owned = true
	}
	if owned {
		defer func() { _ = db.Close() }()
	}

	urlTmpl, err := template.New("url").Parse(s.cfg.ExposureURLTemplate)
	if err != nil {
		return nil, fmt.Errorf("mysql: exposure_url_template: %w", err)
	}

	query := catalogSQL(s.cfg)
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("mysql: query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	catalogRows, err := scanCatalog(rows, s.cfg)
	if err != nil {
		return nil, err
	}

	var exposures []exposure.Exposure
	for _, row := range catalogRows {
		exp, ok, err := s.buildExposure(row, m, urlTmpl)
		if err != nil {
			return nil, err
		}
		if ok {
			exposures = append(exposures, exp)
		}
	}
	if len(exposures) == 0 {
		return nil, nil
	}
	return []exposure.File{{Name: fileName, Exposures: exposures}}, nil
}

type catalogRow struct {
	ID          string
	SQL         string
	Description string
}

func (s *Source) buildExposure(row catalogRow, m *manifest.Finder, urlTmpl *template.Template) (exposure.Exposure, bool, error) {
	tables := parse.Tables(row.SQL, s.parse)
	lookup := m.Refs(tables)
	if len(lookup.Found) == 0 && len(lookup.NotFound) == 0 {
		return exposure.Exposure{}, false, nil
	}

	url, err := renderURL(urlTmpl, row.ID)
	if err != nil {
		return exposure.Exposure{}, false, err
	}

	desc := row.Description
	if desc == "" || desc == "NULL" {
		desc = defaultDesc
	}
	if len(lookup.NotFound) > 0 {
		desc += "\n" + notFoundPrefix + strings.Join(lookup.NotFound, ", ")
	}

	id := sanitizeID(row.ID)
	return exposure.Exposure{
		Name:        s.cfg.ExposurePrefix + "_" + id,
		Type:        "application",
		URL:         url,
		Label:       s.cfg.ExposurePrefix + ": " + id,
		Description: desc,
		OwnerName:   s.owner.Name,
		OwnerEmail:  s.owner.Email,
		DependsOn:   lookup.Found,
	}, true, nil
}

func renderURL(tmpl *template.Template, id string) (string, error) {
	var buf strings.Builder
	if err := tmpl.Execute(&buf, struct{ ID string }{ID: id}); err != nil {
		return "", fmt.Errorf("mysql: exposure_url_template: %w", err)
	}
	return buf.String(), nil
}

// sanitizeID keeps dbt exposure name chars ([A-Za-z0-9_]); everything else → _.
func sanitizeID(id string) string {
	var b strings.Builder
	b.Grow(len(id))
	for i := 0; i < len(id); i++ {
		c := id[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '_':
			b.WriteByte(c)
		default:
			b.WriteByte('_')
		}
	}
	out := b.String()
	if out == "" {
		return "item"
	}
	return out
}

func catalogSQL(cfg config.SQLCatalog) string {
	if cfg.Query != "" {
		return cfg.Query
	}
	cols := []string{quoteIdent(cfg.IDColumn), quoteIdent(cfg.SQLColumn)}
	if cfg.DescriptionColumn != "" {
		cols = append(cols, quoteIdent(cfg.DescriptionColumn))
	}
	q := fmt.Sprintf("SELECT %s FROM %s.%s",
		strings.Join(cols, ", "),
		quoteIdent(cfg.Database),
		quoteIdent(cfg.Table))
	if cfg.ExtraFilters != "" {
		q += " WHERE " + cfg.ExtraFilters
	}
	return q
}

func quoteIdent(s string) string {
	return "`" + strings.ReplaceAll(s, "`", "``") + "`"
}

func scanCatalog(rows RowScanner, cfg config.SQLCatalog) ([]catalogRow, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("mysql: columns: %w", err)
	}
	idx := map[string]int{}
	for i, c := range cols {
		idx[c] = i
	}
	for _, need := range []string{cfg.IDColumn, cfg.SQLColumn} {
		if _, ok := idx[need]; !ok {
			return nil, fmt.Errorf("mysql: result missing column %q", need)
		}
	}
	descIdx, hasDesc := -1, false
	if cfg.DescriptionColumn != "" {
		if i, ok := idx[cfg.DescriptionColumn]; ok {
			descIdx, hasDesc = i, true
		}
		// Missing description column is OK (e.g. custom query without it).
	}

	var out []catalogRow
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("mysql: scan: %w", err)
		}
		row := catalogRow{
			ID:  asString(vals[idx[cfg.IDColumn]]),
			SQL: asString(vals[idx[cfg.SQLColumn]]),
		}
		if hasDesc {
			row.Description = asString(vals[descIdx])
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mysql: rows: %w", err)
	}
	return out, nil
}

func asString(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case []byte:
		return string(t)
	case string:
		return t
	default:
		return fmt.Sprint(t)
	}
}

type sqlQuerier struct {
	db *sql.DB
}

func (q sqlQuerier) QueryContext(ctx context.Context, query string, args ...any) (RowScanner, error) {
	return q.db.QueryContext(ctx, query, args...)
}

func (q sqlQuerier) Close() error {
	return q.db.Close()
}

func openDB(ctx context.Context, cfg config.SQLCatalog) (Querier, error) {
	port := cfg.Port
	if port == 0 {
		port = defaultPort
	}
	mc := mysql.Config{
		User:                 cfg.User,
		Passwd:               cfg.Password,
		Net:                  "tcp",
		Addr:                 fmt.Sprintf("%s:%d", cfg.Host, port),
		DBName:               cfg.Database,
		AllowNativePasswords: true,
	}
	db, err := sql.Open("mysql", mc.FormatDSN())
	if err != nil {
		return nil, fmt.Errorf("mysql: open: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("mysql: ping: %w", err)
	}
	return sqlQuerier{db: db}, nil
}

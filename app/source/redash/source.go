package redash

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/barloc/dbt-esync/app/config"
	"github.com/barloc/dbt-esync/app/exposure"
	"github.com/barloc/dbt-esync/app/manifest"
	"github.com/barloc/dbt-esync/app/parse"
)

const (
	fileDashboard  = "redash_dashboard"
	fileAlerts     = "redash_alerts"
	pageSize       = 100
	notFoundPrefix = "Tables not in dbt: "
	entityDash     = "dashboard"
	entityAlerts   = "alerts"
)

// Options configures a Redash source.
type Options struct {
	PackageName string // source[].name → exposure name prefix
	Owner       config.DefaultOwner
	Parse       parse.Options
	HTTPClient  *http.Client
}

// Source collects exposures from the Redash REST API.
type Source struct {
	cfg     config.RedashConfig
	pkg     string
	owner   config.DefaultOwner
	parse   parse.Options
	client  *http.Client
	display string
}

// New returns a Redash source.
func New(cfg config.RedashConfig, opt Options) *Source {
	client := opt.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	display := cfg.HostToDisplay
	if display == "" {
		display = cfg.Host
	}
	display = strings.TrimRight(display, "/")
	return &Source{
		cfg:     cfg,
		pkg:     opt.PackageName,
		owner:   opt.Owner,
		parse:   opt.Parse,
		client:  client,
		display: display,
	}
}

// Collect implements source.Source.
func (s *Source) Collect(ctx context.Context, m *manifest.Finder) ([]exposure.File, error) {
	matched, err := s.matchedDataSources(ctx)
	if err != nil {
		return nil, err
	}

	qcache := map[int]queryBody{}
	var files []exposure.File

	for _, et := range s.entityTypes() {
		var exps []exposure.Exposure
		switch et {
		case entityDash:
			exps, err = s.collectDashboards(ctx, m, matched, qcache)
		case entityAlerts:
			exps, err = s.collectAlerts(ctx, m, matched, qcache)
		}
		if err != nil {
			return nil, err
		}
		if len(exps) == 0 {
			continue
		}
		name := fileDashboard
		if et == entityAlerts {
			name = fileAlerts
		}
		files = append(files, exposure.File{Name: name, Exposures: exps})
	}
	if len(files) == 0 {
		return nil, nil
	}
	return files, nil
}

func (s *Source) entityTypes() []string {
	if len(s.cfg.EntityTypes) == 0 {
		return []string{entityDash, entityAlerts}
	}
	return s.cfg.EntityTypes
}

type apiUser struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type dataSource struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type dashListPage struct {
	Results []dashListItem `json:"results"`
}

type dashListItem struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type dashDetail struct {
	ID      int          `json:"id"`
	Name    string       `json:"name"`
	User    apiUser      `json:"user"`
	Widgets []dashWidget `json:"widgets"`
}

type dashWidget struct {
	Visualization *struct {
		Query *struct {
			ID           int    `json:"id"`
			Name         string `json:"name"`
			DataSourceID int    `json:"data_source_id"`
		} `json:"query"`
	} `json:"visualization"`
}

type alertItem struct {
	ID   int     `json:"id"`
	Name string  `json:"name"`
	User apiUser `json:"user"`
	Query *struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"query"`
}

type queryBody struct {
	ID           int    `json:"id"`
	Query        string `json:"query"`
	DataSourceID int    `json:"data_source_id"`
}

type collected struct {
	ID          int
	Name        string
	Description string
	SQL         string
	OwnerName   string
	OwnerEmail  string
	URLPath     string // "dashboards" or "alerts"
	NamePrefix  string // "dashboard" or "alert"
	LabelPrefix string
}

func (s *Source) matchedDataSources(ctx context.Context) (map[int]struct{}, error) {
	var list []dataSource
	if err := s.getJSON(ctx, "/api/data_sources", &list); err != nil {
		return nil, err
	}
	needle := strings.ToLower(s.cfg.DatasourceNameContains)
	out := map[int]struct{}{}
	for _, ds := range list {
		if strings.Contains(strings.ToLower(ds.Name), needle) {
			out[ds.ID] = struct{}{}
		}
	}
	return out, nil
}

func (s *Source) collectDashboards(ctx context.Context, m *manifest.Finder, matched map[int]struct{}, qcache map[int]queryBody) ([]exposure.Exposure, error) {
	items, err := s.listDashboards(ctx)
	if err != nil {
		return nil, err
	}
	var out []exposure.Exposure
	for _, item := range items {
		detail, err := s.fetchDashboard(ctx, item.ID)
		if err != nil {
			return nil, err
		}
		c, ok, err := s.fromDashboard(ctx, detail, matched, qcache)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		if exp, ok := s.buildExposure(c, m); ok {
			out = append(out, exp)
		}
	}
	return out, nil
}

func (s *Source) collectAlerts(ctx context.Context, m *manifest.Finder, matched map[int]struct{}, qcache map[int]queryBody) ([]exposure.Exposure, error) {
	var alerts []alertItem
	if err := s.getJSON(ctx, "/api/alerts", &alerts); err != nil {
		return nil, err
	}
	var out []exposure.Exposure
	for _, a := range alerts {
		c, ok, err := s.fromAlert(ctx, a, matched, qcache)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		if exp, ok := s.buildExposure(c, m); ok {
			out = append(out, exp)
		}
	}
	return out, nil
}

func (s *Source) listDashboards(ctx context.Context) ([]dashListItem, error) {
	var all []dashListItem
	for page := 1; ; page++ {
		path := fmt.Sprintf("/api/dashboards?page=%d&page_size=%d", page, pageSize)
		var p dashListPage
		if err := s.getJSON(ctx, path, &p); err != nil {
			return nil, err
		}
		if len(p.Results) == 0 {
			break
		}
		all = append(all, p.Results...)
		if len(p.Results) < pageSize {
			break
		}
	}
	return all, nil
}

func (s *Source) fetchDashboard(ctx context.Context, id int) (dashDetail, error) {
	var d dashDetail
	err := s.getJSON(ctx, "/api/dashboards/"+strconv.Itoa(id), &d)
	return d, err
}

func (s *Source) fetchQuery(ctx context.Context, id int, qcache map[int]queryBody) (queryBody, error) {
	if q, ok := qcache[id]; ok {
		return q, nil
	}
	var q queryBody
	if err := s.getJSON(ctx, "/api/queries/"+strconv.Itoa(id), &q); err != nil {
		return q, err
	}
	qcache[id] = q
	return q, nil
}

func (s *Source) fromDashboard(ctx context.Context, d dashDetail, matched map[int]struct{}, qcache map[int]queryBody) (collected, bool, error) {
	sqlByID := map[int]string{}
	links := map[string]struct{}{}
	for _, w := range d.Widgets {
		if w.Visualization == nil || w.Visualization.Query == nil {
			continue
		}
		wq := w.Visualization.Query
		if _, ok := matched[wq.DataSourceID]; !ok {
			continue
		}
		q, err := s.fetchQuery(ctx, wq.ID, qcache)
		if err != nil {
			return collected{}, false, err
		}
		sqlByID[wq.ID] = q.Query
		links[fmt.Sprintf("* [%s](%s/queries/%d)", wq.Name, s.display, wq.ID)] = struct{}{}
	}
	if len(links) == 0 {
		return collected{}, false, nil
	}
	ids := make([]int, 0, len(sqlByID))
	for id := range sqlByID {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	sqlParts := make([]string, 0, len(ids))
	for _, id := range ids {
		sqlParts = append(sqlParts, sqlByID[id])
	}
	rows := make([]string, 0, len(links))
	for row := range links {
		rows = append(rows, row)
	}
	sort.Strings(rows)
	return collected{
		ID:          d.ID,
		Name:        d.Name,
		Description: "Queries in dashboard:\n" + strings.Join(rows, "\n"),
		SQL:         strings.Join(sqlParts, "\n"),
		OwnerName:   d.User.Name,
		OwnerEmail:  d.User.Email,
		URLPath:     "dashboards",
		NamePrefix:  "dashboard",
		LabelPrefix: "REDASH",
	}, true, nil
}

func (s *Source) fromAlert(ctx context.Context, a alertItem, matched map[int]struct{}, qcache map[int]queryBody) (collected, bool, error) {
	if a.Query == nil {
		return collected{}, false, nil
	}
	q, err := s.fetchQuery(ctx, a.Query.ID, qcache)
	if err != nil {
		return collected{}, false, err
	}
	if _, ok := matched[q.DataSourceID]; !ok {
		return collected{}, false, nil
	}
	link := fmt.Sprintf("* [%s](%s/queries/%d)", a.Query.Name, s.display, a.Query.ID)
	return collected{
		ID:          a.ID,
		Name:        a.Name,
		Description: "Query in alert:\n" + link,
		SQL:         q.Query,
		OwnerName:   a.User.Name,
		OwnerEmail:  a.User.Email,
		URLPath:     "alerts",
		NamePrefix:  "alert",
		LabelPrefix: "REDASH ALERTS",
	}, true, nil
}

func (s *Source) buildExposure(c collected, m *manifest.Finder) (exposure.Exposure, bool) {
	tables := parse.Tables(c.SQL, s.parse)
	lookup := m.Refs(tables)
	if len(lookup.Found) == 0 && len(lookup.NotFound) == 0 {
		return exposure.Exposure{}, false
	}

	desc := c.Description
	if len(lookup.NotFound) > 0 {
		desc += "\n" + notFoundPrefix + strings.Join(lookup.NotFound, ", ")
	}

	ownerName, ownerEmail := s.resolveOwner(c.OwnerName, c.OwnerEmail)
	return exposure.Exposure{
		Name:        fmt.Sprintf("%s_%s_%d", s.pkg, c.NamePrefix, c.ID),
		Type:        "dashboard",
		URL:         fmt.Sprintf("%s/%s/%d", s.display, c.URLPath, c.ID),
		Label:       c.LabelPrefix + ": " + c.Name,
		Description: desc,
		OwnerName:   ownerName,
		OwnerEmail:  ownerEmail,
		DependsOn:   lookup.Found,
	}, true
}

func (s *Source) resolveOwner(apiName, apiEmail string) (string, string) {
	name, email := s.owner.Name, s.owner.Email
	if !s.cfg.UseAPIOwner {
		return name, email
	}
	if apiName != "" {
		name = apiName
	}
	if apiEmail != "" {
		email = apiEmail
	}
	return name, email
}

func (s *Source) getJSON(ctx context.Context, path string, dest any) error {
	rawURL := strings.TrimRight(s.cfg.Host, "/") + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return fmt.Errorf("redash: request: %w", err)
	}
	req.Header.Set("Authorization", "Key "+s.cfg.APIKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("redash: fetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("redash: read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("redash: status %d", resp.StatusCode)
	}
	if err := json.Unmarshal(body, dest); err != nil {
		return fmt.Errorf("redash: decode: %w", err)
	}
	return nil
}

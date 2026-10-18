package superset

import (
	"bytes"
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
	fileName       = "superset"
	pageSize       = 100
	notFoundPrefix = "Tables not in dbt: "
)

// Options configures a Superset source.
type Options struct {
	PackageName string // source[].name → exposure name prefix
	Owner       config.DefaultOwner
	Parse       parse.Options
	HTTPClient  *http.Client
}

// Source collects exposures from the Superset REST API.
type Source struct {
	cfg     config.SupersetConfig
	pkg     string
	owner   config.DefaultOwner
	parse   parse.Options
	client  *http.Client
	display string
	token   string // Bearer token (API key or login JWT)
}

// New returns a Superset source.
func New(cfg config.SupersetConfig, opt Options) *Source {
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
	if err := s.ensureToken(ctx); err != nil {
		return nil, err
	}
	matched, err := s.matchedDatabases(ctx)
	if err != nil {
		return nil, err
	}
	dashboards, err := s.listDashboards(ctx)
	if err != nil {
		return nil, err
	}

	var exposures []exposure.Exposure
	for _, d := range dashboards {
		exp, ok, err := s.collectDashboard(ctx, d, matched, m)
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

func (s *Source) ensureToken(ctx context.Context) error {
	if s.cfg.APIKey != "" {
		s.token = s.cfg.APIKey
		return nil
	}
	token, err := s.login(ctx)
	if err != nil {
		return err
	}
	s.token = token
	return nil
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Provider string `json:"provider"`
	Refresh  bool   `json:"refresh"`
}

type loginResponse struct {
	AccessToken string `json:"access_token"`
}

func (s *Source) login(ctx context.Context) (string, error) {
	body, err := json.Marshal(loginRequest{
		Username: s.cfg.Username,
		Password: s.cfg.Password,
		Provider: "db",
		Refresh:  true,
	})
	if err != nil {
		return "", fmt.Errorf("superset: login encode: %w", err)
	}
	rawURL := strings.TrimRight(s.cfg.Host, "/") + "/api/v1/security/login"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("superset: login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("superset: login fetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("superset: login read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("superset: login status %d", resp.StatusCode)
	}
	var out loginResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("superset: login decode: %w", err)
	}
	if out.AccessToken == "" {
		return "", fmt.Errorf("superset: login: empty access_token")
	}
	return out.AccessToken, nil
}

type listPage struct {
	Count  int             `json:"count"`
	Result json.RawMessage `json:"result"`
}

type databaseItem struct {
	ID           int    `json:"id"`
	DatabaseName string `json:"database_name"`
}

type dashboardItem struct {
	ID             int        `json:"id"`
	DashboardTitle string     `json:"dashboard_title"`
	Owners         []apiOwner `json:"owners"`
}

type apiOwner struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
}

type datasetItem struct {
	ID        int             `json:"id"`
	Name      string          `json:"name"`
	TableName string          `json:"table_name"`
	Schema    string          `json:"schema"`
	SQL       string          `json:"sql"`
	Database  datasetDatabase `json:"database"`
}

type datasetDatabase struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type datasetsResponse struct {
	Result []datasetItem `json:"result"`
}

func (s *Source) matchedDatabases(ctx context.Context) (map[int]struct{}, error) {
	var all []databaseItem
	for page := 0; ; page++ {
		var batch []databaseItem
		n, err := s.getListPage(ctx, "/api/v1/database/", page, &batch)
		if err != nil {
			return nil, err
		}
		all = append(all, batch...)
		if len(all) >= n || len(batch) == 0 {
			break
		}
	}
	needle := strings.ToLower(s.cfg.DatabaseNameContains)
	out := map[int]struct{}{}
	for _, db := range all {
		if strings.Contains(strings.ToLower(db.DatabaseName), needle) {
			out[db.ID] = struct{}{}
		}
	}
	return out, nil
}

func (s *Source) listDashboards(ctx context.Context) ([]dashboardItem, error) {
	var all []dashboardItem
	for page := 0; ; page++ {
		var batch []dashboardItem
		n, err := s.getListPage(ctx, "/api/v1/dashboard/", page, &batch)
		if err != nil {
			return nil, err
		}
		all = append(all, batch...)
		if len(all) >= n || len(batch) == 0 {
			break
		}
	}
	return all, nil
}

func (s *Source) getListPage(ctx context.Context, path string, page int, dest any) (int, error) {
	q := fmt.Sprintf("?q=(page:%d,page_size:%d)", page, pageSize)
	var p listPage
	if err := s.getJSON(ctx, path+q, &p); err != nil {
		return 0, err
	}
	if err := json.Unmarshal(p.Result, dest); err != nil {
		return 0, fmt.Errorf("superset: list decode: %w", err)
	}
	return p.Count, nil
}

func (s *Source) fetchDatasets(ctx context.Context, dashID int) ([]datasetItem, error) {
	var out datasetsResponse
	path := "/api/v1/dashboard/" + strconv.Itoa(dashID) + "/datasets"
	if err := s.getJSON(ctx, path, &out); err != nil {
		return nil, err
	}
	return out.Result, nil
}

func (s *Source) collectDashboard(ctx context.Context, d dashboardItem, matched map[int]struct{}, m *manifest.Finder) (exposure.Exposure, bool, error) {
	datasets, err := s.fetchDatasets(ctx, d.ID)
	if err != nil {
		return exposure.Exposure{}, false, err
	}

	sqlByID := map[int]string{}
	links := map[string]struct{}{}
	for _, ds := range datasets {
		if _, ok := matched[ds.Database.ID]; !ok {
			continue
		}
		sqlText := strings.TrimSpace(ds.SQL)
		if sqlText == "" {
			sqlText = physicalSelect(ds.Schema, ds.TableName)
			if sqlText == "" {
				continue
			}
		}
		sqlByID[ds.ID] = sqlText
		label := ds.Name
		if label == "" {
			label = ds.TableName
		}
		if label == "" {
			label = strconv.Itoa(ds.ID)
		}
		links[fmt.Sprintf("* [%s](%s/explore/?datasource_type=table&datasource_id=%d)", label, s.display, ds.ID)] = struct{}{}
	}
	if len(sqlByID) == 0 {
		return exposure.Exposure{}, false, nil
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

	ownerName, ownerEmail := "", ""
	if len(d.Owners) > 0 {
		o := d.Owners[0]
		ownerName = strings.TrimSpace(o.FirstName + " " + o.LastName)
		ownerEmail = o.Email
	}

	exp, ok := s.buildExposure(d, strings.Join(sqlParts, "\n"), "Datasets in dashboard:\n"+strings.Join(rows, "\n"), ownerName, ownerEmail, m)
	return exp, ok, nil
}

// physicalSelect builds SQL that parse.Tables can extract (schema.table only).
// Empty schema or table → skip; bare FROM table is not a valid warehouse ref.
func physicalSelect(schema, table string) string {
	schema = strings.TrimSpace(schema)
	table = strings.TrimSpace(table)
	if schema == "" || table == "" {
		return ""
	}
	return "SELECT 1 FROM " + schema + "." + table
}

func (s *Source) buildExposure(d dashboardItem, sql, desc, apiName, apiEmail string, m *manifest.Finder) (exposure.Exposure, bool) {
	tables := parse.Tables(sql, s.parse)
	lookup := m.Refs(tables)
	if len(lookup.Found) == 0 && len(lookup.NotFound) == 0 {
		return exposure.Exposure{}, false
	}
	if len(lookup.NotFound) > 0 {
		desc += "\n" + notFoundPrefix + strings.Join(lookup.NotFound, ", ")
	}
	ownerName, ownerEmail := s.resolveOwner(apiName, apiEmail)
	title := d.DashboardTitle
	if title == "" {
		title = strconv.Itoa(d.ID)
	}
	return exposure.Exposure{
		Name:        fmt.Sprintf("%s_dashboard_%d", s.pkg, d.ID),
		Type:        "dashboard",
		URL:         fmt.Sprintf("%s/superset/dashboard/%d/", s.display, d.ID),
		Label:       "SUPERSET: " + title,
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
		return fmt.Errorf("superset: request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.token)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("superset: fetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("superset: read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("superset: status %d", resp.StatusCode)
	}
	if err := json.Unmarshal(body, dest); err != nil {
		return fmt.Errorf("superset: decode: %w", err)
	}
	return nil
}

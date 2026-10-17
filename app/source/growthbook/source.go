package growthbook

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/barloc/dbt-esync/app/config"
	"github.com/barloc/dbt-esync/app/exposure"
	"github.com/barloc/dbt-esync/app/manifest"
	"github.com/barloc/dbt-esync/app/parse"
)

const (
	fileName        = "growthbook"
	defaultDesc     = "GrowthBook assignment query"
	defaultPageSize = 100
	notFoundPrefix  = "Tables not in dbt: "
)

// Options configures a GrowthBook source.
type Options struct {
	Owner      config.DefaultOwner
	Parse      parse.Options
	HTTPClient *http.Client
}

// Source collects exposures from the GrowthBook data-sources API.
type Source struct {
	cfg    config.GrowthBookConfig
	owner  config.DefaultOwner
	parse  parse.Options
	client *http.Client
}

// New returns a GrowthBook source.
func New(cfg config.GrowthBookConfig, opt Options) *Source {
	client := opt.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	return &Source{
		cfg:    cfg,
		owner:  opt.Owner,
		parse:  opt.Parse,
		client: client,
	}
}

// Collect implements source.Source.
func (s *Source) Collect(ctx context.Context, m *manifest.Finder) ([]exposure.File, error) {
	sources, err := s.fetchAll(ctx)
	if err != nil {
		return nil, err
	}

	var exposures []exposure.Exposure
	for _, ds := range sources {
		if ds.Type != s.cfg.FilterSource {
			continue
		}
		for _, q := range ds.AssignmentQueries {
			exp, ok := s.buildExposure(ds.ID, q, m)
			if ok {
				exposures = append(exposures, exp)
			}
		}
	}
	if len(exposures) == 0 {
		return nil, nil
	}
	return []exposure.File{{Name: fileName, Exposures: exposures}}, nil
}

type apiResponse struct {
	Count       int          `json:"count"`
	DataSources []dataSource `json:"dataSources"`
}

type dataSource struct {
	ID                string            `json:"id"`
	Type              string            `json:"type"`
	AssignmentQueries []assignmentQuery `json:"assignmentQueries"`
}

type assignmentQuery struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	SQL         string `json:"sql"`
}

func (s *Source) fetchAll(ctx context.Context) ([]dataSource, error) {
	base, err := url.Parse(s.cfg.Host)
	if err != nil {
		return nil, fmt.Errorf("growthbook: host URL: %w", err)
	}

	var all []dataSource
	offset := 0
	for {
		pageURL := *base
		q := pageURL.Query()
		q.Set("limit", strconv.Itoa(defaultPageSize))
		q.Set("offset", strconv.Itoa(offset))
		pageURL.RawQuery = q.Encode()

		page, err := s.fetchPage(ctx, pageURL.String())
		if err != nil {
			return nil, err
		}
		n := len(page.DataSources)
		if n == 0 {
			break
		}
		all = append(all, page.DataSources...)
		offset += n
		if offset >= page.Count {
			break
		}
	}
	return all, nil
}

func (s *Source) fetchPage(ctx context.Context, rawURL string) (apiResponse, error) {
	var out apiResponse
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return out, fmt.Errorf("growthbook: request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.cfg.APIKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return out, fmt.Errorf("growthbook: fetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return out, fmt.Errorf("growthbook: read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return out, fmt.Errorf("growthbook: status %d", resp.StatusCode)
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return out, fmt.Errorf("growthbook: decode: %w", err)
	}
	return out, nil
}

func (s *Source) buildExposure(dataSourceID string, q assignmentQuery, m *manifest.Finder) (exposure.Exposure, bool) {
	tables := parse.Tables(q.SQL, s.parse)
	lookup := m.Refs(tables)
	if len(lookup.Found) == 0 && len(lookup.NotFound) == 0 {
		return exposure.Exposure{}, false
	}

	desc := q.Description
	if desc == "" {
		desc = defaultDesc
	}
	if len(lookup.NotFound) > 0 {
		desc += "\n" + notFoundPrefix + strings.Join(lookup.NotFound, ", ")
	}

	return exposure.Exposure{
		Name:        fmt.Sprintf("growthbook_%s_%s", dataSourceID, q.ID),
		Type:        "application",
		URL:         s.cfg.ExposureURL + "/" + dataSourceID,
		Label:       "GROWTHBOOK: " + q.ID,
		Description: desc,
		OwnerName:   s.owner.Name,
		OwnerEmail:  s.owner.Email,
		DependsOn:   lookup.Found,
	}, true
}

package qlik

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"text/template"

	"github.com/barloc/dbt-esync/app/config"
	"github.com/barloc/dbt-esync/app/exposure"
	"github.com/barloc/dbt-esync/app/manifest"
	"github.com/barloc/dbt-esync/app/parse"
)

const (
	fileName       = "qlik"
	defaultDesc    = "Qlik Sense app"
	notFoundPrefix = "Tables not in dbt: "
)

// App is one Sense app in a stream.
type App struct {
	ID     string
	Name   string
	Stream string
}

// Catalog lists apps with stream names.
type Catalog interface {
	ListApps(ctx context.Context) ([]App, error)
}

// ScriptFetcher reads the load script for one app.
type ScriptFetcher interface {
	LoadScript(ctx context.Context, appID string) (string, error)
}

// TagFetcher returns QRS tags for an app (optional).
type TagFetcher interface {
	AppTags(ctx context.Context, appID string) ([]parse.Tag, error)
}

// Options configures a Qlik source.
type Options struct {
	PackageName string
	Owner       config.DefaultOwner
	Parse       parse.Options
	Catalog     Catalog
	Scripts     ScriptFetcher
	Tags        TagFetcher // nil → skip maturity/tags
	HTTPClient  *http.Client
}

// Source collects exposures from Qlik Sense apps.
type Source struct {
	cfg     config.QlikConfig
	pkg     string
	owner   config.DefaultOwner
	parse   parse.Options
	catalog Catalog
	scripts ScriptFetcher
	tags    TagFetcher
	http    *http.Client
}

// New returns a Qlik source.
func New(cfg config.QlikConfig, opt Options) *Source {
	return &Source{
		cfg:     cfg,
		pkg:     opt.PackageName,
		owner:   opt.Owner,
		parse:   opt.Parse,
		catalog: opt.Catalog,
		scripts: opt.Scripts,
		tags:    opt.Tags,
		http:    opt.HTTPClient,
	}
}

// Collect implements source.Source.
func (s *Source) Collect(ctx context.Context, m *manifest.Finder) ([]exposure.File, error) {
	if err := s.ensureClients(); err != nil {
		return nil, err
	}

	urlTmpl, err := template.New("url").Parse(s.cfg.ExposureURLTemplate)
	if err != nil {
		return nil, fmt.Errorf("qlik: exposure_url_template: %w", err)
	}

	apps, err := s.catalog.ListApps(ctx)
	if err != nil {
		return nil, fmt.Errorf("qlik: list apps: %w", err)
	}

	var exposures []exposure.Exposure
	for _, app := range apps {
		if !streamAllowed(app.Stream, s.cfg.StreamInclude, s.cfg.StreamExclude) {
			continue
		}
		script, err := s.scripts.LoadScript(ctx, app.ID)
		if err != nil {
			return nil, fmt.Errorf("qlik: script %s: %w", app.ID, err)
		}
		filtered := parse.FilterQlikScript(script, s.cfg.Datasource)
		if strings.TrimSpace(filtered) == "" {
			continue
		}
		tables := parse.Tables(filtered, s.parse)
		lookup := m.Refs(tables)
		if len(lookup.Found) == 0 && len(lookup.NotFound) == 0 {
			continue
		}
		exp, err := s.buildExposure(ctx, app, lookup, urlTmpl)
		if err != nil {
			return nil, err
		}
		exposures = append(exposures, exp)
	}
	if len(exposures) == 0 {
		return nil, nil
	}
	return []exposure.File{{Name: fileName, Exposures: exposures}}, nil
}

func (s *Source) ensureClients() error {
	if s.catalog != nil && s.scripts != nil {
		return nil
	}
	clients, err := newDefaultClients(s.cfg, s.http)
	if err != nil {
		return err
	}
	if s.catalog == nil {
		s.catalog = clients
	}
	if s.scripts == nil {
		s.scripts = clients
	}
	if s.tags == nil && s.cfg.QRC.Host != "" {
		s.tags = clients
	}
	return nil
}

func (s *Source) buildExposure(ctx context.Context, app App, lookup manifest.Lookup, urlTmpl *template.Template) (exposure.Exposure, error) {
	url, err := renderURL(urlTmpl, app.ID)
	if err != nil {
		return exposure.Exposure{}, err
	}
	desc := defaultDesc
	if len(lookup.NotFound) > 0 {
		desc += "\n" + notFoundPrefix + strings.Join(lookup.NotFound, ", ")
	}

	maturity := ""
	var tags []string
	if s.tags != nil {
		raw, err := s.tags.AppTags(ctx, app.ID)
		if err != nil {
			return exposure.Exposure{}, fmt.Errorf("qlik: tags %s: %w", app.ID, err)
		}
		maturity, tags = parse.Tags(raw)
	}

	prefix := s.pkg
	if prefix == "" {
		prefix = "qlik"
	}
	id := sanitizeID(app.ID)
	return exposure.Exposure{
		Name:        prefix + "_" + id,
		Type:        "dashboard",
		URL:         url,
		Label:       "QLIK: " + app.Name,
		Description: desc,
		OwnerName:   s.owner.Name,
		OwnerEmail:  s.owner.Email,
		DependsOn:   lookup.Found,
		Maturity:    maturity,
		Tags:        tags,
	}, nil
}

func renderURL(tmpl *template.Template, appID string) (string, error) {
	var buf strings.Builder
	if err := tmpl.Execute(&buf, struct{ AppID string }{AppID: appID}); err != nil {
		return "", fmt.Errorf("qlik: exposure_url_template: %w", err)
	}
	return buf.String(), nil
}

// streamAllowed: empty include = all; then apply exclude.
func streamAllowed(stream string, include, exclude []string) bool {
	if len(include) > 0 {
		ok := false
		for _, s := range include {
			if stream == s {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	for _, s := range exclude {
		if stream == s {
			return false
		}
	}
	return true
}

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

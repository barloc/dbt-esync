package gitscan

import (
	"context"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"

	"github.com/barloc/dbt-esync/app/config"
	"github.com/barloc/dbt-esync/app/exposure"
	"github.com/barloc/dbt-esync/app/manifest"
	"github.com/barloc/dbt-esync/app/parse"
)

const (
	fileName       = "gitscan"
	defaultDesc    = "Source file in git"
	notFoundPrefix = "Tables not in dbt: "
)

// Cloner prepares a readable working tree (injectable for tests).
type Cloner interface {
	Clone(ctx context.Context, cfg config.GitscanConfig) (root string, err error)
}

// Options configures a gitscan source.
type Options struct {
	Owner      config.DefaultOwner
	Parse      parse.Options
	FilterName string // manifest.filter_name — path_db_tokens gate
	Clone      Cloner // nil → wipe+clone into cfg.LocalPath
}

// Source collects exposures by scanning a git repo for SQL/scripts.
type Source struct {
	cfg        config.GitscanConfig
	owner      config.DefaultOwner
	parse      parse.Options
	filterName string
	clone      Cloner
}

// New returns a gitscan source.
func New(cfg config.GitscanConfig, opt Options) *Source {
	return &Source{
		cfg:        cfg,
		owner:      opt.Owner,
		parse:      opt.Parse,
		filterName: opt.FilterName,
		clone:      opt.Clone,
	}
}

// Collect implements source.Source.
func (s *Source) Collect(ctx context.Context, m *manifest.Finder) ([]exposure.File, error) {
	cloner := s.clone
	if cloner == nil {
		cloner = defaultCloner{}
	}
	root, err := cloner.Clone(ctx, s.cfg)
	if err != nil {
		return nil, err
	}

	ref, err := resolveRef(root)
	if err != nil {
		// Fixture trees without .git: browse URLs use "HEAD".
		ref = "HEAD"
	}

	exts := extSet(s.cfg.Extensions)
	tokens := lowerSlice(s.cfg.PathDBTokens)
	filter := strings.ToLower(s.filterName)

	var exposures []exposure.Exposure
	seen := map[string]struct{}{}

	roots := scanRoots(root, s.cfg.Dirs)
	for _, walkRoot := range roots {
		// Match Python Scanner: missing dirs are skipped, not fatal
		// (md/scripts layout may omit some configured folders).
		if _, err := os.Stat(walkRoot); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("gitscan: stat %s: %w", walkRoot, err)
		}
		err := filepath.WalkDir(walkRoot, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			if d.IsDir() {
				if d.Name() == ".git" {
					return fs.SkipDir
				}
				return nil
			}
			if !matchExt(d.Name(), exts) {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return fmt.Errorf("gitscan: rel %s: %w", path, err)
			}
			rel = filepath.ToSlash(rel)
			if !pathAllowed(rel, tokens, filter) {
				return nil
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("gitscan: read %s: %w", rel, err)
			}
			tables := parse.Tables(string(body), s.parse)
			lookup := m.Refs(tables)
			if len(lookup.Found) == 0 && len(lookup.NotFound) == 0 {
				return nil
			}
			exp, err := s.buildExposure(rel, ref, lookup)
			if err != nil {
				return err
			}
			if _, ok := seen[exp.Name]; ok {
				exp.Name = exp.Name + "_" + shortHash(rel)
			}
			seen[exp.Name] = struct{}{}
			exposures = append(exposures, exp)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	if len(exposures) == 0 {
		return nil, nil
	}
	return []exposure.File{{Name: fileName, Exposures: exposures}}, nil
}

func (s *Source) buildExposure(rel, ref string, lookup manifest.Lookup) (exposure.Exposure, error) {
	u, err := browseURL(s.cfg.VCS, s.cfg.RepoURL, ref, rel)
	if err != nil {
		return exposure.Exposure{}, err
	}
	desc := defaultDesc
	if s.owner.Slack != "" {
		desc = "Slack: " + s.owner.Slack
	}
	if len(lookup.NotFound) > 0 {
		desc += "\n" + notFoundPrefix + strings.Join(lookup.NotFound, ", ")
	}
	id := sanitizePathID(rel)
	return exposure.Exposure{
		Name:        "git_" + id,
		Type:        "analysis",
		URL:         u,
		Label:       "GIT: " + rel,
		Description: desc,
		OwnerName:   s.owner.Name,
		OwnerEmail:  s.owner.Email,
		DependsOn:   lookup.Found,
	}, nil
}

type defaultCloner struct{}

func (defaultCloner) Clone(ctx context.Context, cfg config.GitscanConfig) (string, error) {
	if err := prepareLocal(cfg.LocalPath); err != nil {
		return "", err
	}
	auth := &githttp.BasicAuth{Username: cfg.Login, Password: cfg.Token}
	_, err := git.PlainCloneContext(ctx, cfg.LocalPath, false, &git.CloneOptions{
		URL:          cfg.RepoURL,
		Auth:         auth,
		SingleBranch: true,
	})
	if err != nil {
		return "", fmt.Errorf("gitscan: clone: %w", err)
	}
	return cfg.LocalPath, nil
}

func prepareLocal(path string) error {
	cleaned := filepath.Clean(path)
	switch cleaned {
	case "", ".", "..", string(os.PathSeparator):
		return fmt.Errorf("gitscan: refusing to wipe %q", path)
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" && cleaned == filepath.Clean(home) {
		return fmt.Errorf("gitscan: refusing to wipe %q", path)
	}
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("gitscan: clear %s: %w", path, err)
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("gitscan: mkdir %s: %w", path, err)
	}
	return nil
}

func resolveRef(root string) (string, error) {
	repo, err := git.PlainOpen(root)
	if err != nil {
		return "", err
	}
	ref, err := repo.Head()
	if err != nil {
		return "", err
	}
	if ref.Name().IsBranch() {
		return ref.Name().Short(), nil
	}
	return ref.Hash().String(), nil
}

func scanRoots(root string, dirs []string) []string {
	if len(dirs) == 0 {
		return []string{root}
	}
	out := make([]string, 0, len(dirs))
	for _, d := range dirs {
		d = filepath.Clean(filepath.FromSlash(d))
		if d == "." || strings.HasPrefix(d, "..") {
			continue
		}
		out = append(out, filepath.Join(root, d))
	}
	if len(out) == 0 {
		return []string{root}
	}
	return out
}

func extSet(exts []string) map[string]struct{} {
	out := make(map[string]struct{}, len(exts))
	for _, e := range exts {
		e = strings.ToLower(strings.TrimSpace(e))
		if e == "" {
			continue
		}
		if !strings.HasPrefix(e, ".") {
			e = "." + e
		}
		out[e] = struct{}{}
	}
	return out
}

func matchExt(name string, exts map[string]struct{}) bool {
	if len(exts) == 0 {
		return false
	}
	_, ok := exts[strings.ToLower(filepath.Ext(name))]
	return ok
}

func lowerSlice(ss []string) []string {
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		s = strings.ToLower(strings.TrimSpace(s))
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// pathAllowed: empty tokens or empty filter → allow all.
// Paths with no token segment → allow (shared).
// Paths with any token segment → allow only if a segment equals filterName.
func pathAllowed(rel string, tokens []string, filterName string) bool {
	if len(tokens) == 0 || filterName == "" {
		return true
	}
	tokSet := make(map[string]struct{}, len(tokens))
	for _, t := range tokens {
		tokSet[t] = struct{}{}
	}
	parts := strings.Split(strings.ToLower(filepath.ToSlash(rel)), "/")
	hitAny, hitFilter := false, false
	for _, p := range parts {
		if _, ok := tokSet[p]; ok {
			hitAny = true
			if p == filterName {
				hitFilter = true
			}
		}
	}
	if !hitAny {
		return true
	}
	return hitFilter
}

func sanitizePathID(rel string) string {
	var b strings.Builder
	b.Grow(len(rel))
	for i := 0; i < len(rel); i++ {
		c := rel[i]
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

func shortHash(s string) string {
	h := uint32(2166136261)
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return fmt.Sprintf("%08x", h)
}

func browseURL(vcs, repoURL, ref, rel string) (string, error) {
	u, err := url.Parse(repoURL)
	if err != nil {
		return "", fmt.Errorf("gitscan: parse repo_url: %w", err)
	}
	base := u.Scheme + "://" + u.Host
	rel = strings.TrimPrefix(filepath.ToSlash(rel), "/")
	switch strings.ToLower(vcs) {
	case "gitlab":
		project, err := gitlabProjectPath(repoURL)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s/%s/-/blob/%s/%s", base, project, url.PathEscape(ref), escapePath(rel)), nil
	case "bitbucket":
		project, repo, err := bitbucketProjectRepo(repoURL)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s/projects/%s/repos/%s/browse/%s?at=refs%%2Fheads%%2F%s",
			base, project, repo, escapePath(rel), url.QueryEscape(ref)), nil
	default:
		return "", fmt.Errorf("gitscan: unsupported vcs %q", vcs)
	}
}

func escapePath(rel string) string {
	parts := strings.Split(rel, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}

func gitlabProjectPath(repoURL string) (string, error) {
	u, err := url.Parse(repoURL)
	if err != nil {
		return "", fmt.Errorf("gitscan: parse repo_url: %w", err)
	}
	path := strings.Trim(u.Path, "/")
	path = strings.TrimSuffix(path, ".git")
	if path == "" || !strings.Contains(path, "/") {
		return "", fmt.Errorf("gitscan: cannot derive gitlab project path from %q", repoURL)
	}
	return path, nil
}

func bitbucketProjectRepo(repoURL string) (project, repo string, err error) {
	u, err := url.Parse(repoURL)
	if err != nil {
		return "", "", fmt.Errorf("gitscan: parse repo_url: %w", err)
	}
	path := strings.Trim(u.Path, "/")
	path = strings.TrimSuffix(path, ".git")
	parts := strings.Split(path, "/")
	switch {
	case len(parts) >= 3 && parts[0] == "scm":
		return parts[1], parts[2], nil
	case len(parts) >= 2:
		return parts[len(parts)-2], parts[len(parts)-1], nil
	default:
		return "", "", fmt.Errorf("gitscan: cannot derive bitbucket project/repo from %q", repoURL)
	}
}

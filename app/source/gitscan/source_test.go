package gitscan_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/barloc/dbt-esync/app/config"
	"github.com/barloc/dbt-esync/app/manifest"
	"github.com/barloc/dbt-esync/app/parse"
	"github.com/barloc/dbt-esync/app/source/gitscan"
)

type fixedRoot struct{ root string }

func (f fixedRoot) Clone(context.Context, config.GitscanConfig) (string, error) {
	return f.root, nil
}

func TestUnitCollectFilterName(t *testing.T) {
	t.Parallel()

	src := newSource(t, gitscan.Options{
		Owner:      config.DefaultOwner{Name: "analytics", Email: "@a", Slack: "#analytics"},
		Parse:      parse.Options{ExcludedCatalogs: []string{"main"}},
		FilterName: "starrocks",
	})

	files, err := src.Collect(context.Background(), loadFinder(t))
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(files) != 1 || files[0].Name != "gitscan" {
		t.Fatalf("files: %+v", files)
	}
	names := map[string]bool{}
	for _, e := range files[0].Exposures {
		names[e.Name] = true
		if e.Type != "analysis" {
			t.Fatalf("type: %q", e.Type)
		}
		if e.OwnerName != "analytics" {
			t.Fatalf("owner: %+v", e)
		}
		if !strings.Contains(e.Description, "Slack: #analytics") {
			t.Fatalf("description: %q", e.Description)
		}
	}
	// starrocks/a.sql + shared/c.sql; not vertica/b.sql; not empty.sql
	if !names["git_analytics_starrocks_a_sql"] {
		t.Fatalf("missing starrocks file: %v", names)
	}
	if !names["git_analytics_shared_c_sql"] {
		t.Fatalf("missing shared file: %v", names)
	}
	if names["git_analytics_vertica_b_sql"] {
		t.Fatalf("vertica should be filtered out: %v", names)
	}
	if len(files[0].Exposures) != 2 {
		t.Fatalf("exposures: %+v", files[0].Exposures)
	}
}

func TestUnitCollectSkipNoTables(t *testing.T) {
	t.Parallel()

	// Only empty.sql under starrocks would match dirs, but we point dirs at a
	// tree that has only the empty file via FilterName allowing all + custom cfg.
	cfg := validCfg(t)
	cfg.Dirs = []string{"analytics/starrocks"}
	src := gitscan.New(cfg, gitscan.Options{
		Parse: parse.Options{ExcludedCatalogs: []string{"main"}},
		Clone: fixedRoot{root: fixtureRoot(t)},
		// No FilterName → pathAllowed allows all under dir; empty.sql skipped.
	})

	files, err := src.Collect(context.Background(), loadFinder(t))
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("files: %+v", files)
	}
	for _, e := range files[0].Exposures {
		if strings.Contains(e.Name, "empty") {
			t.Fatalf("empty.sql should be skipped: %+v", e)
		}
	}
}

func TestUnitCollectNotFoundSuffix(t *testing.T) {
	t.Parallel()

	cfg := validCfg(t)
	cfg.Dirs = []string{"analytics/vertica"}
	src := gitscan.New(cfg, gitscan.Options{
		Owner:      config.DefaultOwner{Name: "a", Email: "e"},
		Parse:      parse.Options{ExcludedCatalogs: []string{"main"}},
		FilterName: "vertica",
		Clone:      fixedRoot{root: fixtureRoot(t)},
	})

	files, err := src.Collect(context.Background(), loadFinder(t))
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(files) != 1 || len(files[0].Exposures) != 1 {
		t.Fatalf("files: %+v", files)
	}
	exp := files[0].Exposures[0]
	if !strings.Contains(exp.Description, "Tables not in dbt:") {
		t.Fatalf("description: %q", exp.Description)
	}
	if len(exp.DependsOn) != 0 {
		t.Fatalf("depends_on: %v", exp.DependsOn)
	}
}

func TestUnitCollectEmptyResult(t *testing.T) {
	t.Parallel()

	cfg := validCfg(t)
	cfg.Dirs = []string{"ignore"}
	cfg.Extensions = []string{".sql"}
	src := gitscan.New(cfg, gitscan.Options{
		Clone: fixedRoot{root: fixtureRoot(t)},
	})

	files, err := src.Collect(context.Background(), loadFinder(t))
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if files != nil {
		t.Fatalf("got %v, want nil", files)
	}
}

func TestUnitPathAllowed(t *testing.T) {
	t.Parallel()

	tokens := []string{"starrocks", "vertica"}
	cases := []struct {
		rel, filter string
		want        bool
	}{
		{"analytics/starrocks/a.sql", "starrocks", true},
		{"analytics/vertica/b.sql", "starrocks", false},
		{"analytics/shared/c.sql", "starrocks", true},
		{"analytics/shared/c.sql", "", true},
		{"starrocks.sql", "starrocks", true}, // basename segment ≠ token
	}
	for _, tc := range cases {
		got := gitscan.PathAllowedForTest(tc.rel, tokens, tc.filter)
		if got != tc.want {
			t.Fatalf("%s filter=%q: got %v want %v", tc.rel, tc.filter, got, tc.want)
		}
	}
}

func TestUnitBrowseURL(t *testing.T) {
	t.Parallel()

	gl, err := gitscan.BrowseURLForTest("gitlab", "https://git.example.com/org/sub/analytics.git", "master", "analytics/a.sql")
	if err != nil {
		t.Fatalf("gitlab: %v", err)
	}
	wantGL := "https://git.example.com/org/sub/analytics/-/blob/master/analytics/a.sql"
	if gl != wantGL {
		t.Fatalf("gitlab: got %q want %q", gl, wantGL)
	}

	bb, err := gitscan.BrowseURLForTest("bitbucket", "https://git.example.com/scm/AN/analytics.git", "master", "analytics/a.sql")
	if err != nil {
		t.Fatalf("bitbucket: %v", err)
	}
	wantBB := "https://git.example.com/projects/AN/repos/analytics/browse/analytics/a.sql?at=refs%2Fheads%2Fmaster"
	if bb != wantBB {
		t.Fatalf("bitbucket: got %q want %q", bb, wantBB)
	}
}

func TestUnitSanitizePathID(t *testing.T) {
	t.Parallel()
	got := gitscan.SanitizePathIDForTest("analytics/starrocks/foo.sql")
	if got != "analytics_starrocks_foo_sql" {
		t.Fatalf("got %q", got)
	}
}

func TestUnitBrowseURLBadVCS(t *testing.T) {
	t.Parallel()
	_, err := gitscan.BrowseURLForTest("github", "https://git.example.com/a/b.git", "main", "f.sql")
	if err == nil || !strings.Contains(err.Error(), "unsupported vcs") {
		t.Fatalf("got %v", err)
	}
}

func newSource(t *testing.T, opt gitscan.Options) *gitscan.Source {
	t.Helper()
	opt.Clone = fixedRoot{root: fixtureRoot(t)}
	return gitscan.New(validCfg(t), opt)
}

func validCfg(t *testing.T) config.GitscanConfig {
	t.Helper()
	return config.GitscanConfig{
		RepoURL:      "https://git.example.com/org/analytics.git",
		LocalPath:    "/tmp/gitscan-test",
		Dirs:         []string{"analytics"},
		Extensions:   []string{".sql"},
		PathDBTokens: []string{"starrocks", "vertica"},
		VCS:          "gitlab",
	}
}

func fixtureRoot(t *testing.T) string {
	t.Helper()
	return filepath.Join("testdata", "repo")
}

func loadFinder(t *testing.T) *manifest.Finder {
	t.Helper()
	path := filepath.Join("..", "..", "manifest", "testdata", "manifest.mini.json")
	f, err := manifest.Load(context.Background(), "", path, []string{"main"})
	if err != nil {
		t.Fatalf("manifest.Load: %v", err)
	}
	return f
}

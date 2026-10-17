package git_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	appconfig "github.com/barloc/dbt-esync/app/config"
	"github.com/barloc/dbt-esync/app/exposure"
	sinkgit "github.com/barloc/dbt-esync/app/sink/git"
)

func TestUnitSyncPushThenNoop(t *testing.T) {
	remote := initBareRemote(t)
	pkgs := []sinkgit.Package{{Name: "bi_qlik", Files: sampleFiles()}}

	cfg := appconfig.GitSink{
		RepoURL:       remote,
		User:          "bot",
		Token:         "tok",
		LocalPath:     filepath.Join(t.TempDir(), "clone1"),
		ExposuresPath: "dbt/models/exposures",
		Branch:        "feature/sync",
		BaseBranch:    "master",
	}

	pushed, err := sinkgit.Sync(context.Background(), cfg, "master", pkgs)
	if err != nil {
		t.Fatalf("first Sync: %v", err)
	}
	if !pushed {
		t.Fatal("first Sync: expected push")
	}

	gotPath := filepath.Join(cfg.LocalPath, "dbt/models/exposures/bi_qlik/dash.yml")
	got, err := os.ReadFile(gotPath)
	if err != nil {
		t.Fatalf("read written yaml: %v", err)
	}
	want, err := pkgs[0].Files[0].Encode()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("yaml mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}

	// Simulate merged MR: tip of feature/sync becomes master.
	mergeFeatureIntoMaster(t, remote, "feature/sync", "master")

	cfg.LocalPath = filepath.Join(t.TempDir(), "clone2")
	pushed, err = sinkgit.Sync(context.Background(), cfg, "master", pkgs)
	if err != nil {
		t.Fatalf("second Sync: %v", err)
	}
	if pushed {
		t.Fatal("second Sync: expected no push")
	}
}

func TestUnitSyncMultiplePackages(t *testing.T) {
	remote := initBareRemote(t)
	pkgs := []sinkgit.Package{
		{Name: "bi_qlik", Files: sampleFiles()},
		{Name: "bi_redash", Files: []exposure.File{{
			Name: "alert",
			Exposures: []exposure.Exposure{{
				Name:       "exp_b",
				Type:       "analysis",
				URL:        "https://example.com/b",
				Label:      "B",
				OwnerName:  "analytics",
				OwnerEmail: "@analytics",
				DependsOn:  []string{"ref('n')"},
			}},
		}}},
	}

	cfg := appconfig.GitSink{
		RepoURL:       remote,
		User:          "bot",
		Token:         "tok",
		LocalPath:     filepath.Join(t.TempDir(), "clone"),
		ExposuresPath: "dbt/models/exposures",
		Branch:        "feature/sync",
		BaseBranch:    "master",
	}

	pushed, err := sinkgit.Sync(context.Background(), cfg, "master", pkgs)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if !pushed {
		t.Fatal("expected push")
	}
	for _, name := range []string{"bi_qlik/dash.yml", "bi_redash/alert.yml"} {
		path := filepath.Join(cfg.LocalPath, "dbt/models/exposures", name)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
}

func TestUnitSyncRejectsEmptyPackages(t *testing.T) {
	t.Parallel()
	_, err := sinkgit.Sync(context.Background(), appconfig.GitSink{}, "master", nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestUnitSyncRejectsEmptyPackageName(t *testing.T) {
	t.Parallel()
	_, err := sinkgit.Sync(context.Background(), appconfig.GitSink{}, "master", []sinkgit.Package{{Name: ""}})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestUnitSyncRejectsDangerousLocalPath(t *testing.T) {
	t.Parallel()
	_, err := sinkgit.Sync(context.Background(), appconfig.GitSink{
		RepoURL: "file:///tmp/x", LocalPath: ".", User: "u", Token: "t",
		ExposuresPath: "e", Branch: "b", BaseBranch: "master",
	}, "master", []sinkgit.Package{{Name: "pkg"}})
	if err == nil || !strings.Contains(err.Error(), "refusing to wipe") {
		t.Fatalf("got %v", err)
	}
}

func TestUnitSyncNonDefaultBaseBranch(t *testing.T) {
	remote := initBareRemote(t)
	addRemoteBranch(t, remote, "develop", "master")

	cfg := appconfig.GitSink{
		RepoURL:       remote,
		User:          "bot",
		Token:         "tok",
		LocalPath:     filepath.Join(t.TempDir(), "clone"),
		ExposuresPath: "dbt/models/exposures",
		Branch:        "feature/from-develop",
		BaseBranch:    "develop",
	}
	pushed, err := sinkgit.Sync(context.Background(), cfg, "develop", []sinkgit.Package{{Name: "bi_qlik", Files: sampleFiles()}})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if !pushed {
		t.Fatal("expected push from non-default base")
	}
}

func sampleFiles() []exposure.File {
	return []exposure.File{{
		Name: "dash",
		Exposures: []exposure.Exposure{{
			Name:       "exp_a",
			Type:       "dashboard",
			URL:        "https://example.com/a",
			Label:      "A",
			OwnerName:  "analytics",
			OwnerEmail: "@analytics",
			DependsOn:  []string{"ref('m')"},
		}},
	}}
}

func initBareRemote(t *testing.T) string {
	t.Helper()
	seed := filepath.Join(t.TempDir(), "seed")
	repo, err := gogit.PlainInit(seed, false)
	if err != nil {
		t.Fatalf("PlainInit seed: %v", err)
	}
	w, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("seed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Add("README.md"); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Commit("seed", &gogit.CommitOptions{
		Author: &object.Signature{Name: "seed", Email: "seed@localhost", When: time.Now()},
	}); err != nil {
		t.Fatal(err)
	}

	bare := filepath.Join(t.TempDir(), "remote.git")
	if _, err := gogit.PlainInit(bare, true); err != nil {
		t.Fatalf("PlainInit bare: %v", err)
	}
	if _, err := repo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{bare},
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Push(&gogit.PushOptions{}); err != nil {
		t.Fatalf("push seed: %v", err)
	}
	return bare
}

func mergeFeatureIntoMaster(t *testing.T, bare, feature, master string) {
	t.Helper()
	addRemoteBranch(t, bare, master, feature)
}

func addRemoteBranch(t *testing.T, bare, newBranch, fromBranch string) {
	t.Helper()
	repo, err := gogit.PlainOpen(bare)
	if err != nil {
		t.Fatal(err)
	}
	fromRef, err := repo.Reference(plumbing.NewBranchReferenceName(fromBranch), true)
	if err != nil {
		t.Fatalf("%s ref: %v", fromBranch, err)
	}
	ref := plumbing.NewHashReference(plumbing.NewBranchReferenceName(newBranch), fromRef.Hash())
	if err := repo.Storer.SetReference(ref); err != nil {
		t.Fatalf("set %s: %v", newBranch, err)
	}
}

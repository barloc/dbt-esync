package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"

	"github.com/barloc/dbt-esync/app/config"
	"github.com/barloc/dbt-esync/app/exposure"
)

const (
	commitName  = "dbt-esync"
	commitEmail = "dbt-esync@localhost"
)

// Package is one exposures package written under exposures_path/<Name>.
type Package struct {
	Name  string
	Files []exposure.File
}

// Sync clones cfg.LocalPath, replaces each package under exposures_path,
// commits on cfg.Branch from baseBranch, and pushes. pushed is false if there is no diff.
func Sync(ctx context.Context, cfg config.GitSink, baseBranch string, packages []Package) (pushed bool, err error) {
	if len(packages) == 0 {
		return false, fmt.Errorf("sink/git: at least one package is required")
	}
	names := make([]string, 0, len(packages))
	for i := range packages {
		if packages[i].Name == "" {
			return false, fmt.Errorf("sink/git: package name is required")
		}
		names = append(names, packages[i].Name)
	}
	if baseBranch == "" {
		return false, fmt.Errorf("sink/git: base branch is required")
	}
	if err := prepareLocal(cfg.LocalPath); err != nil {
		return false, err
	}

	auth := &githttp.BasicAuth{Username: cfg.User, Password: cfg.Token}
	baseRef := plumbing.NewBranchReferenceName(baseBranch)
	repo, err := git.PlainCloneContext(ctx, cfg.LocalPath, false, &git.CloneOptions{
		URL:           cfg.RepoURL,
		Auth:          auth,
		ReferenceName: baseRef,
		SingleBranch:  true,
	})
	if err != nil {
		return false, fmt.Errorf("sink/git: clone: %w", err)
	}

	w, err := repo.Worktree()
	if err != nil {
		return false, fmt.Errorf("sink/git: worktree: %w", err)
	}

	// Clone already checked out baseBranch; create the feature branch from HEAD.
	if err := w.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(cfg.Branch),
		Create: true,
	}); err != nil {
		return false, fmt.Errorf("sink/git: create branch %s: %w", cfg.Branch, err)
	}

	for i := range packages {
		pkgRel := filepath.ToSlash(filepath.Join(cfg.ExposuresPath, packages[i].Name))
		pkgAbs := filepath.Join(cfg.LocalPath, filepath.FromSlash(pkgRel))
		if err := replacePackage(w, cfg.LocalPath, pkgRel, pkgAbs, packages[i].Files); err != nil {
			return false, err
		}
	}

	status, err := w.Status()
	if err != nil {
		return false, fmt.Errorf("sink/git: status: %w", err)
	}
	if status.IsClean() {
		return false, nil
	}

	msg := fmt.Sprintf("dbt-esync: update %s", strings.Join(names, ", "))
	if _, err := w.Commit(msg, &git.CommitOptions{
		Author: &object.Signature{
			Name:  commitName,
			Email: commitEmail,
			When:  time.Now(),
		},
	}); err != nil {
		return false, fmt.Errorf("sink/git: commit: %w", err)
	}

	refSpec := gitconfig.RefSpec(fmt.Sprintf("refs/heads/%s:refs/heads/%s", cfg.Branch, cfg.Branch))
	if err := repo.PushContext(ctx, &git.PushOptions{
		Auth:     auth,
		RefSpecs: []gitconfig.RefSpec{refSpec},
	}); err != nil {
		return false, fmt.Errorf("sink/git: push: %w", err)
	}
	return true, nil
}

func prepareLocal(path string) error {
	cleaned := filepath.Clean(path)
	switch cleaned {
	case "", ".", "..", string(os.PathSeparator):
		return fmt.Errorf("sink/git: refusing to wipe %q", path)
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" && cleaned == filepath.Clean(home) {
		return fmt.Errorf("sink/git: refusing to wipe %q", path)
	}
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("sink/git: clear %s: %w", path, err)
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("sink/git: mkdir %s: %w", path, err)
	}
	return nil
}

func replacePackage(w *git.Worktree, localPath, pkgRel, pkgAbs string, files []exposure.File) error {
	if err := removePackage(w, localPath, pkgAbs); err != nil {
		return err
	}
	if err := os.MkdirAll(pkgAbs, 0o755); err != nil {
		return fmt.Errorf("sink/git: mkdir package: %w", err)
	}
	for i := range files {
		if _, err := files[i].Write(pkgAbs); err != nil {
			return err
		}
	}
	if _, err := w.Add(pkgRel); err != nil {
		return fmt.Errorf("sink/git: add %s: %w", pkgRel, err)
	}
	return nil
}

func removePackage(w *git.Worktree, localPath, pkgAbs string) error {
	info, err := os.Stat(pkgAbs)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("sink/git: stat package: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("sink/git: package path is not a directory: %s", pkgAbs)
	}

	err = filepath.Walk(pkgAbs, func(path string, fi os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if fi.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(localPath, path)
		if err != nil {
			return err
		}
		if _, err := w.Remove(filepath.ToSlash(rel)); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("sink/git: untrack package: %w", err)
	}
	if err := os.RemoveAll(pkgAbs); err != nil {
		return fmt.Errorf("sink/git: remove package: %w", err)
	}
	return nil
}

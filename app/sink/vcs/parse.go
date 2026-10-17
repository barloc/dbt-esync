package vcs

import (
	"fmt"
	"net/url"
	"strings"
)

// gitlabProjectPath returns "group/subgroup/repo" from a clone URL.
func gitlabProjectPath(repoURL string) (string, error) {
	u, err := url.Parse(repoURL)
	if err != nil {
		return "", fmt.Errorf("sink/vcs: parse repo_url: %w", err)
	}
	path := strings.Trim(u.Path, "/")
	path = strings.TrimSuffix(path, ".git")
	if path == "" || !strings.Contains(path, "/") {
		return "", fmt.Errorf("sink/vcs: cannot derive gitlab project path from %q", repoURL)
	}
	return path, nil
}

// bitbucketProjectRepo returns project key and repository slug.
// Supports /scm/{project}/{repo}.git (Bitbucket Server) and /{project}/{repo}.git.
func bitbucketProjectRepo(repoURL string) (project, repo string, err error) {
	u, err := url.Parse(repoURL)
	if err != nil {
		return "", "", fmt.Errorf("sink/vcs: parse repo_url: %w", err)
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
		return "", "", fmt.Errorf("sink/vcs: cannot derive bitbucket project/repo from %q", repoURL)
	}
}

package vcs

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/barloc/dbt-esync/app/config"
)

// Open creates a GitLab MR or Bitbucket Server PR for the sink branch.
// An already-open MR/PR for the same branches is treated as success.
func Open(ctx context.Context, mr config.MRSink, git config.GitSink, title string) error {
	if !mr.Enabled {
		return nil
	}
	host := strings.TrimRight(mr.Host, "/")
	fullTitle := fmt.Sprintf("DBT exposures update - %s", title)
	switch mr.Provider {
	case "gitlab":
		return openGitLab(ctx, host, git, fullTitle)
	case "bitbucket":
		return openBitbucket(ctx, host, git, fullTitle)
	default:
		return fmt.Errorf("sink/vcs: unknown provider %q", mr.Provider)
	}
}

func openGitLab(ctx context.Context, host string, git config.GitSink, title string) error {
	project, err := gitlabProjectPath(git.RepoURL)
	if err != nil {
		return err
	}
	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests", host, url.PathEscape(project))
	body := map[string]any{
		"source_branch":        git.Branch,
		"target_branch":        git.BaseBranch,
		"title":                title,
		"squash":               true,
		"remove_source_branch": true,
	}
	hdr := http.Header{"PRIVATE-TOKEN": {git.Token}}
	code, respBody, err := postJSON(ctx, endpoint, hdr, body)
	if err != nil {
		return err
	}
	return acceptCreateOrExists(code, respBody, "gitlab MR")
}

func openBitbucket(ctx context.Context, host string, git config.GitSink, title string) error {
	project, repo, err := bitbucketProjectRepo(git.RepoURL)
	if err != nil {
		return err
	}
	endpoint := fmt.Sprintf("%s/rest/api/1.0/projects/%s/repos/%s/pull-requests",
		host, url.PathEscape(project), url.PathEscape(repo))
	body := map[string]any{
		"title":       title,
		"description": title,
		"fromRef": map[string]any{
			"id": "refs/heads/" + git.Branch,
		},
		"toRef": map[string]any{
			"id": "refs/heads/" + git.BaseBranch,
		},
	}
	auth := base64.StdEncoding.EncodeToString([]byte(git.User + ":" + git.Token))
	hdr := http.Header{"Authorization": {"Basic " + auth}}
	code, respBody, err := postJSON(ctx, endpoint, hdr, body)
	if err != nil {
		return err
	}
	return acceptCreateOrExists(code, respBody, "bitbucket PR")
}

func postJSON(ctx context.Context, endpoint string, hdr http.Header, payload any) (int, string, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return 0, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, vals := range hdr {
		for _, v := range vals {
			req.Header.Set(k, v)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, "", fmt.Errorf("sink/vcs: request: %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return resp.StatusCode, string(b), nil
}

func acceptCreateOrExists(code int, body, label string) error {
	switch {
	case code == http.StatusCreated || code == http.StatusOK:
		return nil
	case alreadyExists(body):
		// Only when the response text says the MR/PR already exists — not every 409.
		return nil
	default:
		return fmt.Errorf("sink/vcs: %s: HTTP %d: %s", label, code, truncate(body))
	}
}

func alreadyExists(body string) bool {
	l := strings.ToLower(body)
	return strings.Contains(l, "already exists") ||
		strings.Contains(l, "open merge request already exists") ||
		strings.Contains(l, "pull request already exists")
}

func truncate(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 300 {
		return s[:300] + "…"
	}
	return s
}

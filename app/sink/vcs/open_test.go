package vcs_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/barloc/dbt-esync/app/config"
	"github.com/barloc/dbt-esync/app/sink/vcs"
)

func TestUnitOpenGitLab(t *testing.T) {
	var gotPath string
	var gotToken string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotToken = r.Header.Get("PRIVATE-TOKEN")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"iid":1}`))
	}))
	t.Cleanup(srv.Close)

	err := vcs.Open(context.Background(), config.MRSink{
		Enabled:  true,
		Provider: "gitlab",
		Host:     srv.URL,
	}, config.GitSink{
		RepoURL:    "https://git.example.com/group/sub/dbt.git",
		User:       "bot",
		Token:      "gl-token",
		Branch:     "feature/sync",
		BaseBranch: "master",
	}, "bi_qlik")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if gotToken != "gl-token" {
		t.Fatalf("token: %q", gotToken)
	}
	if !strings.Contains(gotPath, "/api/v4/projects/") || !strings.HasSuffix(gotPath, "/merge_requests") {
		t.Fatalf("path: %q", gotPath)
	}
	if gotBody["source_branch"] != "feature/sync" || gotBody["target_branch"] != "master" {
		t.Fatalf("body: %#v", gotBody)
	}
	if title, _ := gotBody["title"].(string); !strings.Contains(title, "bi_qlik") {
		t.Fatalf("title: %#v", gotBody["title"])
	}
}

func TestUnitOpenGitLabAlreadyExists(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":["Another open merge request already exists for this source branch"]}`))
	}))
	t.Cleanup(srv.Close)

	err := vcs.Open(context.Background(), config.MRSink{
		Enabled:  true,
		Provider: "gitlab",
		Host:     srv.URL,
	}, config.GitSink{
		RepoURL:    "https://git.example.com/org/dbt.git",
		Token:      "t",
		Branch:     "feature/sync",
		BaseBranch: "master",
	}, "pkg")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
}

func TestUnitOpenGitLabConflictWithoutAlreadyExists(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":["branch name conflict"]}`))
	}))
	t.Cleanup(srv.Close)

	err := vcs.Open(context.Background(), config.MRSink{
		Enabled:  true,
		Provider: "gitlab",
		Host:     srv.URL,
	}, config.GitSink{
		RepoURL:    "https://git.example.com/org/dbt.git",
		Token:      "t",
		Branch:     "feature/sync",
		BaseBranch: "master",
	}, "pkg")
	if err == nil {
		t.Fatal("expected error for unrelated 409")
	}
}

func TestUnitOpenBitbucketAlreadyExists(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"errors":[{"message":"A pull request already exists for these refs"}]}`))
	}))
	t.Cleanup(srv.Close)

	err := vcs.Open(context.Background(), config.MRSink{
		Enabled:  true,
		Provider: "bitbucket",
		Host:     srv.URL,
	}, config.GitSink{
		RepoURL:    "https://bitbucket.example.com/scm/DP/dbt.git",
		User:       "bot",
		Token:      "t",
		Branch:     "feature/sync",
		BaseBranch: "master",
	}, "pkg")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
}

func TestUnitOpenBitbucket(t *testing.T) {
	var gotPath string
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":1}`))
	}))
	t.Cleanup(srv.Close)

	err := vcs.Open(context.Background(), config.MRSink{
		Enabled:  true,
		Provider: "bitbucket",
		Host:     srv.URL,
	}, config.GitSink{
		RepoURL:    "https://bitbucket.example.com/scm/DP/vertica-dbt.git",
		User:       "bot",
		Token:      "bb-token",
		Branch:     "feature/sync",
		BaseBranch: "master",
	}, "bi_qlik")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !strings.HasPrefix(gotAuth, "Basic ") {
		t.Fatalf("auth: %q", gotAuth)
	}
	want := "/rest/api/1.0/projects/DP/repos/vertica-dbt/pull-requests"
	if gotPath != want {
		t.Fatalf("path: got %q want %q", gotPath, want)
	}
}

func TestUnitOpenDisabled(t *testing.T) {
	t.Parallel()
	err := vcs.Open(context.Background(), config.MRSink{}, config.GitSink{}, "x")
	if err != nil {
		t.Fatal(err)
	}
}

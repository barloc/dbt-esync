package vcs

import (
	"testing"
)

func TestUnitGitlabProjectPath(t *testing.T) {
	t.Parallel()
	path, err := gitlabProjectPath("https://git.example.com/group/sub/dbt.git")
	if err != nil {
		t.Fatal(err)
	}
	if path != "group/sub/dbt" {
		t.Fatalf("got %q", path)
	}
	if _, err := gitlabProjectPath("https://git.example.com/onlyrepo.git"); err == nil {
		t.Fatal("expected error for single segment")
	}
}

func TestUnitBitbucketProjectRepo(t *testing.T) {
	t.Parallel()

	proj, repo, err := bitbucketProjectRepo("https://bb.example.com/scm/DP/vertica-dbt.git")
	if err != nil {
		t.Fatal(err)
	}
	if proj != "DP" || repo != "vertica-dbt" {
		t.Fatalf("scm: got %q %q", proj, repo)
	}

	proj, repo, err = bitbucketProjectRepo("https://bb.example.com/ORG/my-dbt.git")
	if err != nil {
		t.Fatal(err)
	}
	if proj != "ORG" || repo != "my-dbt" {
		t.Fatalf("path: got %q %q", proj, repo)
	}

	if _, _, err := bitbucketProjectRepo("https://bb.example.com/only.git"); err == nil {
		t.Fatal("expected error for single segment")
	}
}

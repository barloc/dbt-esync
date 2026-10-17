package config_test

import (
	"strings"
	"testing"

	"github.com/barloc/dbt-esync/app/config"
)

func TestUnitEnvOverridesWinOverYAML(t *testing.T) {
	// No t.Parallel: Setenv + Load must not race other Load tests.
	t.Setenv("DBT_ESYNC_SINK__GIT__TOKEN", "from-env")
	t.Setenv("DBT_ESYNC_SOURCE__0__QLIK__PASSWORD", "secret")
	t.Setenv("DBT_ESYNC_SOURCE__0__QLIK__STREAM_EXCLUDE", "A, B")
	t.Setenv("DBT_ESYNC_SINK__MR__ENABLED", "false")
	t.Setenv("DBT_ESYNC_DEFAULT_OWNER__SLACK", "#ops")

	cfg, err := config.Load(writeTempYAML(t, validQlikYAML()+`
  mr:
    enabled: true
    provider: gitlab
`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if cfg.Sink.Git.Token != "from-env" {
		t.Fatalf("token: got %q", cfg.Sink.Git.Token)
	}
	if cfg.Source[0].Qlik.Password != "secret" {
		t.Fatalf("password: got %q", cfg.Source[0].Qlik.Password)
	}
	if got := cfg.Source[0].Qlik.StreamExclude; len(got) != 2 || got[0] != "A" || got[1] != "B" {
		t.Fatalf("stream_exclude: %v", got)
	}
	if cfg.Sink.MR.Enabled {
		t.Fatal("mr.enabled should be false")
	}
	if cfg.DefaultOwner.Slack != "#ops" {
		t.Fatalf("slack: got %q", cfg.DefaultOwner.Slack)
	}
}

func TestUnitEnvOverrideUnknownPath(t *testing.T) {
	t.Setenv("DBT_ESYNC_NOPE__FIELD", "x")
	_, err := config.Load(writeTempYAML(t, validQlikYAML()))
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("got %v", err)
	}
}

func TestUnitEnvBoolRejectsParseBoolExtras(t *testing.T) {
	// strconv.ParseBool would accept "t"; we only allow true/false/1/0.
	t.Setenv("DBT_ESYNC_SINK__MR__ENABLED", "t")
	_, err := config.Load(writeTempYAML(t, validQlikYAML()))
	if err == nil || !strings.Contains(err.Error(), "true/false/1/0") {
		t.Fatalf("got %v", err)
	}
}

func TestUnitEnvAllocatesNilSourceBlock(t *testing.T) {
	t.Setenv("DBT_ESYNC_SOURCE__0__TYPE", "mysql")
	t.Setenv("DBT_ESYNC_SOURCE__0__NAME", "pkg")
	t.Setenv("DBT_ESYNC_SOURCE__0__MYSQL__HOST", "mysql.example.com")
	t.Setenv("DBT_ESYNC_MANIFEST__FILE", "m.json")
	t.Setenv("DBT_ESYNC_SINK__GIT__REPO_URL", "https://git.example.com/r.git")
	t.Setenv("DBT_ESYNC_SINK__GIT__EXPOSURES_PATH", "e")
	t.Setenv("DBT_ESYNC_SINK__GIT__USER", "bot")
	t.Setenv("DBT_ESYNC_SINK__GIT__TOKEN", "tok")
	t.Setenv("DBT_ESYNC_SINK__GIT__LOCAL_PATH", "/tmp/dbt_repo")
	t.Setenv("DBT_ESYNC_SINK__GIT__BRANCH", "feature/sync")
	t.Setenv("DBT_ESYNC_SINK__GIT__BASE_BRANCH", "master")

	cfg, err := config.Load(writeTempYAML(t, `
source:
  - name: unused
    type: qlik
manifest:
  file: unused.json
`+validSinkYAML()))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if cfg.Source[0].Type != "mysql" {
		t.Fatalf("type: %q", cfg.Source[0].Type)
	}
	if cfg.Source[0].MySQL == nil || cfg.Source[0].MySQL.Host != "mysql.example.com" {
		t.Fatalf("mysql: %+v", cfg.Source[0].MySQL)
	}
}

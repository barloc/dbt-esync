# dbt-esync

Sync dbt exposures from BI tools and code repos into your dbt project.

[![All Contributors](https://img.shields.io/badge/all_contributors-2-orange.svg?style=flat-square)](#contributors)

Go CLI / CronJob with YAML `--config`. One config = N named sources + one sink.

## Build & run

Build the binary and run with the example config:

```bash
make build
make run                          # uses config.example.yaml
# or
go run . --config config.example.yaml
```

What a run does:

1. Load and validate the YAML config (env overrides applied).
2. Load `manifest.json` (URL or file).
3. Collect each `source[]` entry (best-effort: one failure does not skip the rest).
4. Write successful non-empty packages into the sink repo in one commit and optionally open an MR/PR.
5. Exit non-zero if any Collect failed.

Checks and image:

```bash
make test
make lint
make docker-build
```

## Docker Hub

```bash
docker pull barloc/dbt-esync:v1.0.0
docker run --rm barloc/dbt-esync:v1.0.0 --help

docker login
make docker-push                 # barloc/dbt-esync:latest
make docker-push VERSION=v1.0.0  # also tags VERSION
```

Image: [`barloc/dbt-esync`](https://hub.docker.com/r/barloc/dbt-esync).

## Configuration

Pass `--config` / `-c` (default `config.yaml`). See [`config.example.yaml`](config.example.yaml) for a complete public example.

### Environment overrides

After the YAML file is loaded, any `DBT_ESYNC_*` environment variable overrides matching fields. **Env wins over the file.**

| Rule | Detail |
|------|--------|
| Prefix | `DBT_ESYNC_` |
| Nesting | `__` between YAML keys (lowercased) |
| Strings | as-is |
| Bool | `true` / `false` / `1` / `0` (case-insensitive; not `t`/`yes`) |
| Int | decimal |
| String lists | comma-separated (`A, B` → `["A","B"]`) |
| Unknown path | Load fails |
| Nil nested blocks | allocated when an override targets them (e.g. `SOURCE__0__MYSQL__HOST`) |
| Array index | numeric segment after the list key (`SOURCE__0__…` → `source[0]`) |

Examples:

```bash
export DBT_ESYNC_SINK__GIT__TOKEN=...
export DBT_ESYNC_SOURCE__0__QLIK__PASSWORD=...
export DBT_ESYNC_SOURCE__0__QLIK__STREAM_EXCLUDE="99-Archive,92-TMP"
export DBT_ESYNC_SINK__MR__ENABLED=false
export DBT_ESYNC_DEFAULT_OWNER__SLACK="#analytics"
```

CronJobs typically mount a base YAML and inject secrets via these env vars (Vault / K8s secrets).

### Common

| Option | Meaning |
|--------|---------|
| `default_owner.name` | Owner name when the source has none |
| `default_owner.email` | Owner email fallback |
| `default_owner.slack` | Optional Slack handle/channel (e.g. gitscan description) |
| `manifest.url` \| `manifest.file` | `manifest.json` — **exactly one** |
| `manifest.filter_name` | Warehouse/engine token for filters (`starrocks`, `vertica`, `bigquery`, …) |

Owners are configured **only** under `default_owner`. Sources may supply an owner from their API; otherwise this fallback is used. There are no per-source owner overrides.

Package [`app/manifest`](app/manifest): `Load(ctx, url, file, excludedCatalogs)` → `Finder.Refs(tables)` → `{Found, NotFound}` (`ref('…')` / `source('…', '…')`). Catalog names in `parse.excluded_catalogs` are omitted from lookup keys (same list as parse). `filter_name` is for sources only.

Package [`app/exposure`](app/exposure): `File{Name, Exposures}` → `Encode()` / `Write(dir)` — dbt exposures YAML `version: 2`, sorted by name; empty maturity/tags omitted; multiline descriptions use `|`.

### Parse

| Option | Meaning |
|--------|---------|
| `parse.excluded_catalogs` | Drop catalog segment from 3-part refs; also shapes manifest lookup keys |
| `parse.schema_allowlist` | Optional allowlist for valid schemas (no corp names in public defaults) |

Package [`app/parse`](app/parse): `FilterQlikScript` (LIB CONNECT + SUB/CALL), `Tables` (FROM/JOIN / INLINE / `$(var)`), `Tags` (maturity vs labels). Empty `schema_allowlist` uses soft structural heuristics (`main` / `marts` / `core` plus length/underscore rules).

### Sources

`source` is a **non-empty array**. Each entry:

| Field | Meaning |
|-------|---------|
| `name` | Package directory under `sink.git.exposures_path` (required; unique; no `/`, `\`, or `..`) |
| `type` | Collector type (required) |
| typed block | Matching nested options (`qlik`, `redash`, …) |

Collect runs every entry (best-effort). Successful packages are written in one sink commit; any Collect failure still exits non-zero after sync.

| `type` | Package | Description |
|--------|---------|-------------|
| `growthbook` | `app/source/growthbook` | GrowthBook datasources and assignment SQL via REST |
| `mysql` | `app/source/mysql` | SQL catalog in MySQL (`SELECT` texts + metadata) |
| `redash` | `app/source/redash` | Redash dashboards and alerts via REST |
| `hive` | `app/source/hive` | SQL catalog in Hive (`SELECT` script texts + owners) |
| `gitscan` | `app/source/gitscan` | Read-only clone of a repo; scan files under `dirs` / `extensions` |
| `qlik` | `app/source/qlik` | Qlik Sense Enterprise apps: QRS list + WSS proxy load scripts |

#### GrowthBook (`growthbook` block)

Reads datasources and assignment queries from the GrowthBook API (API host, not the UI).

| Option | Filter / meaning |
|--------|------------------|
| `host` | API host (not the UI) |
| `api_key` | Bearer token |
| `filter_source` | Datasource type (= `manifest.filter_name`) |
| `exposure_url` | UI template |

#### MySQL SQL catalog (`mysql` block)

Reads exposure SQL from a catalog table (or custom `query`) over MySQL. Same YAML shape as `hive`; `type` selects the driver only.

| Option | Filter / meaning |
|--------|------------------|
| `host` | Server host |
| `port` | Server port (optional) |
| `user` | DB user |
| `password` | DB password |
| `database` | Database / schema |
| `query` | Full SELECT (alternative to table + columns) |
| `table` | Catalog table when not using `query` |
| `id_column` | Row id column |
| `sql_column` | SQL / script text column |
| `description_column` | Optional description column |
| `extra_filters` | Extra SQL `WHERE` |
| `exposure_prefix` | Exposure name prefix |
| `exposure_url_template` | UI link (`{{.ID}}`, …) |

#### Redash (`redash` block)

Lists dashboards and/or alerts, then reads query SQL for matching datasources. Does not execute queries.

| Option | Filter / meaning |
|--------|------------------|
| `host` | Redash API host |
| `api_key` | API key |
| `datasource_name_contains` | Substring match on datasource name (required) |
| `entity_types` | `dashboard` and/or `alerts`; empty = both |
| `host_to_display` | URL written into exposures (empty → `host`) |
| `use_api_owner` | `true` → owner from Redash `user`; `false` (default) → `default_owner`; empty API fields fall back per-field |

#### Hive SQL catalog (`hive` block)

Same options as MySQL; connects with the Hive driver (typically LDAP). `type: hive` vs `type: mysql` selects the driver only.

| Option | Filter / meaning |
|--------|------------------|
| `host` | Server host |
| `port` | Server port (usually set for Hive) |
| `user` | DB user |
| `password` | DB password |
| `database` | Database / schema |
| `query` | Full SELECT (alternative to table + columns) |
| `table` | Catalog table when not using `query` |
| `id_column` | Row id column |
| `sql_column` | SQL / script text column |
| `description_column` | Optional description column |
| `extra_filters` | Extra SQL `WHERE` |
| `exposure_prefix` | Exposure name prefix |
| `exposure_url_template` | UI link (`{{.ID}}`, …) |

#### Gitscan (`gitscan` block)

Clones a source repo read-only and scans files under configured directories and extensions. Path tokens are matched against `manifest.filter_name`.

| Option | Filter / meaning |
|--------|------------------|
| `repo_url` | Source repo URL (read-only clone) |
| `login` | Clone username |
| `token` | Clone token |
| `local_path` | Working directory for the clone |
| `dirs` | Directories to scan |
| `extensions` | e.g. `.sql`, `.py`, `.php` |
| `path_db_tokens` | Path tokens matched with `manifest.filter_name` |
| `vcs` | `gitlab` \| `bitbucket` (browse URL shape) |

#### Qlik Sense (`qlik` block)

Targets **Qlik Sense Enterprise** (not Qlik Cloud). Lists apps via QRS (client cert), reads load scripts via the proxy ticket + WSS path (same as Python `QlikConnect`), optionally resolves tags/maturity via QRS.

| Option | Filter / meaning |
|--------|------------------|
| `host` | WSS proxy URL for scripts (e.g. `wss://qlik.example/app/`) — forms ticket auth |
| `user` | Forms login (`DIRECTORY\userid` or `DIRECTORY+userid`); also QRS identity fallback |
| `password` | Forms password for proxy ticket |
| `datasource` | `LIB CONNECT TO` warehouse slice of the load script |
| `stream_include` | Allowlist of **Qlik streams** (not app names); empty = no allowlist |
| `stream_exclude` | Blocklist of Qlik streams |
| `exposure_url_template` | Dashboard URL (`{{.AppID}}`, …) |
| `qrc.host` | QRS HTTPS host (**required**: app list + tags), e.g. `https://qlik.example:4242` |
| `qrc.cert_b64` | Client material (base64; whitespace OK): PKCS#12 **or** PEM certificate |
| `qrc.password` | PKCS#12 passphrase, **or** base64 PEM private key (legacy Python shape) |
| `qrc.user_directory` | `X-Qlik-User` directory |
| `qrc.user_id` | `X-Qlik-User` user id |

### Sink

| Option | Meaning |
|--------|---------|
| `sink.git.repo_url` | Target dbt repo (required) |
| `sink.git.user` | Write username (required) |
| `sink.git.token` | Write token (required; often via env) |
| `sink.git.local_path` | Clone working directory (required) |
| `sink.git.exposures_path` | Parent subtree for packages (required) |
| `sink.git.branch` | Feature branch to push (required) |
| `sink.git.base_branch` | Branch Sync starts from / MR target (required) |
| `sink.git.commit_name` / `commit_email` | Optional commit author (default `dbt-esync` / `dbt-esync@localhost`) |
| `sink.mr.enabled` | Open MR/PR after a successful push |
| `sink.mr.provider` | `gitlab` or `bitbucket` (required when enabled) |
| `sink.mr.host` | VCS API host URL (required when enabled) |

Package path written by the sink: `{exposures_path}/{source[].name}/*.yml` (one subtree per successful source). No diff vs `base_branch` → skip push and MR.

## Permissions

All sources are **read-only**. Only the sink writes.

| Component | Credential | Minimum rights | Reads / writes |
|-----------|------------|----------------|----------------|
| **growthbook** | API Bearer | read datasources / assignment queries | data sources + SQL |
| **mysql** | MySQL user | `SELECT` on SQL catalog (+ users JOIN) | SQL texts + metadata |
| **redash** | API key | list/read dashboards, alerts, data_sources, queries | SQL of widgets/alerts (**no** execute) |
| **hive** | Hive LDAP | `SELECT` on scripts table (+ owners) | script texts |
| **gitscan** | git clone token | **read** clone of source repo | files under `dirs` / `extensions` |
| **qlik** | WSS forms user/password + QRC PKCS#12/PEM cert + `X-Qlik-User` | Qlik Sense Enterprise: streams via QRS; **read load script** via proxy ticket/WSS; tags via QRS | scripts + tags |
| **sink.git** | bot + token | **write** target dbt repo (push branch) | — |
| **sink.mr** | same or separate token | create MR/PR on `base_branch` | — |
| **manifest** | HTTP(S) or file | read URL / file | dbt manifest |

Warehouse **execute SQL** is never required.

## Contributors

Thanks goes to these wonderful people ([emoji key](https://allcontributors.org/docs/en/emoji-key)):

<!-- ALL-CONTRIBUTORS-LIST:START - Do not remove or modify this section -->
<!-- prettier-ignore-start -->
<!-- markdownlint-disable -->
<table>
  <tbody>
    <tr>
      <td align="center" valign="top" width="14.28%"><a href="https://github.com/filinspace"><img src="https://avatars.githubusercontent.com/u/117206354?v=4?s=100" width="100px;" alt="filinspace"/><br /><sub><b>filinspace</b></sub></a><br /><a href="https://github.com/barloc/dbt-esync/commits?author=filinspace" title="Code">💻</a></td>
      <td align="center" valign="top" width="14.28%"><a href="https://github.com/ido177"><img src="https://avatars.githubusercontent.com/u/63454209?v=4?s=100" width="100px;" alt="Kirill Romanikhin"/><br /><sub><b>Kirill Romanikhin</b></sub></a><br /><a href="https://github.com/barloc/dbt-esync/commits?author=ido177" title="Code">💻</a></td>
    </tr>
  </tbody>
</table>

<!-- markdownlint-enable -->
<!-- prettier-ignore-end -->
<!-- ALL-CONTRIBUTORS-LIST:END -->

This project follows the [all-contributors](https://github.com/all-contributors/all-contributors) specification. Contributions of any kind welcome!

## License

Apache-2.0

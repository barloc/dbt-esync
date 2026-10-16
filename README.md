# dbt-esync

Sync dbt exposures from BI tools and code repos into your dbt project.

[![All Contributors](https://img.shields.io/badge/all_contributors-2-orange.svg?style=flat-square)](#contributors)

Go CLI / CronJob with YAML `--config`. One config = one source + one sink.

## Status

Config contract (T2) is in place. **Parse (T3):** `app/parse` extracts table refs and tags from SQL / Qlik scripts (`FilterQlikScript`, `Tables`, `Tags`). Source stubs still return `not implemented`; sinks and source implementations follow in later tasks.

## Build & run

```bash
make build
make run                          # uses config.example.yaml
# or
go run . --config config.example.yaml
```

Expected today: process loads and validates the full config, then exits with `not implemented` for `source.type`. Parse is covered by unit tests (`make test`).

```bash
make test
make lint
make docker-build
```

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
| Nil nested blocks | allocated when an override targets them (e.g. `SOURCE__MYSQL__HOST`) |

Examples:

```bash
export DBT_ESYNC_SINK__GIT__TOKEN=...
export DBT_ESYNC_SOURCE__QLIK__PASSWORD=...
export DBT_ESYNC_SOURCE__QLIK__STREAM_EXCLUDE="99-Archive,92-TMP"
export DBT_ESYNC_SINK__MR__ENABLED=false
export DBT_ESYNC_DEFAULT_OWNER__SLACK="#analytics"
```

Corp CronJobs typically mount a base YAML and inject secrets via these env vars (Vault / K8s secrets).

### Common

| Option | Meaning |
|--------|---------|
| `save_path` | Staging / YAML filename prefix |
| `default_owner.name` | Owner name when the source has none |
| `default_owner.email` | Owner email fallback |
| `default_owner.slack` | Optional Slack handle/channel (e.g. gitscan description) |
| `manifest.url` \| `manifest.file` | `manifest.json` — **exactly one** |
| `manifest.filter_name` | Warehouse/engine token for filters (`starrocks`, `vertica`, `bigquery`, …) |

Owners are configured **only** under `default_owner`. Sources may supply an owner from their API; otherwise this fallback is used. There are no per-source owner overrides.

### Parse

| Option | Meaning |
|--------|---------|
| `parse.excluded_catalogs` | Drop catalog segment from 3-part refs |
| `parse.schema_allowlist` | Optional allowlist for valid schemas (no corp names in public defaults) |

Package [`app/parse`](app/parse): `FilterQlikScript` (LIB CONNECT + SUB/CALL), `Tables` (FROM/JOIN / INLINE / `$(var)`), `Tags` (maturity vs labels). Empty `schema_allowlist` uses soft structural heuristics (`main` / `marts` / `core` plus length/underscore rules).

### Source types

Set `source.type`, then fill the matching nested block.

| `source.type` | Package | Status |
|---------------|---------|--------|
| `growthbook` | `app/source/growthbook` | stub |
| `mysql` | `app/source/mysql` | stub |
| `redash` | `app/source/redash` | stub |
| `hive` | `app/source/hive` | stub |
| `gitscan` | `app/source/gitscan` | stub |
| `qlik` | `app/source/qlik` | stub |

#### Qlik (`source.qlik`)

| Option | Filter / meaning |
|--------|------------------|
| `host` | Sense Engine host |
| `user` | Sense user |
| `password` | Sense password |
| `datasource` | `LIB CONNECT TO` warehouse slice of the load script |
| `stream_include` | Allowlist of **Qlik streams** (not app names); empty = no allowlist |
| `stream_exclude` | Blocklist of Qlik streams |
| `exposure_url_template` | Dashboard URL (`{{.AppID}}`, …) |
| `qrc.host` | Optional QRS/QRC host for tags/maturity |
| `qrc.cert_b64` | Client cert (base64) |
| `qrc.password` | QRC cert/key password |
| `qrc.user_directory` | QRC identity directory |
| `qrc.user_id` | QRC identity user |

#### Redash (`source.redash`)

| Option | Filter / meaning |
|--------|------------------|
| `host` | Redash API host |
| `api_key` | API key |
| `datasource_name_contains` | Substring match on datasource name |
| `entity_types` | `dashboard` and/or `alerts` |
| `host_to_display` | URL written into exposures |

#### Gitscan (`source.gitscan`)

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

#### Hive / MySQL SQL catalog (`source.hive` / `source.mysql`)

Same YAML schema; `source.type` selects the driver only (`hive` vs `mysql`). MySQL is the ex-Sigma SQL catalog.

| Option | Filter / meaning |
|--------|------------------|
| `host` | Server host |
| `port` | Server port (optional; Hive usually set) |
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

#### GrowthBook (`source.growthbook`)

| Option | Filter / meaning |
|--------|------------------|
| `host` | API host (not the UI) |
| `api_key` | Bearer token |
| `filter_source` | Datasource type (= `manifest.filter_name`) |
| `exposure_url` | UI template |

### Sink

| Option | Meaning |
|--------|---------|
| `sink.git.repo_url` | Target dbt repo (required) |
| `sink.git.user` | Write username |
| `sink.git.token` | Write token |
| `sink.git.local_path` | Clone working directory |
| `sink.git.exposures_path` | Subtree for exposure YAML (required) |
| `sink.git.branch` | Feature branch to push |
| `sink.git.base_branch` | MR/PR base branch |
| `sink.mr.enabled` | Open MR/PR after push |
| `sink.mr.provider` | `gitlab` or `bitbucket` |
| `sink.mr.host` | VCS API/host URL |

## Permissions

All sources are **read-only**. Only the sink writes.

| Component | Credential | Minimum rights | Reads / writes |
|-----------|------------|----------------|----------------|
| **growthbook** | API Bearer | read datasources / assignment queries | data sources + SQL |
| **mysql** | MySQL user | `SELECT` on SQL catalog (+ users JOIN) | SQL texts + metadata |
| **redash** | API key | list/read dashboards, alerts, data_sources, queries | SQL of widgets/alerts (**no** execute) |
| **hive** | Hive LDAP | `SELECT` on scripts table (+ owners) | script texts |
| **gitscan** | git clone token | **read** clone of source repo | files under `dirs` / `extensions` |
| **qlik** | Sense user; optional QRC mTLS | streams + **read load script**; QRS read app meta | scripts + tags |
| **sink.git** | bot + token | **write** target dbt repo (push branch) | — |
| **sink.mr** | same or separate token | create MR/PR on `base_branch` | — |
| **manifest** | HTTP(S) or file | read URL / file | dbt manifest |

Warehouse **execute SQL** is never required.

## Legacy env → YAML (rename notes)

Older deployments used `DBT_EXP_*` (and related) env vars. Mapping highlights:

| Legacy | YAML |
|--------|------|
| `DBT_EXP_SOURCE_NAME` | `source.type` |
| `from_sigma` | `mysql` |
| `from_git` | `gitscan` |
| `from_hive` | `hive` |
| `DBT_EXP_APP_EXCLUDE` / `APP_INCLUDE` | `qlik.stream_exclude` / `stream_include` (streams, not apps) |
| `DBT_EXP_DEFAULT_OWNER_*` / `GIT_EXPO_OWNERS` | `default_owner.*` (not per-source) |
| `SIGMA_EXPO_MYSQL_*` / `DBT_EXP_HIVE_*` | shared `SQLCatalog` under `source.mysql` / `source.hive` |
| `GIT_EXPO_*` (except owners) | `source.gitscan.*` |
| `DBT_EXP_GIT_*` | `sink.git.*` / `sink.mr.*` |

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

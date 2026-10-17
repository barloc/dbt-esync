# Changelog

## [1.0.2] — 2026-10-18

### Fixed

- Qlik `qrc.cert_b64`: also strip literal `\\n` / `\\r` sequences (Airflow Variable / Python `replace('\\n','')` shape), keep only StdEncoding alphabet

## [1.0.1] — 2026-10-17

### Fixed

- Qlik `qrc.cert_b64`: strip whitespace in base64; accept legacy PEM cert + PEM key (Python `QRCConnector` shape) as well as PKCS#12
- Optional `sink.git.commit_name` / `commit_email` for GitLab push rules that reject `@localhost` authors

### Changed

- `make docker-push` publishes multi-arch images (`linux/amd64` + `linux/arm64`) via buildx

## [1.0.0] — 2026-10-17

First public release.

### Added

- YAML `--config` with `DBT_ESYNC_*` env overrides (`__` nesting; env wins)
- `app/parse`: table extraction, Qlik Sense script filter/SUB, tags/maturity
- `app/manifest`: `manifest.json` from URL or file → `ref()` / `source()`
- `app/exposure`: dbt exposures YAML v2 encode/write
- `app/sink/git`: clone target repo, replace exposure packages, push branch
- `app/sink/vcs`: GitLab MR or Bitbucket PR when `sink.mr.enabled`
- Sources (read-only Collect): GrowthBook, MySQL/Hive SQL catalogs, Redash, gitscan, Qlik Sense Enterprise
- Multi-source config: N named packages + one sink commit (best-effort Collect; any Collect error → exit ≠ 0)
- Multi-stage Docker image (`barloc/dbt-esync`), Apache-2.0 license
- Local workflow: `make test`, `make build`, `make run`, `make lint`, `make docker-build`, `make docker-push`

### Notes

- No GitHub Actions CI — verify and publish images with Make
- Public fixtures are synthetic only; corp hosts and secrets stay in private configs / env

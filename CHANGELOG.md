# Changelog

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

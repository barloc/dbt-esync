# dbt-esync

Sync dbt exposures from BI tools and code repos into your dbt project.

Go CLI / CronJob with YAML `--config`. One config = one source + one sink.

## Status

Skeleton (T1): cobra entrypoint, package boundaries, and source stubs that return `ErrNotImplemented`. Full config schema, parsers, sinks, and source implementations follow in later tasks.

## Build & run

```bash
make build
make run                          # uses config.example.yaml
# or
go run . --config config.example.yaml
```

Expected on T1: process starts, reads `source.type`, and exits with `not implemented` for that source.

```bash
make test
make lint
make docker-build
```

## Source types

| `source.type` | Package | Status |
|---------------|---------|--------|
| `growthbook` | `app/source/growthbook` | stub |
| `mysql` | `app/source/mysql` | stub |
| `redash` | `app/source/redash` | stub |
| `hive` | `app/source/hive` | stub |
| `gitscan` | `app/source/gitscan` | stub |
| `qlik` | `app/source/qlik` | stub |

## License

Apache-2.0

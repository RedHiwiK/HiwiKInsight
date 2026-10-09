# The `insight` CLI

`insight` is a small command-line client for the [query API](query-api.md), meant for both people and AI assistants. It has no dependencies beyond the Go standard library.

## Install

```bash
go install github.com/RedHiwiK/HiwiKInsight/cmd/insight@latest
```

It is also included in the release archives and in the container image (`/usr/local/bin/insight`), and `make build` writes `bin/insight`.

## Endpoint and token

| Setting | Resolution order |
|---|---|
| Endpoint | `--endpoint URL`, then `$INSIGHT_ENDPOINT`, then `http://localhost:8080` |
| Token | `--token TOKEN`, then `$INSIGHT_TOKEN`, then the file `~/.config/hiwikinsight/token` |

```bash
export INSIGHT_ENDPOINT=https://insight.example.com
mkdir -p ~/.config/hiwikinsight
echo '<token>' > ~/.config/hiwikinsight/token
chmod 600 ~/.config/hiwikinsight/token
insight apps    # quick check
```

Tokens are configured on the server under `query.tokens`. On HTTP 401 the CLI says where the token came from and how to set one.

## Commands

| Command | What it does |
|---|---|
| `insight describe` | Print the semantic layer (Markdown): tables, views, conventions, event catalogs, metric list. Read this first |
| `insight apps` | List apps (key, name, bundle, color) plus today's date, time zone and currency |
| `insight metric <name> [flags]` | Run a built-in metric ([metrics.md](metrics.md)) |
| `insight sql "<SELECT ...>"` | Run read-only SQL (one `SELECT`/`WITH`, max 1000 rows, 10 s) |
| `insight user <install_id> [--app KEY] [--limit N]` | One user's summary, activity calendar and recent events |
| `insight mcp` | Run a Model Context Protocol server over stdio ([ai-integration.md](ai-integration.md)) |
| `insight version` | Print the CLI version |
| `insight help` | Show help |

### Metric flags

| Flag | Meaning |
|---|---|
| `--app KEY` | App key; omitted means the server's default (first configured) app |
| `--env ENV` | `production` (default), `sandbox` or `xcode` |
| `--from YYYY-MM-DD` | First day (default: 30 days ending at `--to`) |
| `--to YYYY-MM-DD` | Last day (default: today) |
| `--<name> VALUE` | Any other flag is passed to the metric as a parameter, e.g. `--cohort week`, `--event entry.created --group_by type`, `--tier heavy --limit 100` |

### Global flags

| Flag | Meaning |
|---|---|
| `--endpoint URL` | Server URL |
| `--token TOKEN` | API token |
| `--json` | Print the raw JSON response (indented) instead of tables |
| `--` | Stop flag parsing (for SQL that starts with `--`) |

Flags may appear anywhere, as `--name value` or `--name=value`.

## Output

By default results are printed as aligned tables: for metrics a header with app, env and date range, the definition, then one section per table; for SQL the columns and rows (with a note when truncated at 1000 rows). `null` prints as `-`. Use `--json` for scripts and agents that parse output.

## Examples

```bash
insight describe
insight apps
insight metric active_users --app pawprint --from 2026-09-01 --to 2026-09-30
insight metric retention --app pawprint --cohort week
insight metric activity_days --app pawprint --to 2026-09-30
insight metric event_trend --app pawprint --event entry.created --group_by type --granularity week
insight metric users --app pawprint --tier heavy --paid 1 --sort active_days --limit 20
insight metric portfolio --range month
insight metric store_funnel --scope all --from 2026-09-03 --to 2026-09-30
insight sql "SELECT module, COUNT(DISTINCT install_id) AS users FROM v_screen_usage WHERE app = 'pawprint' AND env = 'production' AND kind = 'viewed' GROUP BY module ORDER BY users DESC"
insight user 00435ad8-0028-40d3-a70e-0ff60b34dce8 --app pawprint --limit 50
insight --json metric funnel --app pawprint
```

## Exit codes

`0` on success, `1` on errors (network, HTTP, missing argument), `2` for usage errors (no command or unknown command).

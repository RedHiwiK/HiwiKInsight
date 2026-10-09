# Query API

The query API under `/v1/query/` is the read-only interface to a HiwiKInsight server. The dashboard, the `insight` CLI, the MCP server and AI agents all use it.

## Authentication

Every endpoint accepts either:

- `Authorization: Bearer <token>`, where `<token>` is one of `query.tokens` in the config, or
- a signed-in dashboard session cookie (or any request when `dashboard.insecure_no_auth` is on).

Otherwise the response is `401 {"error": "unauthorized"}`. Without configured tokens the API is reachable only from the dashboard.

## Errors

Errors are JSON: `{"error": "message"}` with status `400` (bad parameters, invalid SQL, SQL errors, unknown metric or app) or `401`.

## Limits

| Limit | Value |
|---|---|
| SQL statements | One `SELECT` or `WITH` statement; comments are stripped, a trailing `;` is allowed, any other `;` is rejected |
| Rows per SQL result | 1000 (`truncated: true` when more exist) |
| Execution time | 10 seconds per SQL query or metric |
| Date range | `from` to `to` at most 400 days |
| SQL request body | 64 KB |
| `user` events | `limit` 1 to 1000, default 200 |

All queries run on a read-only SQLite connection (`mode=ro` and `query_only`), so even a statement that passed validation cannot modify data.

## Endpoints

| Method and path | Purpose |
|---|---|
| `GET /v1/query/describe` | Semantic layer as Markdown |
| `GET /v1/query/meta` | Apps, today's date, time zone, currency, exchange rates |
| `GET /v1/query/metric/{name}` | Run a built-in metric |
| `POST /v1/query/sql` | Run read-only SQL |
| `GET /v1/query/user/{install_id}` | One user's summary, calendar and recent events |

The examples below use the demo server (`demo-token`).

### GET /v1/query/describe

Returns `text/markdown`: conventions (reporting time zone, users, environments, truly new users, sessions, parameters, money, retention), every table and view with its columns, example queries, the configured apps, the built-in metrics with their definitions, data volume per app and environment, undocumented events, and each app's event catalog.

```bash
curl -s -H "Authorization: Bearer demo-token" https://insight.example.com/v1/query/describe
```

```markdown
# HiwiKInsight data guide (semantic layer for AI assistants and people)

Self-hosted anonymous app analytics plus App Store transactions, stored in SQLite. Every query is read-only.

## Conventions

- **Time zone**: America/Los_Angeles. Every day column in tables and views is a calendar date (YYYY-MM-DD) in this zone. ...
...
## Built-in metrics (insight metric <name> --app --env --from --to)
...
## Undocumented events (stored but missing from the catalog; ask the developer what they mean)
...
## Event catalog: pawprint
```

This is the first thing an AI agent should read.

### GET /v1/query/meta

```bash
curl -s -H "Authorization: Bearer demo-token" https://insight.example.com/v1/query/meta
```

```json
{
  "apps": [
    {
      "key": "pawprint", "name": "Pawprint", "bundle": "com.example.pawprint", "color": "orange",
      "products": {"com.example.pawprint.pro.lifetime": "Pawprint Pro (Lifetime)"},
      "paywall_contexts": {"onboarding": "Onboarding", "record_limit": "Record limit"},
      "modules": [["onboarding", "First-launch welcome and pet setup"], ["timeline", "Journal timeline, entry details and search"]],
      "events": {"app.installed": "First launch with the SDK on this install", "entry.created": "A journal entry was saved"}
    }
  ],
  "today": "2026-10-09",
  "timezone": "America/Los_Angeles",
  "currency": "USD",
  "fx": {"AUD": 1.437873, "CAD": 1.423081, "EUR": 0.892251, "JPY": 158.033512, "USD": 1}
}
```

Apps are in config order with their resolved palette color. `fx` holds the exchange rates (1 base unit = N units) for the dashboard's display currencies; it may be empty until rates are fetched.

### GET /v1/query/metric/{name}

Query parameters:

| Parameter | Default | Meaning |
|---|---|---|
| `app` | first configured app | App key |
| `env` | `production` | `production`, `sandbox` or `xcode` |
| `from` | `to` minus 29 days | First day, `YYYY-MM-DD`, reporting time zone, inclusive |
| `to` | today | Last day, inclusive |
| `limit` | metric-specific | Used by `users` |
| anything else | | Passed to the metric as an extra parameter (for example `cohort=week`, `event=entry.created`) |

Metric names, definitions, extra parameters and output tables: [metrics.md](metrics.md).

```bash
curl -s -H "Authorization: Bearer demo-token" \
  "https://insight.example.com/v1/query/metric/active_users?app=pawprint&from=2026-10-01&to=2026-10-03"
```

```json
{
  "metric": "active_users",
  "definition": "Daily DAU (distinct install_ids with any event that day), WAU (trailing 7 days including the day), MAU (trailing 30 days), stickiness DAU/MAU, and the number of truly new users that day (first download day = first seen day)",
  "app": "pawprint",
  "env": "production",
  "from": "2026-10-01",
  "to": "2026-10-03",
  "tables": [
    {"name": "summary", "columns": ["days", "avg_dau", "users_in_range", "new_users"], "rows": [[3, 54.3, 105, 12]]},
    {"name": "daily", "columns": ["day", "dau", "wau", "mau", "new_users", "stickiness_pct"],
     "rows": [["2026-10-01", 56, 169, 292, 4, 19.2], ["2026-10-02", 60, 172, 299, 5, 20.1], ["2026-10-03", 47, 169, 300, 3, 15.7]]}
  ]
}
```

Every metric returns this shape: `metric`, `definition`, `app`, `env`, `from`, `to` and a list of named `tables`, each with `columns` and `rows`. Percentages are numbers with one decimal; `null` means "not observable" or "no data".

### POST /v1/query/sql

Body: `{"sql": "<one SELECT or WITH statement>"}`.

```bash
curl -s -H "Authorization: Bearer demo-token" -H "Content-Type: application/json" \
  -d '{"sql": "SELECT name, COUNT(*) AS n FROM events WHERE app = '\''pawprint'\'' GROUP BY name ORDER BY n DESC LIMIT 3"}' \
  https://insight.example.com/v1/query/sql
```

```json
{"columns": ["name", "n"], "rows": [["screen.viewed", 24363], ["screen.left", 24363], ["session.started", 7661]], "truncated": false}
```

A write attempt:

```json
{"error": "only SELECT / WITH queries are allowed"}
```

SQL tips:

- Prefer the views `v_user_days`, `v_user_summary`, `v_sessions`, `v_screen_usage` (see [architecture.md](architecture.md#tables-and-views)).
- Always filter by `app` and usually `env = 'production'`.
- Dates: `day` columns are already in the reporting time zone. For millisecond timestamps use `local_date(ts)` and `local_datetime(ts)`; for today use `local_today()`, e.g. `day >= date(local_today(), '-6 days')`.
- Event params are a JSON string: `json_extract(params, '$.context')`.
- `appstore_events.environment` is capitalized (`Production`, `Sandbox`).

### GET /v1/query/user/{install_id}

Parameters: `app` (default: first app), `limit` (recent events, default 200, maximum 1000). The id is matched case-insensitively.

```bash
curl -s -H "Authorization: Bearer demo-token" \
  "https://insight.example.com/v1/query/user/00435ad8-0028-40d3-a70e-0ff60b34dce8?app=pawprint&limit=3"
```

Returns the metric shape with `metric: "user"` and three tables:

| Table | Columns |
|---|---|
| `summary` | The `v_user_summary` row: app, env, install_id, first_seen_day, acquired_day, is_new, last_seen_day, first_version, last_version, device, device_name, os_version, region, storefront, language, active_days, sessions, duration_s, is_paid |
| `calendar` | day, env, sessions, duration_s, events (every active day, newest first) |
| `events` | time_local, name, session (first 8 characters), params, app_version (newest first, up to `limit`) |

An unknown install id returns empty tables.

## Other endpoints (not part of the query API)

| Method and path | Auth | Purpose |
|---|---|---|
| `POST /v1/events` | app key allow-list | SDK ingestion ([sdk-integration.md](sdk-integration.md)) |
| `POST /v1/appstore/notifications` | Apple signature | App Store Server Notifications V2 |
| `POST /v1/auth/login` | | Body `{"username", "password"}`; sets the session cookie |
| `POST /v1/auth/logout` | | Clears the cookie |
| `GET /v1/auth/me` | session | Current user |
| `GET /v1/admin/report/preview` | session only | Render a report ([reports-and-alerts.md](reports-and-alerts.md)) |
| `POST /v1/admin/report/send` | session only | Send a report now |
| `GET /healthz` | | Liveness: `ok` |

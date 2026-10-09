# Extending HiwiKInsight

This page shows how to add the most common kinds of functionality. Read [AGENTS.md](../AGENTS.md) for the repository map and conventions and [CONTRIBUTING.md](../CONTRIBUTING.md) for the contribution flow.

Before you start:

```bash
make web build
go test ./...
./bin/hiwikinsight demo -data ./demo-data     # http://localhost:8080/dashboard/, demo / demo, token demo-token
```

## Add a metric

Metrics live in `internal/metrics` and are shared by the API, CLI, MCP, dashboard and reports.

1. Implement the computation:

   ```go
   // internal/metrics/widgets.go
   package metrics

   func init() {
       defs = append(defs, Def{"widget_launches",
           "Sessions started from a widget per day: sessions and distinct users with session.started source starting with widget_",
           widgetLaunches})
   }

   func widgetLaunches(ctx context.Context, db *sql.DB, p Params) ([]Table, error) {
       t, err := queryTable(ctx, db, "daily", `
           SELECT day, COUNT(*) AS sessions, COUNT(DISTINCT install_id) AS users
           FROM events
           WHERE app = ? AND env = ? AND day BETWEEN ? AND ?
             AND name = 'session.started' AND substr(json_extract(params, '$.source'), 1, 7) = 'widget_'
           GROUP BY day ORDER BY day`, p.App, p.Env, p.From, p.To)
       if err != nil {
           return nil, err
       }
       return []Table{t}, nil
   }
   ```

   - Use `p.App`, `p.Env`, `p.From`, `p.To` (reporting-time-zone days), `p.Bundle` for App Store tables, and `p.arg("name")` for extra parameters.
   - Use `day` columns or `local_date()`; never compute days from UTC.
   - Validate any parameter that becomes part of the SQL text (see `eventNameRE` and `paramKeyRE` in `dashboard.go`); pass values as `?` arguments.
   - The definition is a contract shown to people and agents. Make it precise: what is counted, over which window, with which exclusions.

2. Test it with seeded data, following `internal/metrics/metrics_test.go`.
3. Document it in [metrics.md](metrics.md). If agents should know it by name, also mention it in `skills/hiwikinsight-analytics/SKILL.md` and in the `metric` tool description in `cmd/insight/mcp.go`.

The metric is immediately available at `/v1/query/metric/widget_launches`, `insight metric widget_launches` and through MCP, and `describe` lists it.

## Add a dashboard page

1. Create `web/src/pages/Widgets.tsx` from an existing page such as `Errors.tsx`. Fetch data with `api.metric(name, params)` and React Query; wrap the page in `Page`; write every string as `t('中文', 'English')`.
2. Register a route in `web/src/App.tsx` and a navigation item in `web/src/components/Layout.tsx`.
3. Colors: only CSS variables from `web/src/styles/tokens.css`. Add a new token to both the light and dark definitions; if a chart uses it, add it to `tokenNames` in `web/src/theme.tsx` (charts read resolved colors through `usePalette()`).
4. `cd web && pnpm typecheck && pnpm build`, then check the page in the demo in both themes and both languages, on a narrow window as well.

## Add an alert rule

1. In `internal/report/alert.go`, add a check with the same signature as the existing ones:

   ```go
   func myAlerts(ctx context.Context, db *sql.DB, rules AlertRules, app App, dayStart int64, now time.Time) ([]alert, error)
   ```

   Return `alert{rule: "my_rule", key: app.Key + "|" + id, view: alertView(subject, title, now, hero, note, occurrences)}`. `rule` and `key` deduplicate the alert to once per day. Query production data only.
2. Add it to the per-app list in `checkAlerts`, or call it once there if it is not per app (like `ascStaleAlert`).
3. Thresholds: add a field to `AlertRules` (`internal/report/report.go`), to `Alerts` in `internal/config/config.go` with a default in `applyDefaults`, and pass it where `report.AlertRules` is built in `internal/server/server.go`.
4. All text through `i18n.T("English", "中文")`.
5. Test in `internal/report/alert_test.go`; document in [reports-and-alerts.md](reports-and-alerts.md).

## Add a config field

1. Add the field with a `yaml` tag and a doc comment in `internal/config/config.go`.
2. Set its default in `applyDefaults` and check it in `validate`. Resolve paths with `c.Resolve` in `Load`.
3. Wire it where it is used, usually in `internal/server/server.go`.
4. Add it to `examples/config.example.yaml` and [configuration.md](configuration.md), and add a test to `internal/config/config_test.go`.

Unknown keys are rejected, so renaming a field breaks existing configs. Prefer adding over renaming.

## Add a SQL function

Functions available in queries are registered in `internal/tz/tz.go` with `sqlite.MustRegisterScalarFunction` (see `local_date`). To add one:

1. Register it in an `init()` with a fixed argument count; return `nil` for invalid input rather than an error where possible.
2. Keep it deterministic and read-only; it runs inside the read-only query API.
3. Mention it in the conventions of `describeIntro` in `internal/query/query.go` so agents know it exists, and in [query-api.md](query-api.md).
4. Remember that the `sqlite3` CLI does not know these functions; do not use them in views unless that is acceptable.

## Add a table or column

1. Append a new migration string to `migrations` in `internal/store/migrations.go`. Never edit an applied migration.
2. Use `json_extract`, not `->>`, in views.
3. Write through the store's single writer (`s.db`); read through `ReadOnly()`.
4. Describe the new table or columns in `describeIntro` (`internal/query/query.go`) so agents can query them.

## Contribution flow

1. Open an issue for anything larger than a small fix, to agree on the approach.
2. Branch, implement, add tests and docs.
3. `go vet ./... && go test ./... && (cd web && pnpm typecheck)`.
4. Open a pull request with a Conventional Commits title. See [CONTRIBUTING.md](../CONTRIBUTING.md).

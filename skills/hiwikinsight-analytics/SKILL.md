---
name: hiwikinsight-analytics
description: Query and analyze product analytics and App Store revenue for apps tracked by a HiwiKInsight server. Use when the user asks about app usage or growth - DAU/WAU/MAU, retention, how many days users come back, returning/resurrected/churned users, which modules or screens are used, paywall funnels, purchase attribution (which entry point led to a purchase), revenue, device/iOS version/region distribution, or one user's journey - or says things like "check the analytics", "look at the data", "analyze our users".
---

# HiwiKInsight analytics

The data lives on a self-hosted HiwiKInsight server and is reached through its read-only query API with the `insight` CLI.

- Endpoint: `--endpoint`, else `$INSIGHT_ENDPOINT`, else `http://localhost:8080`
- Token: `--token`, else `$INSIGHT_TOKEN`, else `~/.config/hiwikinsight/token`

If a command fails with HTTP 401 or "cannot reach", tell the user how to set the endpoint/token instead of guessing data.

## Workflow

1. **Run `insight describe` first.** It returns the semantic layer: tables, views, conventions, event catalogs, every predefined metric with its definition, current data volume and unregistered events. Run it at least once per session; never assume column or event names from memory.
2. **Run `insight apps`** if you do not know the app key. When `--app` is omitted the server uses its default (first configured) app; say which app you queried.
3. **Prefer predefined metrics** with `insight metric <name>` - their definitions are fixed and documented:
   - `active_users` - DAU/WAU/MAU, stickiness, new users
   - `activity_days` - distribution of active days in the last 7 / 28 days (do users come every day?)
   - `retention` - D1/D3/D7/D14/D30 retention of truly new users
   - `returning` - new / returning / resurrected / churned
   - `module_usage` - reach, counts and time spent per module and screen
   - `funnel` - paywall funnel by entry point (context)
   - `revenue` - real App Store revenue by entry point / product / region, days from install to purchase
   - `distribution` - device, iOS version, app version, region, App Store country, language
   - others such as `overview`, `sessions_daily`, `users`, `event_trend`, `errors`, `portfolio`, `store_funnel` - see describe
   - Common flags: `--app myapp`, `--env production` (default), `--from YYYY-MM-DD --to YYYY-MM-DD` (default: last 30 days). Any other `--key value` is passed to the metric as a parameter (describe lists them).
4. **Fall back to `insight sql "..."`** only when no metric fits: a single SELECT/WITH, max 1000 rows, 10 second timeout. Prefer the views `v_user_days`, `v_user_summary`, `v_sessions`, `v_screen_usage`. Read event parameters with `json_extract(params, '$.key')`. For dates use `local_today()` and `local_date(ms)`, which follow the server's reporting time zone; do not hard-code UTC offsets.
5. **Inspect a single user** with `insight user <install_id> [--app myapp] [--limit 200]` (summary, activity calendar, recent events).
6. Add `--json` when you need machine-readable output.

## Conventions and discipline

- **Time zone**: the `day` columns and `local_*()` functions use the reporting time zone stated in describe. `ts` columns are epoch milliseconds; use `local_datetime(ts)` to display them.
- **Environment**: default to `env = 'production'`. `sandbox` (TestFlight) and `xcode` (development) are excluded unless the user asks.
- **Users** are anonymous `install_id`s. Reinstalling the app produces a new id, so "users" means installs.
- **Truly new user** = first seen on the same day as the App Store first download (`v_user_summary.is_new = 1`). Existing users who upgrade into a tracked version also emit install events but are not new; never count them as acquisition.
- Data starts when tracking was added to each app; describe shows the volume per app. Do not estimate periods with no data.
- **Small samples**: always give absolute counts next to ratios and do not draw conclusions from single-digit samples.
- **Only state what the data supports.** If data is missing, an event is unregistered or a definition is unclear, say so instead of filling the gap.
- The API is read-only; never attempt writes.

## Common recipes

**Do users come every day? (active-day distribution)**
```bash
insight metric activity_days --app myapp --to 2026-09-30
```

**Which module do resurrected users open first?** (back after 8 or more inactive days)
```bash
insight sql "
WITH d AS (
  SELECT install_id, day, LAG(day) OVER (PARTITION BY install_id ORDER BY day) AS prev
  FROM v_user_days WHERE app='myapp' AND env='production'
), back AS (
  SELECT install_id, day FROM d WHERE prev IS NOT NULL AND julianday(day) - julianday(prev) >= 8
), first_screen AS (
  SELECT b.install_id, b.day, s.module,
    ROW_NUMBER() OVER (PARTITION BY b.install_id, b.day ORDER BY s.ts) AS rn
  FROM back b JOIN v_screen_usage s ON s.install_id = b.install_id AND s.day = b.day AND s.kind = 'viewed'
)
SELECT module, COUNT(*) AS resurrected_visits FROM first_screen WHERE rn = 1 GROUP BY module ORDER BY 2 DESC"
```

**First-day behavior vs. day-7 retention** (example: did the user create an entry on day one? Replace `entry.created` with a key action from the app's event catalog in describe.)
```bash
insight sql "
WITH n AS (
  SELECT install_id, first_seen_day FROM v_user_summary
  WHERE app='myapp' AND env='production' AND is_new = 1
    AND first_seen_day <= date(local_today(), '-7 days')
), f AS (
  SELECT n.install_id,
    EXISTS (SELECT 1 FROM events e WHERE e.app='myapp' AND e.install_id = n.install_id
            AND e.name = 'entry.created' AND e.day = n.first_seen_day) AS did_action,
    EXISTS (SELECT 1 FROM v_user_days d WHERE d.app='myapp' AND d.install_id = n.install_id
            AND d.day = date(n.first_seen_day, '+7 days')) AS d7
  FROM n
)
SELECT did_action, COUNT(*) AS users, SUM(d7) AS d7_retained, ROUND(AVG(d7) * 100, 1) AS d7_pct
FROM f GROUP BY did_action"
```

**How do sessions start? (do widgets or notifications bring people back?)**
```bash
insight sql "
SELECT source, COUNT(*) AS sessions, COUNT(DISTINCT install_id) AS users
FROM v_sessions
WHERE app='myapp' AND env='production' AND day >= date(local_today(), '-29 days')
GROUP BY source ORDER BY sessions DESC"
```

**Paywall entry points and real revenue**
```bash
insight metric funnel --app myapp
insight metric revenue --app myapp --from 2026-09-01
```

**One user's journey**
```bash
insight user 3F2A9C1E-0000-0000-0000-000000000000 --app myapp --limit 200
```

## Output

- Lead with the conclusion, then the supporting numbers: absolute counts plus ratios, the date range and the env (and app) queried.
- Call out small samples and data gaps explicitly.
- Include the exact commands you ran so the result can be reproduced.

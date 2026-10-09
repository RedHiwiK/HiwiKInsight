# Dashboard

The web dashboard is a React app embedded in the server binary and served at `https://insight.example.com/dashboard/` (`/` redirects there). It reads everything through the [query API](query-api.md) with your sign-in session, so it shows exactly what the API and AI assistants see. Turn it off with `dashboard.enabled: false`.

## Sign-in

- Users come from `dashboard.users` in the config. Create a password hash with `hiwikinsight hash-password` (it prompts for the password; at least 8 characters) and restart.
- A sign-in lasts `dashboard.session_ttl` (default 30 days). The session is an HttpOnly cookie signed with a random secret stored in `data_dir/session.key`; it survives restarts. It is marked `Secure` when the request is HTTPS (directly or via `X-Forwarded-Proto: https`).
- To sign everyone out, delete `session.key` and restart. Removing a user from the config ends that user's sessions.
- After 10 failed attempts from one IP within 15 minutes, further attempts get HTTP 429 until the window passes.
- For local use only, `dashboard.insecure_no_auth: true` disables sign-in; the server refuses this unless `listen` is a loopback address.

If no users are configured, the server logs a warning and the sign-in page cannot be passed.

## Filters

Most pages share a header with:

- **App**: one of the configured apps.
- **Environment**: `production` (default), `sandbox` (TestFlight) or `xcode`. Non-production data is flagged as test data.
- **Date range**: presets or custom dates, in the reporting time zone.

## Pages

| Page | Path | What it shows | Main metrics |
|---|---|---|---|
| Portfolio | `/dashboard/` | All apps together: proceeds month to date vs. last month, last month, year to date, last 12 months; per-app comparison; trend; revenue mix; countries; store conversion; live feed of transactions | `portfolio`, `store_funnel` |
| Overview | `/overview` | One app: KPIs vs. the previous period, daily active mix, user tiers, top modules, app versions | `overview`, `active_users`, `returning`, `module_usage`, `distribution` |
| Engagement | `/activity` | Do users come back every day: L7 and L28 active-day distributions, DAU/WAU/MAU, new/returning/resurrected users, daily sessions | `activity_days`, `active_users`, `returning`, `sessions_daily` |
| Retention | `/retention` | Retention heatmap and curve for truly new users | `retention` |
| Modules | `/modules` | Module and screen ranking, feature usage, coverage vs. time spent | `module_usage` |
| Users | `/users` | Every anonymous user active in the period, with tier, filters, sorting and paging | `users` |
| User detail | `/users/<install_id>` | Profile, activity calendar and recent events of one user | user endpoint |
| Acquisition | `/acquisition` | Store funnel: impressions, product page views, first-time downloads; daily trend; by source; by country | `store_funnel` |
| Revenue | `/revenue` | Daily revenue, paywall funnel, revenue by product, entry point and App Store country, days from install to purchase | `revenue`, `funnel` |
| Devices & Regions | `/devices` | Devices, OS versions, app versions, regions, storefronts, languages of active users | `distribution` |
| Events | `/events` | Pick any event: trend by day/week/month, grouped by a param, param breakdown, catalog description | `event_trend` |
| Errors | `/errors` | Daily error counts and the error list sorted by affected users | `errors` |
| Settings | `/settings` | Theme, language, display currency, report previews, sign-out | |

Portfolio and Acquisition need App Store Connect sync ([app-store-setup.md](app-store-setup.md)); without it they show an empty state.

## Languages, theme and currencies

These are per-browser preferences (stored in `localStorage`) on the Settings page:

- **Language**: English or Chinese. The default follows the browser language. Texts from your event catalog (module and event descriptions) are shown as written. The language of emails is set separately by `language` in the config.
- **Theme**: system, light or dark.
- **Display currency**: the base currency (`currency` in the config) or one of USD, EUR, JPY, GBP, CNY, AUD, CAD, CHF, HKD, SGD. Amounts are stored in the base currency and converted for display with the day's exchange rates from `/v1/query/meta`.

App colors come from `apps[].color` (palette slots `purple`, `teal`, `orange`, `pink`, `blue`, `violet`, `green`, `gold`).

## Report previews

Settings links to `/v1/admin/report/preview?kind=daily` and `?kind=weekly`, which render the email reports in the browser for the signed-in user. See [reports-and-alerts.md](reports-and-alerts.md).

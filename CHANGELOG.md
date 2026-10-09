# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [v1.0.0] — first open-source release

### Added

- Single Go binary (`hiwikinsight`) with embedded pure-Go SQLite (no cgo), subcommands `serve`, `demo`, `check-config`, `hash-password` and `version`.
- One YAML configuration file with `${NAME}` / `${NAME:-default}` environment expansion, strict validation, configurable reporting time zone, language (`en`/`zh`) and base currency.
- Event ingestion (`POST /v1/events`) for the HiwiKInsightKit Swift SDK (protocol schema 1): raw DEFLATE, limits, deduplication, clock correction, per-install daily cap, no IP addresses stored.
- Derived tables for installs, daily activity and purchase attempts; analysis views `v_user_days`, `v_user_summary`, `v_sessions`, `v_screen_usage`; raw event retention (`retention_days`).
- "Truly new user" detection from the App Store original download date.
- Environment safety nets: production data from never-released builds and from development devices is reclassified as sandbox.
- App Store Server Notifications V2 (`POST /v1/appstore/notifications`) with JWS verification, idempotent storage, base-currency conversion, transaction emails and purchase attribution through `appAccountToken`.
- App Store Connect sync: Summary Sales Reports (daily and monthly history), Analytics Reports (impressions, product page views, first-time downloads by source and country) and the released-build list.
- 15 built-in metrics with fixed definitions: `active_users`, `activity_days`, `retention`, `returning`, `module_usage`, `funnel`, `revenue`, `distribution`, `overview`, `sessions_daily`, `users`, `event_trend`, `errors`, `portfolio`, `store_funnel`.
- Read-only query API (`/v1/query/describe`, `meta`, `metric/{name}`, `sql`, `user/{id}`) with bearer tokens, a read-only SQLite connection and limits (single SELECT/WITH, 1000 rows, 10 s, 400-day range).
- `describe` semantic layer for AI assistants: conventions, tables, views, metrics, data volume, undocumented events and per-app event catalogs.
- `insight` CLI and a built-in MCP server (`insight mcp`) with the tools `describe`, `list_apps`, `metric`, `sql` and `user`.
- Claude Code skill `hiwikinsight-analytics`.
- Web dashboard with 13 pages (Portfolio, Overview, Engagement, Retention, Modules, Users, User detail, Acquisition, Revenue, Devices & Regions, Events, Errors, Settings), English and Chinese, light and dark mode, display currencies, bcrypt sign-in.
- Daily and weekly email reports with preview and resend endpoints.
- Alerts for failed purchases, payment errors, new and spiking errors, stale App Store Connect reports and silent apps, deduplicated per day.
- Demo mode with three sample apps and 90 days of generated data (`hiwikinsight demo`, `docker-compose.demo.yml`, `make demo`).
- Deployment assets: multi-arch container image on GHCR, Docker Compose with Caddy, systemd unit, nginx example, release archives for linux and darwin (amd64, arm64).
- Documentation in `docs/`, `AGENTS.md` for AI coding agents, English and Chinese READMEs.

[Unreleased]: https://github.com/RedHiwiK/HiwiKInsight/compare/v1.0.0...HEAD
[v1.0.0]: https://github.com/RedHiwiK/HiwiKInsight/releases/tag/v1.0.0

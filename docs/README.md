# HiwiKInsight documentation

The documentation in this directory is English only. The project overview is also available in Chinese: [README.zh-CN.md](../README.zh-CN.md).

## Using HiwiKInsight

| Document | What it covers |
|---|---|
| [getting-started.md](getting-started.md) | Run the demo, deploy for real in about ten minutes, check the first event |
| [configuration.md](configuration.md) | Every config field with type, default and meaning; environment variables |
| [deployment.md](deployment.md) | Docker Compose + Caddy, binary + systemd + nginx, backups, upgrades, security checklist |
| [sdk-integration.md](sdk-integration.md) | HiwiKInsightKit: events, screens, launch sources, errors, snapshots, purchase attribution |
| [event-catalog.md](event-catalog.md) | The per-app event catalog file and how `describe` uses it |
| [app-store-setup.md](app-store-setup.md) | App Store Server Notifications V2 and the App Store Connect API key |
| [dashboard.md](dashboard.md) | Dashboard pages, sign-in, currencies and languages |
| [reports-and-alerts.md](reports-and-alerts.md) | Transaction emails, daily and weekly reports, alert rules |
| [privacy.md](privacy.md) | What is collected and what is not, retention |

## Querying data

| Document | What it covers |
|---|---|
| [metrics.md](metrics.md) | Every built-in metric: definition, parameters, output tables |
| [query-api.md](query-api.md) | `/v1/query/*` endpoints, authentication, limits |
| [cli.md](cli.md) | The `insight` command-line client |
| [ai-integration.md](ai-integration.md) | Claude Code skill, MCP server, HTTP API, prompting tips |

## Developing HiwiKInsight

| Document | What it covers |
|---|---|
| [architecture.md](architecture.md) | Components, data flow, tables and views, time zone and currency model, design decisions |
| [extending.md](extending.md) | Adding a metric, dashboard page, alert rule, config field or SQL function |
| [../AGENTS.md](../AGENTS.md) | Repo map, build and test commands, conventions (for AI coding agents and people) |
| [../CONTRIBUTING.md](../CONTRIBUTING.md) | Development setup, style, commits, pull requests |

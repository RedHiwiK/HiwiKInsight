# Contributing

Thanks for your interest in HiwiKInsight. Bug reports, documentation fixes and pull requests are welcome.

## Before you start

- For bugs, open an issue with the version (`hiwikinsight version`), what you did, what you expected and what happened. Include logs with secrets removed.
- For new features, open an issue first to agree on the approach. HiwiKInsight aims to stay small: one binary, one SQLite file, one config file, few dependencies.
- Security issues: do not open a public issue; see [SECURITY.md](SECURITY.md).
- Changes to the event wire protocol must go to [HiwiKInsightKit's PROTOCOL.md](https://github.com/RedHiwiK/hiwikinsight-ios/blob/main/PROTOCOL.md) first and stay backward compatible.

## Development setup

Requirements: Go 1.26+, Node 22, pnpm 10 (`corepack enable`).

```bash
git clone https://github.com/RedHiwiK/HiwiKInsight.git
cd HiwiKInsight
make web          # dashboard -> web/dist/app (embedded in the binary)
make build        # bin/hiwikinsight, bin/insight
make demo         # http://localhost:8080/dashboard/ (demo / demo), query token demo-token
```

Dashboard development with hot reload:

```bash
go run ./cmd/hiwikinsight demo -data ./demo-data -listen :8090   # the Vite dev proxy targets :8090
cd web && pnpm dev
```

[AGENTS.md](AGENTS.md) has the repository map, conventions and step-by-step guides for adding metrics, dashboard pages, alert rules and config fields; [docs/extending.md](docs/extending.md) has examples.

## Tests

```bash
go vet ./...
go test ./...
cd web && pnpm typecheck
```

`make test` runs all three. CI runs vet, tests and a full build on every pull request. Add tests for new behavior; metric and report tests use seeded SQLite databases (see the existing `_test.go` files).

## Style

- Go: `gofmt`, standard library first, small packages with a package comment, errors returned rather than logged where possible. No cgo.
- All dates in the reporting time zone through package `tz` and the SQL functions `local_date`, `local_datetime`, `local_today`; never hard-code a UTC offset.
- Server-generated text through `i18n.T("English", "中文")`; dashboard text through `t('中文', 'English')`.
- Dashboard colors only through the tokens in `web/src/styles/tokens.css`.
- Database migrations are append-only.
- The query API stays read-only.
- Comments and identifiers in English.

## Commit messages

Use [Conventional Commits](https://www.conventionalcommits.org/) in English:

```
feat(metrics): add widget_launches metric
fix(ingest): reject events with empty install_id
docs: explain App Store Connect key roles
```

Common types: `feat`, `fix`, `docs`, `refactor`, `perf`, `test`, `build`, `ci`, `chore`. Scopes are usually package or area names (`ingest`, `metrics`, `query`, `notify`, `report`, `asc`, `dashboard`, `cli`, `mcp`, `deploy`).

## Pull request checklist

- [ ] `go vet ./...` and `go test ./...` pass.
- [ ] `cd web && pnpm typecheck` passes (if the dashboard changed).
- [ ] New behavior has tests.
- [ ] Docs updated (`docs/`, `README.md`, `examples/config.example.yaml` for config changes, `CHANGELOG.md` under "Unreleased").
- [ ] New migrations appended, existing ones untouched.
- [ ] No new personal data collected; ingestion still ignores client IPs.
- [ ] Event wire protocol unchanged, or changed compatibly together with PROTOCOL.md.
- [ ] Dashboard changes checked in light and dark mode and in English and Chinese.
- [ ] No secrets, real app names, bundle IDs or domains in code, tests or docs (use `pawprint`, `com.example.*`, `insight.example.com`).

## License

By contributing you agree that your contributions are licensed under the [MIT License](LICENSE).

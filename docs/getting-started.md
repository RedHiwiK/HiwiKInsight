# Getting started

This guide takes you from nothing to a running demo, then to a real deployment receiving events from your app.

## 1. Run the demo

The demo starts the server with three fictional apps (Pawprint, Ledgerly, Trailmark) and 90 days of generated events, App Store transactions and App Store Connect report data. Mail, reports, alerts and App Store Connect sync are off.

With Docker:

```bash
git clone https://github.com/RedHiwiK/HiwiKInsight.git
cd HiwiKInsight
docker compose -f docker-compose.demo.yml up --build
```

From source (Go 1.26+, Node 22, pnpm 10):

```bash
make demo        # builds the dashboard, then runs: go run ./cmd/hiwikinsight demo -data ./demo-data
```

Then:

- Dashboard: http://localhost:8080/dashboard/ , user `demo`, password `demo`.
- Query API token: `demo-token`.

```bash
curl -s -H "Authorization: Bearer demo-token" http://localhost:8080/v1/query/describe | head -60
```

The demo writes `config.yaml`, `catalogs/` and the database into its data directory and reuses the database on later runs. Delete the directory (or run `docker compose -f docker-compose.demo.yml down -v`) to regenerate data ending today. `hiwikinsight demo` accepts `-data DIR` (default `./demo-data`) and `-listen ADDR` (default `:8080`).

## 2. Deploy for real (about ten minutes)

Prerequisites:

- A Linux server with Docker and Docker Compose, ports 80 and 443 open.
- A DNS name pointing at it, for example `insight.example.com`.

Steps:

```bash
git clone https://github.com/RedHiwiK/HiwiKInsight.git && cd HiwiKInsight
mkdir -p config/catalogs
cp examples/config.example.yaml config/config.yaml
cp deploy/.env.example .env
```

1. **Hash a dashboard password**:

   ```bash
   docker run --rm -it ghcr.io/redhiwik/hiwikinsight:latest hash-password
   ```

   Put the hash in `config/config.yaml` under `dashboard.users[].password_hash` (single-quoted), or in `.env` as `HIWIKINSIGHT_ADMIN_HASH='$2a$10$...'` if your config references `${HIWIKINSIGHT_ADMIN_HASH}`. Keep the single quotes in `.env`: bcrypt hashes contain `$`, which Docker Compose would otherwise try to interpolate.

2. **Create a query token** for the CLI and AI agents and put it in `.env` as `HIWIKINSIGHT_QUERY_TOKEN`:

   ```bash
   openssl rand -hex 32
   ```

3. **Edit `config/config.yaml`**:
   - `timezone`: the IANA zone that defines your "day", for example `America/New_York`.
   - `currency`: the base currency for revenue, for example `USD`.
   - `apps`: one entry per app with `key` (the SDK's `appKey`), `bundle_id`, and optionally `name`, `catalog`, `products`, `paywall_contexts`.
   - Remove `data_dir` (the container stores data in the `/data` volume).
   - Remove or fill in `mail` and `app_store_connect`; both can be added later.

   See [configuration.md](configuration.md) for every field.

4. **Set `DOMAIN`** in `.env` to your host name.

5. **Validate and start**:

   ```bash
   docker compose run --rm hiwikinsight check-config
   docker compose up -d
   docker compose logs -f hiwikinsight
   ```

   Caddy obtains a certificate for `DOMAIN` automatically. Open `https://insight.example.com/dashboard/` and sign in.

For a binary install with systemd and nginx, see [deployment.md](deployment.md).

## 3. Send the first event

Integrate HiwiKInsightKit in your app (details in [sdk-integration.md](sdk-integration.md)):

```swift
import HiwiKInsightKit

HiwiKInsight.start(.init(appKey: "pawprint", endpoint: URL(string: "https://insight.example.com")!))
```

Run the app. The SDK sends `app.installed` and `session.started` in its first batch (immediately at 50 queued events, every 30 seconds, and when the app goes to the background). Debug builds report `env = xcode`, so switch the dashboard's environment filter accordingly.

Check from the command line with the `insight` CLI:

```bash
go install github.com/RedHiwiK/HiwiKInsight/cmd/insight@latest
export INSIGHT_ENDPOINT=https://insight.example.com
export INSIGHT_TOKEN=<your query token>
insight sql "SELECT * FROM events ORDER BY ts DESC LIMIT 5"
```

Or from the server itself (the container image includes the CLI):

```bash
docker compose exec hiwikinsight insight --token <your query token> sql "SELECT * FROM events ORDER BY ts DESC LIMIT 5"
```

If nothing arrives:

- The server log shows `reject events: unknown app` when the SDK's `appKey` is not in `apps[].key`.
- Turn on `debugLogging` in the SDK configuration to see each event and the upload result.
- `curl https://insight.example.com/healthz` should print `ok`.

## 4. Next steps

- Connect App Store revenue: [app-store-setup.md](app-store-setup.md).
- Write an event catalog so people and AI know what each event means: [event-catalog.md](event-catalog.md).
- Configure SMTP for transaction emails, reports and alerts: [reports-and-alerts.md](reports-and-alerts.md).
- Ask questions with Claude or another MCP client: [ai-integration.md](ai-integration.md).
- Set up backups: [deployment.md](deployment.md#backups).

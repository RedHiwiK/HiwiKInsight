# AI integration

HiwiKInsight is designed so an AI assistant can answer product questions ("did the new onboarding improve D7 retention?", "which paywall brings the most revenue?") directly from your data, without you exporting anything. There are three ways to connect an assistant, all read-only.

| Way | Best for | Setup |
|---|---|---|
| CLI + Claude Code skill | Claude Code in a terminal | Install `insight`, copy the skill |
| MCP server (`insight mcp`) | Claude Code, Claude Desktop, Cursor and other MCP clients | Add one server entry |
| HTTP API | Your own agents, scripts, notebooks | Bearer token + `/v1/query/*` |

All three need a query token from `query.tokens` in the server config (generate with `openssl rand -hex 32`). Use a separate token per assistant so you can revoke it independently.

## The semantic layer: start with `describe`

Every integration exposes `describe` (`insight describe`, MCP tool `describe`, `GET /v1/query/describe`). It returns Markdown generated from the live server:

- **Conventions**: reporting time zone, what a user is (anonymous `install_id`), environments, what a truly new user is, what active means, how sessions work, how params are stored and bucketed, the base currency, retention.
- **Tables and views** with their columns, plus example queries.
- **Apps** and **built-in metrics** with their exact definitions.
- **Data on hand**: events, installs and date range per app and environment.
- **Undocumented events**: names that arrive but are not in the event catalog.
- **Event catalogs**: what each event and param means, written by the app developer ([event-catalog.md](event-catalog.md)).

An assistant that reads `describe` first does not need to guess column names, event names or metric definitions. The better your event catalog, the better the answers.

## 1. CLI + Claude Code skill

```bash
go install github.com/RedHiwiK/HiwiKInsight/cmd/insight@latest
export INSIGHT_ENDPOINT=https://insight.example.com
mkdir -p ~/.config/hiwikinsight && echo '<token>' > ~/.config/hiwikinsight/token
insight apps   # quick check
```

Install the skill for all projects:

```bash
mkdir -p ~/.claude/skills
cp -r skills/hiwikinsight-analytics ~/.claude/skills/
```

or for one repository (shared with everyone working on it):

```bash
mkdir -p .claude/skills
cp -r skills/hiwikinsight-analytics .claude/skills/
```

The skill ([skills/hiwikinsight-analytics/SKILL.md](../skills/hiwikinsight-analytics/SKILL.md)) teaches Claude Code the workflow (describe first, metrics before SQL, default to production), the conventions, reporting discipline (absolute counts next to ratios, no conclusions from tiny samples) and common recipes. Claude uses it automatically when you ask analytics questions. See [cli.md](cli.md) for every command.

## 2. MCP server

`insight mcp` runs a Model Context Protocol server over stdio (JSON-RPC, protocol versions `2025-06-18`, `2025-03-26` and `2024-11-05`). It reads the same endpoint and token settings as the CLI, so pass them as environment variables.

Tools (all annotated read-only):

| Tool | Arguments | Returns |
|---|---|---|
| `describe` | none | The semantic layer (Markdown) |
| `list_apps` | none | Apps, today's date, time zone, currency (JSON) |
| `metric` | `name` (required), `app`, `env`, `from`, `to`, `args` (object of extra string parameters) | Metric result (JSON) |
| `sql` | `sql` (required) | `{columns, rows, truncated}` |
| `user` | `install_id` (required), `app`, `limit` | One user's summary, calendar and events |

The server also sends instructions to the client: call `describe` first, prefer metrics over SQL, default to `env=production`, report absolute counts with ratios, and do not over-interpret small samples.

### Claude Code

```bash
claude mcp add hiwikinsight -e INSIGHT_ENDPOINT=https://insight.example.com -e INSIGHT_TOKEN=<token> -- insight mcp
```

### Claude Desktop

Add to `claude_desktop_config.json` (Settings > Developer > Edit Config):

```json
{
  "mcpServers": {
    "hiwikinsight": {
      "command": "insight",
      "args": ["mcp"],
      "env": {
        "INSIGHT_ENDPOINT": "https://insight.example.com",
        "INSIGHT_TOKEN": "<token>"
      }
    }
  }
}
```

If the client cannot find `insight`, use the absolute path (for example `/Users/you/go/bin/insight`).

### Cursor and other MCP clients

Cursor reads the same `mcpServers` format from `.cursor/mcp.json` (per project) or `~/.cursor/mcp.json` (global). Any client that can launch a stdio MCP server works with the same command, arguments and environment.

## 3. HTTP API

For your own agents, use the [query API](query-api.md) directly:

```bash
curl -s -H "Authorization: Bearer $TOKEN" https://insight.example.com/v1/query/describe
curl -s -H "Authorization: Bearer $TOKEN" "https://insight.example.com/v1/query/metric/retention?app=pawprint&from=2026-09-01&to=2026-09-30"
curl -s -H "Authorization: Bearer $TOKEN" -d '{"sql":"SELECT COUNT(*) FROM v_user_summary WHERE app = '\''pawprint'\'' AND is_new = 1"}' https://insight.example.com/v1/query/sql
```

A minimal tool set for a custom agent mirrors the MCP tools: `describe` (GET describe, put the result in the system prompt or first tool result), `metric` (GET metric with query parameters), `sql` (POST sql), `user` (GET user).

## Prompting tips

- Name the app and period: "For pawprint, production, September 2026, ...". Otherwise the default app and the last 30 days are used.
- Ask for the definition: "use the retention metric and quote its definition". Metric definitions are fixed and returned with every result.
- Ask for counts with ratios: "show absolute numbers next to percentages". Indie-scale samples are small.
- Ask for the commands or SQL it ran, so you can reproduce the answer.
- Good questions: "Do users come back every day? Use activity_days.", "Which module do resurrected users open first?", "Compare D7 retention of users who created an entry on day one vs. those who did not.", "Which paywall context converts best, and how much verified revenue did each bring?", "Which errors affect the most users this week?", "How do sessions start: icon, widget or notification?"
- If `describe` lists undocumented events, ask the assistant to propose catalog entries, then review them.

## Safety

- The query API cannot modify data: SQL is limited to one `SELECT`/`WITH` statement and runs on a read-only SQLite connection (`mode=ro`, `query_only`). The MCP tools are annotated `readOnlyHint`.
- Results are capped at 1000 rows and 10 seconds per query.
- The data is anonymous (install ids, no personal data, no IPs), but it is still your business data: give tokens only to assistants you trust, and remove a token from `query.tokens` to revoke access.
- Dashboard-only endpoints (`/v1/admin/*`) do not accept query tokens.

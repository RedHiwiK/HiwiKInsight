# Event catalog

Each app can have an event catalog: a YAML file that documents its modules, screens, events and parameters. It is written for people and for AI assistants. HiwiKInsight shows it verbatim in `describe`, so an agent knows what `entry.created` or `limit.hit` means before it writes a query.

Configure it per app (the path is relative to the config file):

```yaml
apps:
  - key: pawprint
    bundle_id: com.example.pawprint
    catalog: catalogs/pawprint.yaml
```

Complete examples: [examples/catalogs/](../examples/catalogs/) (`pawprint.yaml`, `ledgerly.yaml`, `trailmark.yaml`).

## Format

The file is YAML-shaped text. Free-form comments are welcome and are passed through to `describe`. Only a few fields are parsed by `internal/catalog`, and they are matched by line, so keep the indentation exactly as shown (two spaces per level):

```yaml
# Event catalog for Pawprint, a pet journal.
# Anything in comments is shown to people and AI assistants.

app: pawprint
display_name: Pawprint

modules:
  timeline: Journal timeline, entry details and search
  entry: Creating and editing journal entries
  paywall: Pawprint Pro purchase screen

screens:
  timeline: Home timeline of journal entries (timeline)
  new_entry: Entry editor (entry)
  paywall: Pawprint Pro offer (paywall)

events:
  - name: entry.created
    desc: A journal entry was saved
    params: {module: entry, type: "daily, meal, walk, weight, vet or custom", photos: "0, 1 or 2-5"}
  - name: paywall.shown
    desc: The Pro paywall was presented
    params: {context: "record_limit, settings_banner, export_limit or onboarding"}
  - name: limit.hit
    desc: A free user reached a free-tier limit
    params: {limit: "entries, pets or export", context: paywall context shown next}
```

| Field | Parsed | Rule | Used for |
|---|---|---|---|
| `modules:` | yes | Top-level line `modules:`, then lines `  key: description` (key `[a-z0-9_]+`) until the next top-level line | Module names in the dashboard (Modules page and elsewhere), returned by `/v1/query/meta` |
| `events:` entries | yes | A line `  - name: event.name` (name `[a-z0-9_.]+`), optionally followed directly by `    desc: text` | Documented-event list (for the undocumented-events check) and event descriptions in the dashboard |
| `display_name:` | parsed, currently unused | Top-level line | |
| `app:`, `screens:`, `params:` and comments | no | Free-form | Read by people and AI through `describe` |

Rules of thumb:

- Document every event the app sends, including SDK built-ins you rely on (`session.started`, `screen.viewed`, `purchase.started`, `error`, ...).
- Under `params`, list each key with its meaning and its possible values or buckets.
- Put the module key on screens (`(timeline)`) so screen-to-module mapping is obvious.
- Write descriptions in the language your team and your AI assistant use; the dashboard shows them as written.

## How `describe` uses it

`GET /v1/query/describe` (also `insight describe` and the MCP `describe` tool) returns Markdown with:

1. Conventions, tables and views.
2. The configured apps and the built-in metrics with their definitions.
3. Data on hand: events, installs, first and last day per app and environment.
4. **Undocumented events**: every `app / event name` stored in `events` but missing from that app's catalog, with its count. Treat this list as a to-do: either add the event to the catalog or stop sending it.
5. **Event catalog: `<app>`**: the full catalog text of each app, in a fenced `yaml` block.

Apps without a catalog simply have no catalog section, and all their events are listed as undocumented.

## Updating

The catalog is read at startup. After editing it, restart the server (`docker compose restart hiwikinsight` or `sudo systemctl restart hiwikinsight`). `hiwikinsight check-config` verifies that every catalog file exists.

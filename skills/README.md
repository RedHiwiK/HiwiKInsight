# Skills

AI assistant skills for working with a HiwiKInsight server.

| Skill | What it does |
| --- | --- |
| [`hiwikinsight-analytics`](hiwikinsight-analytics/SKILL.md) | Teaches Claude Code how to answer app analytics questions (active users, retention, funnels, revenue, module usage, user journeys) with the `insight` CLI. |

## Prerequisites

1. Install the CLI and put it on your `PATH`:

   ```bash
   go install github.com/RedHiwiK/HiwiKInsight/cmd/insight@latest
   ```

2. Point it at your server and give it a query token (one of the tokens listed under `query.tokens` in the server config):

   ```bash
   export INSIGHT_ENDPOINT=https://insight.example.com
   mkdir -p ~/.config/hiwikinsight && echo '<token>' > ~/.config/hiwikinsight/token
   insight apps   # quick check
   ```

## Install the skill (Claude Code)

Copy the skill directory into either your personal skills folder (available in every project):

```bash
mkdir -p ~/.claude/skills
cp -r skills/hiwikinsight-analytics ~/.claude/skills/
```

or into a project's skills folder (shared with everyone working on that repository):

```bash
mkdir -p .claude/skills
cp -r skills/hiwikinsight-analytics .claude/skills/
```

Claude Code picks the skill up automatically and uses it when you ask questions such as "what is our D7 retention this month?" or "which screens do returning users open first?".

## Other AI clients: MCP

Clients that support the Model Context Protocol (Claude Desktop, IDE assistants and others) can use the built-in MCP server instead of the skill. It runs over stdio and exposes the tools `describe`, `list_apps`, `metric`, `sql` and `user`:

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

For Claude Code you can also register it with:

```bash
claude mcp add hiwikinsight -e INSIGHT_ENDPOINT=https://insight.example.com -e INSIGHT_TOKEN=<token> -- insight mcp
```

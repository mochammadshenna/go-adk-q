# How to Connect to Claude Desktop

This guide shows you how to configure Claude Desktop to use dev-task-pubsite as an MCP server.

## Prerequisites

- Claude Desktop installed and signed in
- dev-task-pubsite binary built: `make build`
- At least one of `YOUTRACK_TOKEN` or `GITHUB_TOKEN` available

---

## Step 1 — Find the config file

| OS | Path |
|---|---|
| macOS | `~/Library/Application Support/Claude/claude_desktop_config.json` |
| Windows | `%APPDATA%\Claude\claude_desktop_config.json` |
| Linux | `~/.config/Claude/claude_desktop_config.json` |

If the file doesn't exist, create it.

---

## Step 2 — Add the server entry

```json
{
  "mcpServers": {
    "dev-task-pubsite": {
      "command": "/absolute/path/to/dev-task-pubsite/.build/dev-task-pubsite",
      "args": ["stdio"],
      "env": {
        "YOUTRACK_URL": "https://youtrack.yourcompany.com",
        "YOUTRACK_TOKEN": "perm:your-permanent-token",
        "GITHUB_TOKEN": "ghp_yourtoken",
        "GITHUB_OWNER": "your-org-or-username"
      }
    }
  }
}
```

> **Important**: use the absolute path to the binary, not a relative path. Claude Desktop starts the process from a different working directory.

---

## Step 3 — Restart Claude Desktop

Quit and relaunch Claude Desktop. The MCP server starts automatically in stdio mode when Claude Desktop opens.

---

## Step 4 — Verify tools are available

In a new conversation, type:

> What tools do you have available?

Claude should list the `yt_*` and `gh_*` tools, along with the `ui://task-dashboard` resource.

If no tools appear, open the Claude Desktop developer console (macOS: **Help → Show Logs**) and look for errors from dev-task-pubsite.

---

## Step 5 — Test a tool

> List the open pull requests for repo sentinel-tech/sentec-pms

Claude will call `list_pull_requests` and return a formatted response.

---

## Using the dashboard resource

The dashboard is available as an MCP App resource:

> Show me the task dashboard

Claude will render the interactive sprint board and PR timeline inline in the conversation.

---

## Partial configuration

If you only have one provider configured, the missing provider's tools simply won't appear — the server starts cleanly in degraded mode. For example, if only `GITHUB_TOKEN` is set:

- `gh_*` tools: available ✓
- `yt_*` tools: absent (not registered)
- Dashboard: available ✓ (always registered)

---

## Updating the binary

After pulling new code and rebuilding:

```bash
git pull
make build
```

Restart Claude Desktop to pick up the new binary.

---

## Running alongside other MCP servers

dev-task-pubsite uses stdio mode for Claude Desktop, so it doesn't need a port. Multiple MCP servers can run simultaneously without port conflicts. However, avoid assigning the same `mcpServers` key name to two different servers.

---

## Related

- [Getting started tutorial →](../tutorials/getting-started.md)
- [Configure YouTrack →](configure-youtrack.md)
- [Configure GitHub →](configure-github.md)
- [Environment variables →](../reference/environment-variables.md)

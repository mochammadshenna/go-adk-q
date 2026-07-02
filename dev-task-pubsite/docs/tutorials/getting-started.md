# Getting Started with dev-task-pubsite

This tutorial walks you through running dev-task-pubsite from source for the first time. By the end, you will have a working MCP server connected to at least one provider (YouTrack or GitHub) and visible inside Claude Desktop.

## What you will build

A locally running MCP server that:
- Exposes 10 tools bridging YouTrack task management and GitHub code activity
- Serves an interactive dashboard at `ui://task-dashboard`
- Runs in either **stdio mode** (for Claude Desktop) or **HTTP mode** (for MCP Inspector / Postman)

## Prerequisites

| Requirement | Version | Check |
|---|---|---|
| Go | ≥ 1.22 | `go version` |
| Git | any | `git --version` |
| Claude Desktop | latest | Installed and signed in |
| YouTrack instance | any (optional) | You need a permanent token |
| GitHub account | any (optional) | You need a personal access token |

You need **at least one** of YouTrack or GitHub to make any tools work. The server starts cleanly without either, but all tools will be absent.

---

## Step 1 — Clone and build

```bash
git clone https://github.com/msw/dev-task-pubsite
cd dev-task-pubsite
go build -o .build/dev-task-pubsite ./cmd/dev-task-pubsite
```

The binary is now at `.build/dev-task-pubsite`.

---

## Step 2 — Set environment variables

Create a `.env` file (never committed — already in `.gitignore`):

```bash
# YouTrack — leave blank if you don't have a YouTrack instance
export YOUTRACK_URL=https://youtrack.yourcompany.com
export YOUTRACK_TOKEN=perm:your-permanent-token

# GitHub — leave blank if you don't have a token
export GITHUB_TOKEN=ghp_yourtoken
export GITHUB_OWNER=your-org-or-username

# Optional
export PORT=9012
export DEBUG=1
```

Source it:

```bash
source .env
```

---

## Step 3 — Run in HTTP mode to verify

HTTP mode lets you test with a browser or MCP Inspector before wiring up Claude Desktop.

```bash
make dev-http
# or: .build/dev-task-pubsite http
```

You should see output like:

```
[task-pubsite:http] Dev Task Pubsite MCP server listening on :9012
[task-pubsite:http]   Dashboard: http://localhost:9012/dashboard
[task-pubsite:http]   MCP:       http://localhost:9012/mcp
[task-pubsite:http]   Health:    http://localhost:9012/health
```

Open `http://localhost:9012/health` in a browser. You should see:

```json
{"status":"ok","version":"dev","youtrack":true,"github":true}
```

If a provider shows `false`, check that its environment variables are set and the remote is reachable.

---

## Step 4 — Try a tool with MCP Inspector

If you have [MCP Inspector](https://github.com/modelcontextprotocol/inspector) installed:

```bash
npx @modelcontextprotocol/inspector http://localhost:9012/mcp
```

In the Inspector UI:
1. Click **Tools**
2. Select `repo_stats`
3. Enter `owner: octocat` and `repo: Hello-World`
4. Click **Run**

You should see repository statistics returned with a `providerStatus` block:

```json
{
  "stats": { "stars": 1924, "forks": 2700, ... },
  "providerStatus": { "source": "github", "degraded": false, "staleAge": "" }
}
```

---

## Step 5 — Connect Claude Desktop

Stop the HTTP server (`Ctrl+C`), then configure Claude Desktop for stdio mode.

Edit `~/Library/Application Support/Claude/claude_desktop_config.json` (macOS):

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

Restart Claude Desktop. You should see the dev-task-pubsite tools available in the tool list.

Try: *"List open pull requests for repo myorg/myrepo"*

---

## Step 6 — Open the dashboard

In a Claude Desktop conversation, ask:

> Show me the task dashboard

Claude will read the `ui://task-dashboard` resource and render the interactive sprint board and PR timeline inline.

---

## What's next?

- [Configure YouTrack connection →](../how-to/configure-youtrack.md)
- [Configure GitHub connection →](../how-to/configure-github.md)
- [Connect to Claude Desktop →](../how-to/connect-claude-desktop.md)
- [All 10 tools reference →](../reference/tools.md)

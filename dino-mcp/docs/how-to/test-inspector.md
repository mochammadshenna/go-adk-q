# How-to: Test with MCP Inspector

> **Interactively debug all MCP protocol methods — tools, resources, and lifecycle.**
> The [MCP Inspector](https://github.com/modelcontextprotocol/inspector) is the official debugging UI for MCP servers.
> It connects via Streamable HTTP and lets you call any tool, read any resource, and inspect JSON-RPC messages.

**Official references:**
- [MCP Inspector — GitHub](https://github.com/modelcontextprotocol/inspector)
- [MCP Spec — JSON-RPC Messages](https://spec.modelcontextprotocol.io/specification/2025-03-26/basic/messages/)
- [MCP Spec — Lifecycle](https://spec.modelcontextprotocol.io/specification/2025-03-26/basic/lifecycle/)

---

## Quick Start

```bash
make test-inspector
```

This runs two things:
1. `make build-fast` — ensures the binary is current
2. `bin/dino-mcp http -addr :9010` — starts the server in background
3. `npx @modelcontextprotocol/inspector` — launches Inspector at http://localhost:5173

> If port 5173 is in use, kill the existing Inspector process first:
> `lsof -ti:5173 | xargs kill`

---

## Step-by-Step Walkthrough

### 1. Connect

1. Open http://localhost:5173
2. In the **Transport** field, select `Streamable HTTP` (should be default)
3. In the **URL** field, enter: `http://localhost:9010/mcp`
4. Click **Connect**

**Expected:** The status indicator turns green, and a session is established.

**What just happened (the MCP lifecycle):**

The Inspector sent two JSON-RPC messages automatically:
1. `initialize` with protocol version `2025-11-05` (from the MCP spec)
2. `notifications/initialized` to confirm readiness

These are handled by `mcp.NewStreamableHTTPHandler()` in `internal/server/server.go` (line 87).

### 2. Explore Tools

Click the **Tools** tab. You should see three tools:

| Tool | Description | Arguments |
|------|-------------|-----------|
| `dino_think` | Random dinosaur fact | (none) |
| `dino_ask` | Dinosaur Q&A | `question` (string, required) |
| `dino_dashboard` | Interactive dashboard | `filter` (string, optional) |

**How the server registers these** (from `internal/server/server.go` `New()` function):

```go
tools.RegisterThink(s)        // zero-arg text tool
tools.RegisterAsk(s)          // single-arg text tool
tools.RegisterDashboardTool(s) + resources.RegisterDashboardResource(s, logger)  // MCP App tool + resource
```

### 3. Call a Tool

Click `dino_think` → click **Call Tool** → see the response:

```json
{
  "content": [
    {
      "type": "text",
      "text": "🦕 Did you know? The Velociraptor was only about the size of a turkey!"
    },
    {
      "type": "structured",
      "structuredContent": {
        "fact": "The Velociraptor was only about the size of a turkey",
        "species": "Velociraptor"
      }
    }
  ]
}
```

**Notice:** The response has TWO content items:
- `type: text` — human-readable text (Markdown-supported)
- `type: structured` — structured JSON for typed clients

This is the dual-output pattern used by all tools in this project. See `think_tool.go` lines 28-48 for the implementation.

### 4. Test the MCP App Tool

Click `dino_dashboard` → in the arguments JSON, enter:

```json
{
  "filter": "Carnivore"
}
```

Click **Call Tool**.

**Observe the response:**
```json
{
  "content": [
    {
      "type": "text",
      "text": "Displaying dinosaur dashboard with 4 dinosaurs (filter: Carnivore)"
    }
  ],
  "structuredContent": {
    "filter": "Carnivore",
    "dinosaurs": [
      {
        "name": "Tyrannosaurus Rex",
        "diet": "Carnivore",
        ...
      }
    ],
    "timestamp": "2026-06-21T..."
  }
}
```

**What makes this an "MCP App"** — look at the tool definition in `dashboard.go` line ~64:

```go
mcp.AddTool(s, &mcp.Tool{
    Name: "dino_dashboard",
    // ...
    Meta: mcp.Meta{
        "ui": map[string]any{
            "resourceUri": "ui://dino-dashboard/mcp-app.html",  // ← THIS
        },
    },
}, /* handler */)
```

The `_meta.ui.resourceUri` field tells the MCP host (like Claude Desktop) that this tool has an associated HTML UI. When the host detects this, it:
1. Calls `resources/read` on the URI → gets back HTML
2. Renders the HTML in a sandboxed iframe
3. The iframe communicates via postMessage using the ext-apps SDK

### 5. Read the HTML Resource

Go to the **Resources** tab → you should see:

```
ui://dino-dashboard/mcp-app.html  —  Dino Dashboard
```

Click it → **Read Resource**.

**Expected response:**
```json
{
  "contents": [
    {
      "uri": "ui://dino-dashboard/mcp-app.html",
      "mimeType": "text/html;profile=mcp-app",
      "text": "<!DOCTYPE html><html>..."
    }
  ]
}
```

**The MIME type is critical:** `text/html;profile=mcp-app` is what identifies this as an MCP App resource (not plain HTML). Without the `profile=mcp-app` suffix, the host would treat it as a regular document instead of an interactive view.

This MIME type is defined as a constant in `dashboard.go` line 19:

```go
const RESOURCE_MIME_TYPE = "text/html;profile=mcp-app"
```

### 6. Run the Integration Tests

Open a third terminal to run the full test suite while Inspector is connected:

```bash
make test
```

This runs 7 protocol-level tests. Each test sends a raw JSON-RPC message and validates the response. The test script (`test_mcp.sh`) exercises:

| Test | JSON-RPC Method | What it validates |
|------|----------------|-------------------|
| 1 | `initialize` | Server responds with protocol version + session ID |
| 2 | `notifications/initialized` | Server accepts (200 OK) |
| 3 | `tools/list` | `dino_dashboard` has `_meta.ui.resourceUri` |
| 4 | `tools/call dino_think` | Structured content has `fact` + `species` |
| 5 | `tools/call dino_dashboard` | Filtered list returned correctly |
| 6 | `resources/read` | MIME type is `text/html;profile=mcp-app` |
| 7 | `tools/call dino_ask` | Question + Answer returned |

---

## What to Check for Each Issue

| Symptom | Likely Cause | Fix |
|---------|-------------|-----|
| Inspector can't connect (no response) | Server not running or wrong URL | `make dev-http` and check `http://localhost:9010/mcp` |
| CORS error in browser console | OPTIONS preflight not handled | Check `corsMiddleware()` in `server.go` |
| Tool returns empty response | Handler returned nil content | Check handler returns `&mcp.CallToolResult{Content: []mcp.Content{...}}` |
| Structured data missing | Result type not set | Ensure handler returns a valid Result type |
| UI not rendering (Claude Desktop) | Missing `_meta.ui.resourceUri` or wrong MIME type | Check `Meta` in `dashboard.go` line ~64 and `RESOURCE_MIME_TYPE` |
| Session ID changes on every request | Wrong transport mode | Use stdio for Claude Desktop, HTTP for Inspector |
| 403 Forbidden | DNS rebinding protection | Set `DisableLocalhostProtection: true` in `StreamableHTTPOptions` (already set in `server.go` line 89) |
| `extract_sid` fails in tests | grep matches CORS header | Anchor to `^Mcp-Session-Id:` (already fixed in `test_mcp.sh`) |

---

## Related

- [`tutorials/get-started.md`](../tutorials/get-started.md) — first-time setup
- [`reference/cli.md`](../reference/cli.md) — all CLI flags and env vars
- [`explanation/architecture.md`](../explanation/architecture.md) — how MCP protocol and Apps protocol work

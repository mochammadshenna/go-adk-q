# Tutorial: Get Started with dino-mcp

> **Zero to running in 5 minutes.** This tutorial walks through every step — build, run, test, and connect to Claude Desktop.
> Along the way you'll learn how the MCP Go SDK, Gin router, and embedded UI fit together.

**Official references:**
- [MCP Spec — Tools](https://spec.modelcontextprotocol.io/specification/2025-03-26/server/tools/)
- [MCP Spec — Transports](https://spec.modelcontextprotocol.io/specification/2025-03-26/basic/transports/)
- [MCP Spec — Streamable HTTP](https://spec.modelcontextprotocol.io/specification/2025-03-26/basic/transports/#streamable-http)
- [MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk)

---

## Prerequisites

| Tool | Version | Why |
|------|---------|-----|
| [Go](https://go.dev/dl/) | ≥ 1.24 | Compiles the server binary |
| `make` | any | Build automation (Mac: `xcode-select --install`) |
| `curl` | any | Protocol-level testing |
| (optional) `cloudflared` | any | Remote tunnel (`brew install cloudflared`) |

No API keys required. All data is built into the binary.

---

## 1. Clone and Inspect the Structure

```bash
git clone <your-repo> dino-mcp
cd dino-mcp

# Look at how the project is organized
ls -la cmd/dino-mcp/main.go     # CLI entry point
ls -la internal/          # server/ (composition root), tools/, resources/
ls -la ui/src/mcp-app.ts         # UI source (TypeScript + ext-apps SDK)
ls -la Makefile                   # Build automation
```

**Key architectural insight** (from `internal/server/server.go` lines 30-67):
The `New()` function creates an `mcp.Server`, then registers three tools and one resource. The same `mcp.Server` instance works with both `stdio` and `Streamable HTTP` transports — the transport is selected at runtime by the CLI subcommand.

```go
// internal/server/server.go — New() function
s := mcp.NewServer(&mcp.Implementation{Name: "dino-mcp", Version: Version}, ...)

// Tools are registered on the same server instance
tools.RegisterThink(s)      // text-only tool
tools.RegisterAsk(s)         // text-only tool with args
tools.RegisterDashboardTool(s) + resources.RegisterDashboardResource(s, logger)  // MCP App tool + resource
```

---

## 2. Build

Two build modes. Start with the fast one:

```bash
# Fast build — Go binary only (reuses existing bundled UI)
make build-fast
# Output: bin/dino-mcp (~11MB, Mach-O 64-bit arm64)

# Or full build — rebuilds Vite UI from TypeScript source, then Go binary
make build
# This takes ~10s because it runs npm ci + vite build first
```

**What happens during `make build-fast`** (from `Makefile`):
```makefile
build-fast: build-go
build-go:
  go build -ldflags="$(LDFLAGS)" -o bin/dino-mcp ./cmd/dino-mcp/
```

The `//go:embed` directive in `internal/resources/dashboard.go` compiles the HTML straight into the binary:

```go
//go:embed dashboard_ui.html
var uiFS embed.FS  // 354KB HTML, CSS, JS — all in one binary
```

---

## 3. Run

Start the server in Streamable HTTP mode:

```bash
make dev-http
```

**Expected output:**
```
=== dino-mcp server ===
Version: bbc44db
Transport: http
Listening on :9010
```

**What happens** (from `cmd/dino-mcp/main.go` lines 65-72):
```go
func runHTTP(addr string, verbose bool) {
    logger := newLogger(verbose)
    s := server.New(logger)          // create server with all tools
    ctx, cancel := signalCtx()        // SIGINT/SIGTERM aware context
    server.RunStreamableHTTP(ctx, s, addr, logger)  // start Gin on :9010
}
```

The `RunStreamableHTTP` function (in `server.go` lines 77-129):
1. Creates a Gin engine with recovery + CORS middleware
2. Creates `mcp.NewStreamableHTTPHandler()` — the MCP protocol handler
3. Mounts it at `/mcp` via `gin.WrapH()`
4. Adds standalone routes: `/dashboard`, `/api/dinosaurs`, `/health`
5. Starts the HTTP server on `:9010`

---

## 4. Test at the Protocol Level

Open a second terminal and run the integration test suite:

```bash
make test
```

**Expected output (last 5 lines):**
```
==========================================
✅ ALL INTEGRATION TESTS PASSED
==========================================
```

**What the tests cover** (from `test_mcp.sh`):

| Step | Method | What it validates | Source reference |
|------|--------|-------------------|------------------|
| 1 | `initialize` | Protocol handshake, session ID creation | `mcp.Server` → `Initialize` handler |
| 2 | `notifications/initialized` | Server accepts lifecycle notification | `mcp.Server` lifecycle |
| 3 | `tools/list` | All 3 tools listed, `dino_dashboard` has `_meta.ui.resourceUri` | `tools.RegisterDashboardTool() + resources.RegisterDashboardResource()` |
| 4 | `tools/call dino_think` | Random fact returned in `structuredContent` | `internal/tools/think.go` `tools.RegisterThink()` |
| 5 | `tools/call dino_dashboard` | Filtered dinosaur list returned | `internal/tools/dashboard.go` `FilteredDinosaurs()` |
| 6 | `resources/read` | HTML returned with MIME type `text/html;profile=mcp-app` | `internal/resources/dashboard.go` resource handler |
| 7 | `tools/call dino_ask` | Question + answer returned | `internal/tools/ask.go` `tools.RegisterAsk()` |

---

## 5. Open the Dashboard

Open http://localhost:9010/dashboard in your browser.

This is the **standalone fallback** — the same HTML that gets served via MCP Apps protocol in Claude Desktop, but fetched directly via HTTP GET. It calls `fetch('/api/dinosaurs')` to populate the cards.

Behind the scenes (from `server.go` lines 103-116):
```go
// Standalone route — serves embedded HTML
r.GET("/dashboard", func(c *gin.Context) {
    htmlData, err := uiFS.ReadFile("dashboard_ui.html")
    c.Data(http.StatusOK, "text/html; charset=utf-8", htmlData)
})

// REST API fallback — returns JSON for the standalone UI
r.GET("/api/dinosaurs", func(c *gin.Context) {
    filter := c.Query("filter")
    dinos := tools.FilteredDinosaurs(filter)
    c.JSON(http.StatusOK, gin.H{"dinosaurs": dinos, ...})
})
```

---

## 6. Test with MCP Inspector (Interactive Debugger)

```bash
make test-inspector
```

This starts the server and launches the MCP Inspector at http://localhost:5173.

In the Inspector:
1. **Transport Type**: leave as default
2. **URL**: `http://localhost:9010/mcp`
3. Click **Connect**
4. Go to **Tools** tab — see `dino_think`, `dino_ask`, `dino_dashboard`
5. Click `dino_think` → **Call Tool** → see JSON response
6. Go to **Resources** tab — see `ui://dino-dashboard/mcp-app.html`
7. Click → **Read Resource** — see the full HTML

---

## 7. Connect to Claude Desktop

Edit `~/Library/Application Support/Claude/claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "dino-mcp": {
      "command": "/ABSOLUTE/PATH/TO/dino-mcp/bin/dino-mcp",
      "args": ["stdio"]
    }
  }
}
```

Replace `/ABSOLUTE/PATH/TO/` with the real path from `pwd`.

Restart Claude Desktop. Look for the hammer icon (🔨) in the chat input — that means your tools are loaded.

**Try these prompts:**
- "Tell me a random dinosaur fact"
- "Show me the dinosaur dashboard with carnivores"
- "What did Triceratops eat?"
- "Show me the dashboard filtered by Jurassic period"

---

## What You Learned

| Concept | How it's used in this project |
|---------|-------------------------------|
| `mcp.Server` | Created in `New()`, reused across transports |
| `mcp.AddTool[T,A,R]()` | Generic registration with typed args/results |
| `mcp.NewStreamableHTTPHandler()` | HTTP transport wrapping |
| `gin.WrapH()` | Bridges `http.Handler` → Gin handler |
| `//go:embed` | Compiles 354KB HTML into the binary |
| `_meta.ui.resourceUri` | Links a tool to its MCP App HTML resource |
| `text/html;profile=mcp-app` | MIME type for MCP App resources |
| `App` class + `PostMessageTransport` | ext-apps SDK for postMessage protocol |
| `ui/initialize` handshake | View registers with the MCP host |

---

## Next Steps

| If you want to... | Read this |
|---|---|
| Add a new tool | [`tutorials/first-tool.md`](first-tool.md) |
| Add a dinosaur species | [`how-to/add-dinosaur.md`](../how-to/add-dinosaur.md) |
| Understand MCP Apps protocol in depth | [`explanation/architecture.md`](../explanation/architecture.md) |
| See CLI reference | [`reference/cli.md`](../reference/cli.md) |

# SYSTEM.md — dino-mcp System Architecture

> **Architecture deep-dive for AI agents and developers.** Understand every layer, data flow, and interaction pattern.

---

## System Overview

```mermaid
C4Context
  title System Context — dino-mcp

  Person(user, "End User", "Interacts via Claude Desktop, browser, or MCP Inspector")
  
  System_Boundary(dino, "dino-mcp Server") {
    Container(mcp, "MCP Protocol Layer", "Go SDK", "Handles JSON-RPC, transports, lifecycle")
    Container(go_server, "Go Server", "Gin + Go MCP SDK", "Tool registration, resource serving, API endpoints")
    Container(html_ui, "Dashboard UI", "Vite + ext-apps SDK", "Bundled HTML/JS/CSS MCP App View")
  }
  
  System_Ext(inspector, "MCP Inspector", "Web UI for testing")
  System_Ext(claude, "Claude Desktop", "MCP client with ext-apps support")
  System_Ext(tunnel, "Cloudflare Tunnel", "Public HTTPS access")
  
  Rel(user, claude, "Uses")
  Rel(user, tunnel, "Opens in browser")
  Rel(user, inspector, "Tests via")
  Rel(claude, mcp, "stdio", "JSON-RPC")
  Rel(tunnel, mcp, "HTTPS", "Streamable HTTP")
  Rel(mcp, go_server, "Routes to", "Gin handlers")
  Rel(go_server, html_ui, "Embeds", "//go:embed")
```

---

## Architecture Layers

```mermaid
C4Container
  title Container Diagram — dino-mcp Layers

  Container_Boundary(cli, "CLI Layer (cmd/dino-mcp/main.go)") {
    Component(stdio_cmd, "stdio subcommand", "Go", "Starts stdio transport")
    Component(http_cmd, "http subcommand", "Go", "Starts Gin HTTP server")
  }
  
  Container_Boundary(server, "Server Layer (internal/server/)") {
    Component(new_fn, "New()", "Go", "Creates MCP server with tools & resources")
    Component(gin_router, "Gin Router", "Go", "Routes /mcp, /dashboard, /api/dinosaurs, /health")
    Component(cors, "CORS Middleware", "Go", "Handles OPTIONS preflight, Access-Control headers")
    Component(mcp_handler, "StreamableHTTPHandler", "Go SDK", "JSON-RPC over HTTP")
  }
  
  Container_Boundary(tools, "Tool Layer") {
    Component(think, "dino_think", "Go", "Random dinosaur fact")
    Component(ask, "dino_ask", "Go", "Curated dinosaur Q&A")
    Component(dashboard, "dino_dashboard", "Go + HTML", "MCP App with UI")
  }
  
  Container_Boundary(ui, "UI Layer") {
    Component(embedded, "dashboard_ui.html", "HTML/JS/CSS", "Bundled Vite output")
    Component(ext_apps, "@modelcontextprotocol/ext-apps", "TS SDK", "App class, postMessage protocol")
  }
  
  Rel(new_fn, think, "Register tool")
  Rel(new_fn, ask, "Register tool")
  Rel(new_fn, dashboard, "Register tool + resource")
  Rel(dashboard, embedded, "Serves via resources/read")
  Rel(gin_router, mcp_handler, "Route /mcp")
  Rel(gin_router, cors, "Wraps all routes")
```

---

## Request Flow

```mermaid
sequenceDiagram
  participant Client as MCP Client (Claude Desktop)
  participant HTTP as Gin Router (:9010)
  participant MCP as StreamableHTTPHandler
  participant Server as mcp.Server
  participant Tool as Tool Handler
  participant Resource as Resource Handler

  Note over Client,Resource: INITIALIZATION
  Client->>HTTP: POST /mcp (initialize)
  HTTP->>MCP: gin.WrapH(handler)
  MCP->>Server: Handle JSON-RPC
  Server-->>MCP: InitializeResult (session ID)
  MCP-->>HTTP: HTTP 200 + SSE/JSON
  HTTP-->>Client: Mcp-Session-Id header

  Note over Client,Resource: TOOL LISTING
  Client->>HTTP: POST /mcp (tools/list)
  HTTP->>MCP: gin.WrapH(handler)
  MCP->>Server: List tools
  Server-->>MCP: tools (with _meta.ui on dino_dashboard)
  MCP-->>Client: Response

  Note over Client,Resource: TOOL CALL (dino_dashboard)
  Client->>HTTP: POST /mcp (tools/call dino_dashboard)
  HTTP->>MCP: gin.WrapH(handler)
  MCP->>Server: Execute tool
  Server->>Tool: Call handler
  Tool-->>Server: DinoDashboardResult
  Server-->>MCP: CallToolResult
  MCP-->>Client: Result + structuredContent

  Note over Client,Resource: UI RESOURCE FETCH
  Client->>HTTP: POST /mcp (resources/read)
  HTTP->>MCP: gin.WrapH(handler)
  MCP->>Server: Read resource
  Server->>Resource: Read embedded HTML
  Resource-->>Server: dashboard_ui.html content
  Server-->>MCP: ResourceContents (text/html;profile=mcp-app)
  MCP-->>Client: HTML with postMessage protocol

  Note over Client,Resource: STANDALONE BROWSER
  Browser->>HTTP: GET /dashboard
  HTTP->>Server: Gin handler
  Server->>Resource: Read embedded HTML
  Resource-->>Server: dashboard_ui.html content
  Server-->>Browser: HTML page
  Browser->>HTTP: GET /api/dinosaurs?filter=Carnivore
  HTTP->>Server: Gin handler
  Server-->>Browser: JSON dinosaur data
```

---

## MCP Apps Protocol (View ↔ Host)

```mermaid
sequenceDiagram
  participant View as View (iframe)
  participant Host as MCP Host (Claude Desktop)
  participant Server as MCP Server
  
  Note over View,Server: 1. Tool activated by LLM
  Host->>Server: tools/call dino_dashboard
  Server-->>Host: CallToolResult
  
  Note over View,Server: 2. Host renders iframe with HTML resource
  Host->>Server: resources/read ui://dino-dashboard/mcp-app.html
  Server-->>Host: HTML content
  
  Note over View,Server: 3. View initializes via postMessage
  View->>Host: {jsonrpc:"2.0", method:"ui/initialize", params:{appCapabilities:{...}}}
  Host-->>View: {jsonrpc:"2.0", result:{hostContext:{...}}}
  View->>Host: {jsonrpc:"2.0", method:"ui/notifications/initialized"}
  
  Note over View,Server: 4. Host sends tool data to View
  Host->>View: {jsonrpc:"2.0", method:"ui/notifications/tool-input", params:{arguments:{filter:"Carnivore"}}}
  Host->>View: {jsonrpc:"2.0", method:"ui/notifications/tool-result", params:{structuredContent:{dinosaurs:[...]}}}
  
  Note over View,Server: 5. View can call tools on server
  View->>Host: {jsonrpc:"2.0", id:1, method:"tools/call", params:{name:"dino_think", arguments:{}}}
  Host->>Server: tools/call dino_think
  Server-->>Host: CallToolResult
  Host-->>View: {jsonrpc:"2.0", id:1, result:{content:[...]}}
```

---

## Transport Modes

### stdio (Claude Desktop)
```
┌──────────────┐     stdin/stdout     ┌──────────────┐
│ Claude       │◄────────────────────►│ dino-mcp     │
│ Desktop      │   JSON-RPC messages  │ (stdio mode) │
└──────────────┘                      └──────────────┘
```

### Streamable HTTP (Web/MCP Inspector)
```
┌──────────────┐                     ┌──────────────┐
│ MCP Client   │── POST /mcp ──────►│ Gin Router   │
│ (curl,       │◄── HTTP response ──│ (:9010)      │
│  Inspector)  │                     └──────┬───────┘
└──────────────┘                             │
                                     ┌──────▼───────┐
                                     │ Streamable   │
                                     │ HTTP Handler │
                                     └──────────────┘
```

### Cloudflare Tunnel
```
┌──────────────┐    HTTPS     ┌──────────────┐    HTTP     ┌──────────────┐
│ Browser /    │─────────────►│ Cloudflare   │────────────►│ dino-mcp     │
│ Client       │◄─────────────│ Tunnel       │◄────────────│ (:9010)      │
└──────────────┘              └──────────────┘              └──────────────┘
```

---

## Data Model

```mermaid
classDiagram
  class DinoSummary {
    +string name
    +string period
    +string diet
    +string length
    +string weight
    +string funFact
    +string imageStyle
  }
  
  class DinoDashboardResult {
    +string filter
    +DinoSummary[] dinosaurs
    +string timestamp
  }
  
  class DinoThinkResult {
    +string fact
    +string species
  }
  
  class DinoAskResult {
    +string question
    +string answer
    +string species
  }
  
  class DashboardData {
    +string filter
    +DinoSummary[] dinosaurs
    +string timestamp
  }
  
  DinoDashboardResult --> "*" DinoSummary
  DashboardData --> "*" DinoSummary
```

---

## Route Table

| Route | Method | Handler | Purpose |
|-------|--------|---------|---------|
| `/mcp` | ANY | `StreamableHTTPHandler` | MCP JSON-RPC endpoint |
| `/mcp/*any` | ANY | `StreamableHTTPHandler` | MCP JSON-RPC (subpath) |
| `/dashboard` | GET | Serves embedded HTML | Standalone UI |
| `/dashboard/*any` | GET | Serves embedded HTML | Standalone UI (subpath) |
| `/api/dinosaurs` | GET | Filtered JSON data | Standalone API |
| `/api/dinosaurs?filter=` | GET | Filtered JSON data | Standalone API |
| `/health` | GET | JSON status | Health check |

---

## Environment Variables

| Variable | Default | Purpose |
|----------|---------|---------|
| `GIN_MODE` | (none) | Set to `release` for production |
| `DINO_VERBOSE` | (none) | Enable debug logging |
| (none — public binary) | | No API keys required |

---

## Dependencies

### Go (runtime)
| Module | Version | Purpose |
|--------|---------|---------|
| `github.com/modelcontextprotocol/go-sdk` | v1.6.1 | MCP core protocol (tools, resources, transports) |
| `github.com/gin-gonic/gin` | v1.12.0 | HTTP router, middleware, CORS |

### Node.js (build-time)
| Package | Version | Purpose |
|---------|---------|---------|
| `@modelcontextprotocol/ext-apps` | latest | MCP Apps SDK (App class, PostMessageTransport) |
| `vite` | ^6.0.0 | Bundler with singlefile plugin |
| `vite-plugin-singlefile` | latest | Inline all assets into one HTML file |

### External (dev/test)
| Tool | Purpose |
|------|---------|
| `cloudflared` | Public HTTPS tunnel for remote testing |
| `npx @modelcontextprotocol/inspector` | MCP Inspector for interactive debugging |
| `curl` / `python3` | Integration test script |

# Architecture — dino-mcp

> **System architecture, component relationships, and data flow.** For architects, senior engineers, and AI agents.

---

## System Context

```mermaid
C4Context
  title System Context Diagram

  Person(end_user, "End User", "Interacts via chat or browser")
  
  System_Boundary(dino, "dino-mcp") {
    Container(binary, "Go Binary", "Go 1.25 + Gin + MCP SDK", "~11MB static binary with embedded HTML")
    Container(html, "Dashboard UI", "HTML/JS/CSS", "Bundled via Vite + ext-apps SDK")
  }
  
  System_Ext(claude, "Claude Desktop", "MCP client with ext-apps support")
  System_Ext(inspector, "MCP Inspector", "Web-based MCP debugging tool")
  System_Ext(tunnel, "Cloudflare Tunnel", "HTTPS public access")
  
  Rel(end_user, claude, "Chats with")
  Rel(end_user, tunnel, "Opens URL")
  Rel(claude, binary, "stdio transport")
  Rel(inspector, binary, "Streamable HTTP")
  Rel(tunnel, binary, "Streamable HTTP")
  Rel(binary, html, "//go:embed + resources/read")
```

---

## Container Architecture

```mermaid
C4Container
  title Container Diagram

  Container_Boundary(cmd, "CLI Layer (cmd/dino-mcp/main.go)") {
    Component(stdio, "stdio subcommand", "Go", "s.Run(StdioTransport)")
    Component(http, "http subcommand", "Go", "Gin server on :9010")
  }
  
  Container_Boundary(srv, "Server Layer (internal/server/)") {
    Component(new_fn, "New()", "Go", "Creates mcp.Server with tool/resource registration")
    Component(gin, "Gin Engine", "Go", "Router, middleware, route handlers")
    Component(mcp_h, "StreamableHTTPHandler", "Go SDK", "JSON-RPC over HTTP with SSE streaming")
    Component(cors_mw, "CORS Middleware", "Go", "Access-Control headers, OPTIONS → 200")
  }
  
  Container_Boundary(tools, "Tool Layer") {
    Component(think, "dino_think", "Go", "mcp.AddTool → random fact from curated list")
    Component(ask, "dino_ask", "Go", "mcp.AddTool → keyword-based Q&A")
    Component(dash, "dino_dashboard", "Go", "mcp.AddTool + s.AddResource")
  }
  
  Container_Boundary(data, "Data Layer") {
    Component(dinos, "Dinosaur Data", "Go slice", "12 DinoSummary structs with facts")
    Component(embed, "dashboard_ui.html", "//go:embed", "354KB bundled HTML/JS/CSS")
  }
  
  Container_Boundary(ui_src, "UI Source (ui/)") {
    Component(app, "mcp-app.ts", "TypeScript", "App class + PostMessageTransport")
    Component(vite, "Vite", "Build tool", "vite-plugin-singlefile → single HTML")
  }
  
  Rel(new_fn, think, "Registers")
  Rel(new_fn, ask, "Registers")
  Rel(new_fn, dash, "Registers")
  Rel(dash, embed, "Serves via resources/read")
  Rel(gin, mcp_h, "gin.WrapH() at /mcp")
  Rel(gin, cors_mw, "Wraps all routes")
  Rel(embed, app, "Runs in iframe")
  Rel(app, vite, "Built by")
```

---

## Request Lifecycle

```mermaid
sequenceDiagram
  participant U as User/Client
  participant C as CLI (main.go)
  participant G as Gin Router
  participant M as MCP Handler
  participant S as Server
  participant T as Tool/Resource
  
  Note over U,T: INITIALIZATION
  U->>C: dino-mcp http -addr :9010
  C->>G: New Gin engine
  C->>S: server.New() → register tools + resources
  S-->>C: Ready
  
  Note over U,T: FIRST REQUEST
  U->>G: POST /mcp (Content-Type: application/json)
  G->>G: CORS check (OPTIONS → 200 earlier)
  G->>M: gin.WrapH(handler)
  M->>M: Validate Accept header
  M->>M: Check DNS rebinding
  M->>S: Initialize protocol
  S-->>M: Session ID created
  M-->>G: HTTP 200 + Mcp-Session-Id
  G-->>U: Response with session
  
  Note over U,T: TOOL CALL
  U->>G: POST /mcp (tools/call dino_think)
  G->>M: Route /mcp
  M->>S: Execute tool
  S->>T: Call handler
  T-->>S: DinoThinkResult
  S-->>M: CallToolResult + structuredContent
  M-->>G: SSE or JSON response
  G-->>U: Result data
  
  Note over U,T: RESOURCE READ
  U->>G: POST /mcp (resources/read)
  G->>M: Route /mcp
  M->>S: Read resource
  S->>T: Read from embed.FS
  T-->>S: dashboard_ui.html content
  S-->>M: ResourceContents (MIME: text/html;profile=mcp-app)
  M-->>G: HTML content
  G-->>U: HTML for iframe rendering
```

---

## MCP Apps Lifecycle

```mermaid
sequenceDiagram
  participant LLM as LLM Agent
  participant Host as MCP Host (Claude Desktop)
  participant S as dino-mcp Server
  participant V as View (iframe)
  
  Note over LLM,V: 1. LLM decides to show dashboard
  LLM->>Host: "Show dinosaur dashboard"
  Host->>S: tools/call dino_dashboard {filter: "Carnivore"}
  S-->>Host: CallToolResult (text + structuredContent)
  
  Note over LLM,V: 2. Host detects MCP App
  Host->>S: resources/read ui://dino-dashboard/mcp-app.html
  S-->>Host: HTML (text/html;profile=mcp-app)
  
  Note over LLM,V: 3. Host renders iframe
  Host->>V: Create iframe with HTML
  V->>V: Load HTML, run JS
  
  Note over LLM,V: 4. View initializes (postMessage)
  V->>Host: {jsonrpc, method: "ui/initialize", params: {appCapabilities}}
  Host-->>V: {jsonrpc, result: {hostContext}}
  V->>Host: {jsonrpc, method: "ui/notifications/initialized"}
  
  Note over LLM,V: 5. Host sends tool data
  Host->>V: {method: "ui/notifications/tool-input", params: {arguments: {filter: "Carnivore"}}}
  Host->>V: {method: "ui/notifications/tool-result", params: {structuredContent: {dinosaurs: [...], filter: "Carnivore"}}}
  
  Note over LLM,V: 6. View renders dashboard
  V->>V: renderDinos(data.dinosaurs)
  V->>V: applyFilter("Carnivore")
  
  Note over LLM,V: 7. User interacts with View
  V->>Host: {method: "tools/call", id: 1, params: {name: "dino_think", arguments: {}}}
  Host->>S: tools/call dino_think
  S-->>Host: DinoThinkResult
  Host-->>V: {jsonrpc, id: 1, result: {content: [...], structuredContent: {...}}}
```

---

## Deployment Topology

```mermaid
flowchart TB
  subgraph Local["Local Development"]
    direction TB
    L1["dino-mcp binary\n:9010"]
    L2["Browser\n/dashboard"]
    L3["MCP Inspector\n:5173"]
    L4["curl / CLI tools"]
  end
  
  subgraph ClaudeDesktop["Claude Desktop Integration"]
    CD["Claude Desktop"]
    CD_Config["claude_desktop_config.json\ncommand: ./bin/dino-mcp stdio"]
  end
  
  subgraph Remote["Remote Access (Cloudflare Tunnel)"]
    T["cloudflared tunnel"]
    T_URL["trycloudflare.com URL"]
    TB["Browser (anywhere)"]
  end
  
  L4 -->|"POST /mcp"| L1
  L2 -->|"GET /dashboard\nGET /api/dinosaurs"| L1
  L3 -->|"POST /mcp"| L1
  CD -->|"stdio (JSON-RPC)"| L1
  CD_Config --> CD
  
  T -->|"HTTPS → HTTP :9010"| L1
  TB -->|"HTTPS"| T
  TB -->|"HTTPS"| T_URL
  
  style L1 fill:#4a4,color:#fff
  style CD fill:#44a,color:#fff
```

---

## Security Boundaries

```mermaid
flowchart LR
  subgraph Browser["Browser"]
    direction TB
    Page["MCP Inspector Page\norigin: localhost:5173"]
    IFrame["Dashboard iframe\norigin: localhost:9010"]
  end
  
  subgraph Server["dino-mcp Server"]
    Gin["Gin Router\nCORS: * origin\nMethods: GET, POST, OPTIONS\nHeaders: Content-Type, Accept, MCP-Session-Id"]
    MCP["StreamableHTTPHandler\nDNS Protection (optional)\nSession IDs\nUser ID validation"]
  end
  
  subgraph Transport["Transport Security"]
    STDIO["stdio\nProcess isolation\nNo network exposure"]
    HTTP["Streamable HTTP\nSession hijacking protection\nOrigin validation"]
  end
  
  Page -->|"OPTIONS preflight"| Gin
  Page -->|"POST /mcp with Accept"| Gin
  Gin -->|"CORS headers"| Page
  Gin -->|"Route"| MCP
  
  IFrame -.-|"postMessage only"| Page
  IFrame -->|"ui/initialize"| Page
  Page -->|"forward to server"| Gin
```

---

## Component Responsibilities

| Component | File | Responsibility |
|-----------|------|----------------|
| CLI | `cmd/dino-mcp/main.go` | Parse subcommand, parse flags, start transport |
| Server | `internal/server/server.go` | Create `mcp.Server`, register tools, Gin routing, CORS |
| Dashboard Tool | `internal/tools/dashboard.go` | `dino_dashboard` handler, resource registration, dinosaur data |
| Think/Ask Tools | `internal/tools/think.go + ask.go` | `dino_think` random fact, `dino_ask` Q&A handlers |
| View (TS) | `ui/src/mcp-app.ts` | `App` class, postMessage protocol, card rendering |
| View (Bundle) | `internal/resources/dashboard_ui.html` | Vite-built single HTML file, served via `//go:embed` |
| Build System | `Makefile` | Build, run, test, lint, clean targets |
| Integration Test | `test_mcp.sh` | 7 protocol-level tests against live server |

---

## Data Flow Summary

```
                    ┌──────────────────────┐
                    │   dino_think          │
                    │   → random fact       │
                    │   → DinoThinkResult   │
                    └──────────┬───────────┘
                               │
┌─────────────┐     ┌──────────▼───────────┐     ┌──────────────┐
│ MCP Client  │────►│                      │────►│ dino_ask     │
│             │     │   mcp.Server         │     │ → Q&A        │
│ Claude Desk │◄────│   (Go SDK)           │◄────│ → DinoAskRslt│
│ Inspector   │     │                      │     └──────────────┘
└─────────────┘     └──────────┬───────────┘
                               │
                    ┌──────────▼───────────┐     ┌──────────────┐
                    │   dino_dashboard     │────►│ HTML Resource│
                    │   → filter + dinos   │     │ (iframe UI)  │
                    │   → structuredContent│     └──────────────┘
                    └──────────────────────┘
```

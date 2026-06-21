# Technical Design Document — dino-mcp

> **Deep technical design for implementation teams.** Covers every subsystem, interface contract, data structure, and edge case.

---

## 1. CLI Layer

### Interface

```
dino-mcp <subcommand> [flags]
```

| Subcommand | Args | Behavior |
|------------|------|----------|
| `stdio` | `[-verbose] [-version]` | Creates server, calls `s.Run(StdioTransport{})` |
| `http` | `[-addr :9010] [-verbose] [-version]` | Creates Gin server, calls `httpServer.ListenAndServe()` |

### Flag Parsing

```go
fs := flag.NewFlagSet("dino-mcp", flag.ExitOnError)
addr := fs.String("addr", ":9010", "HTTP listen address (for http mode)")
verbose := fs.Bool("verbose", false, "Enable verbose debug logging")
version := fs.Bool("version", false, "Print version and exit")
```

### Error Handling

- Unknown subcommand → print usage to stderr, exit 1
- Server runtime error → log error, exit 1
- Signal handling → `signal.NotifyContext(ctx, SIGINT, SIGTERM)` → graceful shutdown

---

## 2. Server Layer

### Configuration

```go
type ServerConfig struct {
    Logger  *slog.Logger
    Version string  // injected via -ldflags
}
```

### Server Creation (`server.New()`)

```go
func New(logger *slog.Logger) *mcp.Server
```

**Registration order** (must be maintained):
1. Create `mcp.Server` with implementation info + instructions
2. `tools.RegisterThink(s)` — text-only, no schema params
3. `tools.RegisterAsk(s)` — text-only, requires `question` param
4. `tools.RegisterDashboardTool(s) + resources.RegisterDashboardResource(s, logger)` — MCP App with UI resource

### Transport Functions

#### `RunStdio`

```go
func RunStdio(ctx context.Context, s *mcp.Server, logger *slog.Logger) error {
    logger.Info("starting dino-mcp on stdio transport")
    return s.Run(ctx, &mcp.StdioTransport{})
}
```

- Blocking call; runs until context cancelled
- JSON-RPC over stdin/stdout
- No HTTP, no CORS, no port binding

#### `RunStreamableHTTP`

```go
func RunStreamableHTTP(ctx context.Context, s *mcp.Server, addr string, logger *slog.Logger) error
```

**Sequence:**
1. Set Gin mode (release or debug)
2. Create `gin.New()` engine
3. Add middleware: Recovery → Logger → CORS
4. Create `mcp.NewStreamableHTTPHandler()` with `DisableLocalhostProtection: true`
5. Mount at `/mcp` and `/mcp/*any` via `gin.WrapH()`
6. Add `/dashboard`, `/dashboard/*any`, `/api/dinosaurs`, `/health`
7. Create `http.Server` with Gin as handler
8. Start graceful shutdown goroutine (ctx.Done → Shutdown)
9. `ListenAndServe()`

---

## 3. MCP SDK Interface

### Tool Registration (`mcp.AddTool`)

```go
func AddTool[Args, Result any](
    s *mcp.Server,
    tool *mcp.Tool,
    handler func(context.Context, *mcp.CallToolRequest, Args) (*mcp.CallToolResult, Result, error),
)
```

**Contract:**
- `Args` type must match JSON schema in `tool.InputSchema`
- Handler returns `(*mcp.CallToolResult, Result, error)`
- If error is non-nil, tool call fails with error message
- If `*mcp.CallToolResult` is nil, it's constructed from `Result`
- `Result` becomes `structuredContent` in response

### Resource Registration (`s.AddResource`)

```go
func (s *Server) AddResource(
    resource *mcp.Resource,
    handler func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error),
)
```

**Contract:**
- Resource URI must use `ui://` scheme for MCP Apps
- MIME type must be `text/html;profile=mcp-app`
- Handler reads embedded file from `//go:embed` filesystem
- Returns `ResourceContents` with URI, MIMEType, and Text

---

## 4. HTTP Layer (Gin)

### Route Table (Complete)

| Method | Path | Handler Function | Purpose |
|--------|------|-----------------|---------|
| ANY | `/mcp` | `gin.WrapH(streamableHandler)` | MCP JSON-RPC |
| ANY | `/mcp/*any` | `gin.WrapH(streamableHandler)` | MCP subpaths |
| GET | `/dashboard` | `serveDashboard(c)` | Standalone HTML |
| GET | `/dashboard/*any` | `serveDashboard(c)` | Standalone subpaths |
| GET | `/api/dinosaurs` | `dinoAPIHandler(c)` | JSON dinosaur data |
| GET | `/api/dinosaurs?filter=X` | `dinoAPIHandler(c)` | Filtered JSON |
| GET | `/health` | `healthHandler(c)` | Health check |

### Middleware Chain

```
Request → gin.Recovery() → ginLogger() → corsMiddleware() → Route Handler
```

**CORS Middleware:**
```go
func corsMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Header("Access-Control-Allow-Origin", "*")
        c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
        c.Header("Access-Control-Allow-Headers", "Content-Type, Accept, Origin, MCP-Session-Id")
        c.Header("Access-Control-Expose-Headers", "MCP-Session-Id, Content-Type")
        if c.Request.Method == http.MethodOptions {
            c.AbortWithStatus(http.StatusOK)
            return
        }
        c.Next()
    }
}
```

---

## 5. Data Model

### DinoSummary (Core Entity)

```go
type DinoSummary struct {
    Name       string `json:"name"`        // Dinosaur name (e.g., "Tyrannosaurus Rex")
    Period     string `json:"period"`       // Geological period (e.g., "Cretaceous")
    Diet       string `json:"diet"`         // Diet type (e.g., "Carnivore")
    Length     string `json:"length"`       // Length description (e.g., "40 ft (12 m)")
    Weight     string `json:"weight"`       // Weight description (e.g., "9 tons (8,000 kg)")
    FunFact    string `json:"funFact"`      // Fun fact for card display
    ImageStyle string `json:"imageStyle"`   // CSS hint for avatar styling (e.g., "bg-red-900")
}
```

### Tool Result Types

```go
type DinoThinkResult struct {
    Fact    string `json:"fact"`     // Random dinosaur fact
    Species string `json:"species"`  // Related species name
}

type DinoAskResult struct {
    Question string `json:"question"`         // Original question
    Answer   string `json:"answer"`           // Curated answer text
    Species  string `json:"species,omitempty"` // Referenced species (if any)
}

type DinoDashboardResult struct {
    Filter    string        `json:"filter"`     // Applied filter
    Dinosaurs []DinoSummary `json:"dinosaurs"`  // Filtered list
    Timestamp string        `json:"timestamp"`  // ISO 8601 UTC
}
```

### Entity Counts

| Entity | Count |
|--------|-------|
| Dinosaur species | 12 |
| Dinosaur facts (random pool) | 11 |
| Filter modes | 6 (All, Carnivore, Herbivore, Triassic, Jurassic, Cretaceous) |
| Named periods | 3 (Triassic, Jurassic, Cretaceous) |
| Diet types | 2 (Carnivore, Herbivore) |

---

## 6. UI Layer (ext-apps SDK)

### App Class Interface

```ts
interface AppOptions {
    name: string;
    version: string;
}

class App {
    constructor(options: AppOptions);
    
    // Lifecycle
    connect(transport?: PostMessageTransport): Promise<void>;
    disconnect(): Promise<void>;
    
    // Handlers
    ontoolinput: ((params: any) => void) | null;
    ontoolinputpartial: ((params: any) => void) | null;
    ontoolresult: ((result: any) => void) | null;
    onhostcontextchanged: ((ctx: HostContext) => void) | null;
    onteardown: (() => Promise<Record<string, unknown>>) | null;
    
    // Methods
    callServerTool(request: CallToolRequest): Promise<CallToolResult>;
}
```

### PostMessage Transport Protocol

```ts
class PostMessageTransport implements Transport {
    // Sends JSON-RPC messages via window.parent.postMessage()
    // Receives messages via window.addEventListener('message', ...)
}
```

### Message Types

**View → Host:**
| Method | When | Purpose |
|--------|------|---------|
| `ui/initialize` | On load | App capabilities handshake |
| `ui/notifications/initialized` | After initialize response | Confirms ready |
| `tools/call` | User interaction | View calls server tool |
| `ui/request-display-mode` | User clicks fullscreen | Display mode change |

**Host → View:**
| Method | When | Purpose |
|--------|------|---------|
| `ui/initialize result` | Response to init | Host context + capabilities |
| `ui/notifications/tool-input` | Tool called (stream) | Partial/complete arguments |
| `ui/notifications/tool-result` | Tool returned | Structured data for rendering |
| `ui/notifications/host-context-changed` | Theme/layout change | Adapt styling |
| `ping` | Periodic | Connection health check |

---

## 7. API Endpoints

### `GET /api/dinosaurs`

**Query Parameters:**
| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `filter` | string | `""` | Filter by diet, period, or name |

**Response (200):**
```json
{
  "dinosaurs": [DinoSummary],
  "filter": "Carnivore",
  "timestamp": "2026-06-21T10:00:00Z",
  "total": 4
}
```

### `GET /health`

**Response (200):**
```json
{
  "status": "ok",
  "server": "dino-mcp",
  "version": "abc1234-dirty"
}
```

---

## 8. Build System

### Makefile Targets

| Target | Dependencies | Description |
|--------|-------------|-------------|
| `all` | `build` | Default full build |
| `build` | `build-ui build-go` | UI + Go binary |
| `build-ui` | npm install | Vite bundle → dist |
| `build-go` | Go source | `go build` with ldflags |
| `build-fast` | `build-go` | Go only (existing UI) |
| `run-stdio` | `build-fast` | Launch stdio mode |
| `run-http` | `build-fast` | Launch HTTP mode |
| `dev-http` | `build-fast` | HTTP + verbose flag |
| `run-tunnel` | `build-fast` | HTTP + cloudflared tunnel |
| `test` | `build-fast` | Run test_mcp.sh |
| `test-inspector` | `build-fast` | Launch MCP Inspector |
| `lint` | — | `go vet ./... && go fmt` |
| `clean` | — | Remove artifacts |

### Version Injection

```makefile
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS = -ldflags="-X github.com/msw/dino-mcp/internal/server.Version=$(VERSION)"
```

### UI Build Pipeline

```
ui/src/mcp-app.ts → Vite + singlefile → ui/dist/mcp-app.html
                                          ↓ (Makefile copies)
                                    internal/resources/dashboard_ui.html
                                          ↓ (Go compiles)
                                    //go:embed → runtime
```

---

## 9. Integration Test Contract

### Test Server Configuration

- Binary: `./bin/dino-mcp`
- Port: `:9010`
- Transport: Streamable HTTP
- Endpoint: `http://localhost:9010/mcp`
- Startup wait: 2 seconds

### Test Sequence

| Step | Method | Request | Validates |
|------|--------|---------|-----------|
| 1 | Initialize | `initialize` + protocol version | Session ID returned |
| 2 | Notify | `notifications/initialized` | 200 OK |
| 3 | List tools | `tools/list` | dino_dashboard has `_meta.ui.resourceUri` |
| 4 | Call think | `tools/call dino_think` | Fact + species in structuredContent |
| 5 | Call dashboard | `tools/call dino_dashboard {filter:"Carnivore"}` | 4 Carnivore dinos returned |
| 6 | Read resource | `resources/read ui://dino-dashboard/mcp-app.html` | MIME type `text/html;profile=mcp-app` |
| 7 | Call ask | `tools/call dino_ask {question:"What did T-Rex eat?"}` | Question + Answer in result |

### Session ID Extraction

```bash
# Header format: Mcp-Session-Id: <UUID>
extract_sid() {
  grep -i '^Mcp-Session-Id: ' | head -1 | sed 's/.*: //' | tr -d '\r'
}
```

---

## 10. Error Handling

### Go Server Errors

| Error Type | Source | User Impact |
|------------|--------|-------------|
| Port in use | `httpServer.ListenAndServe()` | Server won't start |
| Context cancelled | `ctx.Done()` | Graceful shutdown |
| Invalid Accept header | `streamableAccepts()` | HTTP 400 |
| DNS rebinding | `DisableLocalhostProtection` | HTTP 403 |
| Session hijack | User ID mismatch | HTTP 403 |
| Asset not found | `//go:embed` read error | 500 on resource/read |

### UI Errors

| Error | Handling |
|-------|----------|
| postMessage timeout (5s) | Fallback to standalone fetch |
| API fetch failure | Show error state with retry |
| Empty dinosaur data | "No Dinosaurs Found" message |
| Host disconnect | Tear down gracefully |

---

## 11. Performance Characteristics

| Metric | Value | Notes |
|--------|-------|-------|
| Binary size | ~11MB | Static Go binary |
| Memory idle | ~15MB | Server startup |
| Memory with UI | ~25MB | After first dashboard load |
| Startup time | <1s | Go binary |
| UI build time | ~700ms | Vite production build |
| Docker image | ~15MB | Scratch-based possible |

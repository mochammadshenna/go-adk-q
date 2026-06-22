# Architecture Overview: Archipelago Hotels MCP Server

> **Type**: Explanation (Diátaxis — understanding-oriented)
> **Audience**: Engineers onboarding to the codebase or extending it
> **Last reviewed**: 2026-06-22

---

## Table of Contents

1. [Why a single Go binary](#1-why-a-single-go-binary)
2. [The two-database topology](#2-the-two-database-topology)
3. [Lazy brand DB connections](#3-lazy-brand-db-connections)
4. [The MCP protocol flow](#4-the-mcp-protocol-flow)
5. [The resource/tool meta relationship](#5-the-resourcetool-meta-relationship)
6. [Transport abstraction](#6-transport-abstraction)
7. [Full sequence diagram: Claude prompt to UI update](#7-full-sequence-diagram-claude-prompt-to-ui-update)
8. [Why Gin for HTTP transport](#8-why-gin-for-http-transport)

---

## 1. Why a single Go binary

The server compiles to one self-contained binary: `archipelago-hotels-mcp`.

**Portability for Claude Desktop integration.** Claude Desktop launches the MCP server as a child process on the user's machine. The only configuration it accepts is a path to a binary and optional CLI arguments. If the server were a Node.js application, the operator would need to install Node, npm, and dependencies on every machine where Claude Desktop runs. A Go binary has zero runtime dependencies — it runs on macOS, Linux, or Windows by copying the file.

**No external assets at runtime.** The dashboard UI (a ~1200-line TypeScript/Vite bundle) is compiled to a single HTML file and embedded with `//go:embed mcp-app.html`. The binary serves it without touching the filesystem:

```go
//go:embed mcp-app.html
var dashboardHTML string
```

**Single deployment unit for both transports.** The same binary supports `stdio` (Claude Desktop) and `http` (web clients, CI). Mode is chosen by a positional CLI argument: `archipelago-hotels-mcp stdio` or `archipelago-hotels-mcp http`. There is nothing to orchestrate, no separate web server to keep in sync with the MCP server.

**Deterministic builds.** Go produces a statically linked binary (when CGO is not involved, as is the case here). The `Makefile` stamps `Version` into the binary at link time with `-ldflags "-X server.Version=$(VERSION)"`. The resulting artifact is reproducible and auditable.

---

## 2. The two-database topology

The server connects to two categories of MySQL database.

### Central catalog database

`db_archipelagowebsite` is the single source of truth for the hotel catalog. It holds:

- `tb_hotels` — every Archipelago property: name, address, coordinates, rating, stars, `starting_price`, currency, `api_hotel_id` (the key into the brand DB)
- `tb_brands` — brand names, `db_prefix_name`, parent-brand relationships
- `tb_region` — region metadata used for filtering

This database is always connected on startup. `NewPool` opens it, pings it, and fails immediately if it is unreachable. It supports 10 max open connections and 3 idle connections.

### Per-brand databases

Each hotel brand has its own operational database. The mapping is:

| Brand | Database |
|-------|----------|
| Aston | `db_astonwebsite` |
| The Alana | `db_alanawebsite` |
| NEO | `db_neowebsite` |
| Harper | `db_harperwebsite` |
| favehotels | `db_favewebsite` |
| PBA | `db_pba` |
| (others) | `db_{prefix}website` |

Brand databases hold `tb_hotels` with brand-specific fields (including `thumbnail_desktop`), room types (`tb_hrooms` or `tb_hroom`), stored room rates, and booking engine credentials (SimpleBooking ID, username, password).

**Why two databases instead of one?** Archipelago grew by acquiring independent hotel brands. Each brand's operational database predates the central catalog and is owned by the brand's engineering team. Rather than a high-risk migration of all brands into one schema, the system bridges them: `tb_hotels.api_hotel_id` in the central DB is the `hotel_id` in the brand DB. The `db_prefix_name` column on `tb_brands` tells the `Pool` which brand database to connect.

---

## 3. Lazy brand DB connections

`Pool.BrandDB(ctx, prefix)` connects to a brand database the first time it is asked for and caches the result — including failures.

### The connect path

```
BrandDB("aston")
  ├─ RLock → check brandDBs["aston"] → not found → RUnlock
  ├─ connectBrand("aston")          ← outside the lock
  │     sql.Open + Ping (3s timeout)
  │     → returns *sql.DB or nil
  ├─ Lock
  │   ├─ double-check: another goroutine may have stored it already
  │   ├─ store brandDBs["aston"] = db   (nil if connect failed)
  │   └─ if db != nil: scanColumns("aston", db)
  └─ Unlock → return db
```

The connect happens outside the write lock deliberately. Multiple goroutines may concurrently try to open _different_ brand databases; holding the write lock during a network operation would serialize them unnecessarily. The double-check inside the write lock handles the rare case where two goroutines race for the _same_ prefix.

### Why nil is stored on failure

When `connectBrand` fails (brand DB unreachable, wrong credentials, DNS failure), `nil` is stored in `brandDBs[prefix]`. Subsequent calls find the key present and return `nil` immediately without retrying. This prevents repeated 3-second timeout stalls on a persistently unreachable brand. Recovery requires a server restart.

Brand databases are configured with conservative limits: `MaxOpenConns=3`, `MaxIdleConns=1`. A single brand's DB should never hold more than a handful of connections.

### Column introspection via scanColumns

Brand schemas are not uniform. PBA uses `tb_hroom` (singular); all other brands use `tb_hrooms`. Some brands lack `thumbnail_desktop`. On first connect, `scanColumns` queries `INFORMATION_SCHEMA.COLUMNS` and caches the result:

```go
SELECT TABLE_NAME, COLUMN_NAME
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_SCHEMA = (SELECT DATABASE())
  AND TABLE_NAME IN ('tb_hotels','tb_hrooms','tb_hroom')
```

All subsequent queries guard optional columns with `pool.HasColumn(prefix, table, column)` before including them in SQL. This eliminates the need for a static per-brand schema config file.

---

## 4. The MCP protocol flow

The Model Context Protocol uses JSON-RPC 2.0. Claude Desktop sends requests; the server responds. The go-sdk (`github.com/modelcontextprotocol/go-sdk`) handles framing, routing, and serialisation. Here is what happens when Claude calls `search_hotels`:

1. **Claude Desktop sends a tool-call request** over the stdio transport as a newline-delimited JSON object:
   ```json
   {"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"search_hotels","arguments":{"city":"Bali","limit":5}}}
   ```

2. **The go-sdk deserialises the request** and routes it to the registered handler for `search_hotels`. The SDK uses the `TArgs` type parameter to unmarshal `arguments` into the tool's input struct:
   ```go
   mcp.AddTool(s, tool, func(ctx context.Context, req *mcp.CallToolRequest, args SearchArgs) (*mcp.CallToolResult, *searchResult, error) { ... })
   ```

3. **The handler executes**: queries `Pool.SearchHotels`, calls `rate.Service.BatchMinRates`, assembles a `searchResult` struct.

4. **The go-sdk serialises the response** in two forms:
   - `result.content[0].text` — a human-readable Markdown summary (Claude reads this)
   - `result.structuredContent` — the full `searchResult` struct as JSON (the UI TypeScript reads this)

5. **The response is sent back** over the same transport:
   ```json
   {"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"Found 5 hotels..."}],"structuredContent":{...},"_meta":{"ui":{"resourceUri":"ui://hotel-dashboard","resourceDomains":["images.archipelagohotels.com"]}}}}
   ```

6. **Claude Desktop sees `_meta.ui.resourceUri`** and fetches `ui://hotel-dashboard` from the server as a resource, then renders it in an iframe panel. The ext-apps SDK inside the iframe calls `get_hotel_detail` via the same session when the user clicks a hotel card.

All application logs are written to `stderr`. This is critical for the stdio transport: `stdout` carries the JSON-RPC stream, and any stray byte on `stdout` (a `fmt.Println`, for example) would corrupt the protocol.

---

## 5. The resource/tool meta relationship

The interactive UI requires two registrations: a **tool** that triggers the UI load, and a **resource** that delivers the HTML. They are linked by a shared URI.

### Tool registration

```go
mcp.AddTool(s, &mcp.Tool{
    Name:        "find_hotels",
    Description: "Opens the hotel dashboard...",
    Meta: mcp.Meta{
        "ui": map[string]any{
            "resourceUri":     "ui://hotel-dashboard",
            "resourceDomains": []string{"images.archipelagohotels.com"},
        },
    },
}, handler)
```

The `Meta` field is passed through to `result._meta` in every tool response. When Claude Desktop receives a result with `_meta.ui.resourceUri`, it instructs the ext-apps SDK to load that resource.

### Resource registration

```go
s.AddResource(&mcp.Resource{
    URI:      "ui://hotel-dashboard",
    Name:     "Hotel Dashboard",
    MIMEType: "text/html;profile=mcp-app",
    Meta: mcp.Meta{
        "ui": map[string]any{
            "resourceDomains": []string{"images.archipelagohotels.com"},
        },
    },
}, handler)
```

The `profile=mcp-app` suffix on `MIMEType` is the signal to the MCP host that this is an interactive application, not a plain document. The host renders it in a sandboxed iframe rather than displaying it as text.

### resourceDomains and CSP

The iframe has a strict Content Security Policy. `resourceDomains` must be declared in **both** the tool and the resource registration. The MCP host merges them to build the `img-src` CSP directive for the iframe. Without `images.archipelagohotels.com` in the allowlist, every hotel thumbnail would be blocked by the browser's CSP enforcement.

### The ui:// scheme

`ui://hotel-dashboard` is not an HTTP URL. It is an opaque identifier resolved entirely by the MCP host — the server registers the resource under this URI, and the host fetches it via the MCP `resources/read` method over the same JSON-RPC session, not via HTTP.

---

## 6. Transport abstraction

`server.New()` returns a `*Service` containing a `*mcp.Server`. The `mcp.Server` is transport-agnostic: it processes requests and produces responses without knowing whether the underlying channel is a pipe or a TCP socket.

```go
// stdio
s.MCP.Run(ctx, &mcp.StdioTransport{})

// HTTP (Gin wraps the go-sdk handler)
mcpHandler := mcp.NewStreamableHTTPHandler(
    func(r *http.Request) *mcp.Server { return svc.MCP },
    &mcp.StreamableHTTPOptions{},
)
r.POST("/mcp", func(c *gin.Context) { mcpHandler.ServeHTTP(c.Writer, c.Request) })
r.GET("/mcp",  func(c *gin.Context) { mcpHandler.ServeHTTP(c.Writer, c.Request) })
```

`StdioTransport` wraps `os.Stdin`/`os.Stdout` as a JSON-RPC channel. `StreamableHTTPHandler` implements the MCP Streamable HTTP transport spec: `POST /mcp` for client-initiated requests, `GET /mcp` for server-sent events. Session affinity is maintained by the `Mcp-Session-Id` header.

The tool handlers and resources registered with `mcp.Server` are identical in both modes. Tests can inject a stdio-backed server; production deploys can switch to HTTP by changing a single CLI argument.

---

## 7. Full sequence diagram: Claude prompt to UI update

```mermaid
sequenceDiagram
    autonumber
    participant U as User
    participant CD as Claude Desktop
    participant STDIO as StdioTransport
    participant MCP as mcp.Server (go-sdk)
    participant H as find_hotels handler
    participant POOL as repository.Pool
    participant RS as rate.Service
    participant EXTAPPS as ext-apps SDK (iframe)

    U->>CD: "Show me hotels in Bali"
    CD->>CD: LLM decides to call find_hotels
    CD->>STDIO: JSON-RPC tools/call\n{"name":"find_hotels","arguments":{"city":"Bali"}}
    STDIO->>MCP: dispatch request
    MCP->>H: handler(ctx, req, DashboardArgs{City:"Bali"})
    H->>POOL: SearchHotels(ctx, {City:"Bali", Limit:20})
    POOL-->>H: []HotelRow (20 results)
    H->>RS: BatchMinRates(ctx, hotels)
    RS-->>H: map[hotelID]float64
    H-->>MCP: (*CallToolResult, *dashboardResult, nil)
    MCP->>STDIO: JSON-RPC result\ncontent[text] + structuredContent + _meta.ui.resourceUri
    STDIO->>CD: response bytes
    CD->>CD: sees _meta.ui.resourceUri = "ui://hotel-dashboard"
    CD->>STDIO: JSON-RPC resources/read\n{"uri":"ui://hotel-dashboard"}
    STDIO->>MCP: dispatch resources/read
    MCP-->>STDIO: resource{mimeType:"text/html;profile=mcp-app", text: <HTML>}
    STDIO-->>CD: resource response
    CD->>EXTAPPS: render HTML in sandboxed iframe\n(CSP img-src += images.archipelagohotels.com)
    EXTAPPS->>EXTAPPS: parse structuredContent from tool result\nrender hotel cards
    U->>EXTAPPS: clicks a hotel card
    EXTAPPS->>STDIO: JSON-RPC tools/call\n{"name":"get_hotel_detail","arguments":{"hotel_id":42}}
    STDIO->>MCP: dispatch
    MCP->>POOL: GetHotelDetail + GetRooms
    POOL-->>MCP: full hotel + rooms
    MCP-->>EXTAPPS: structuredContent with room list
    EXTAPPS->>EXTAPPS: render detail panel
```

---

## 8. Why Gin for HTTP transport

The HTTP mode requires more than just an MCP endpoint. It also serves `/dashboard`, `/api/hotels`, `/api/brands`, `/api/regions`, and `/health`. These routes need CORS middleware, input validation, error recovery from panics, and conditional request logging.

Gin was chosen because:

- **Minimal footprint.** Gin adds a router and middleware chain on top of `net/http`. It does not bring an ORM, template engine, or session framework. The full dependency is `github.com/gin-gonic/gin` and its handful of transitives.
- **CORS middleware in ~10 lines.** The MCP client (browser-hosted agent) and the standalone dashboard originate from different hosts. A middleware function sets the required headers on every response and handles `OPTIONS` preflight in three lines.
- **`gin.Recovery()` prevents server crashes.** If a handler panics (nil pointer on degraded-mode pool access, for example), Recovery logs the stack trace to stderr and returns HTTP 500. Without this, a single panic would terminate the entire server process.
- **`gin.ReleaseMode` suppresses debug noise.** In production the router prints nothing. In verbose mode (`-verbose` flag), `gin.LoggerWithWriter(os.Stderr)` logs each request without polluting stdout.
- **Idiomatic Go.** Gin handlers take `*gin.Context`, which wraps `http.Request` and `http.ResponseWriter`. The MCP handler (`mcp.NewStreamableHTTPHandler`) implements `http.Handler` and is bridged with one line: `mcpHandler.ServeHTTP(c.Writer, c.Request)`. No adapter layer required.

The alternative was `net/http` with manual middleware chaining. Given that CORS handling, panic recovery, and logging are already solved problems in Gin, and the server is not performance-sensitive at the middleware level, the small dependency cost is justified.

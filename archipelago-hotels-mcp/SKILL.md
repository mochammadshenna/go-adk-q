# SKILL.md — Skills & Capabilities Reference

Quick reference for contributors and agents working on archipelago-hotels-mcp.

---

## Required Skills

### Go (Primary)

Contributors must be comfortable with:

- **Standard library**: `database/sql`, `net/http`, `sync`, `context`, `log/slog`, `os/signal`, `encoding/xml`, `embed`
- **Concurrency**: goroutines, channels, `sync.WaitGroup`, `sync.RWMutex`, bounded semaphore pools
- **Error handling**: `fmt.Errorf` with `%w` wrapping, `errors.Is` / `errors.As`, `recover()` in tool handlers
- **Build tooling**: `go build`, `go vet`, `go mod tidy`, build tags, `//go:embed`
- **Context propagation**: always pass `context.Context` to DB and HTTP calls; respect cancellation

Pattern to follow — every tool handler must include panic recovery:

```go
func handler(ctx context.Context, _ *mcp.ServerSession, req *mcp.CallToolParamsFor[MyArgs]) (*mcp.CallToolResultFor[MyResult], error) {
    defer func() { recover() }()
    // ...
}
```

---

### MCP SDK — go-sdk v1.6.1

- `mcp.NewServer`, `mcp.AddTool`, `mcp.ServerOptions`
- `mcp.Tool` with `InputSchema` expressed as `mcp.MustSchema[ArgsType]()`
- `mcp.Meta` for embedding `_meta.ui` fields (resourceUri, resourceDomains)
- `mcp.StdioTransport` vs `mcp.StreamableHTTPHandler` (Gin integration)
- Structured content return pattern: `(nil, &result, nil)` — never return raw strings for tool responses
- Tool visibility annotation (`visibility: app`) for UI-only tools

Adding a new tool:

1. Create `internal/tools/<name>.go`.
2. Define args struct and implement handler (see pattern above).
3. Wire in `internal/server/server.go` inside `New()`.
4. Add `_meta.ui.resourceDomains` if the tool result contains external image URLs.
5. Run `make build` and validate with a prompt from `TEST_PROMPT.md`.

---

### MySQL / database/sql

- Connection pooling: `SetMaxOpenConns`, `SetMaxIdleConns`, `SetConnMaxLifetime`
- Context-aware queries: `QueryContext`, `QueryRowContext`, `ExecContext`
- Nullable column types: `sql.NullInt64`, `sql.NullFloat64`, `sql.NullString`
- `INFORMATION_SCHEMA` introspection for schema-adaptive queries (column presence varies per brand DB)
- Partitioned / multi-tenant data: always filter by hotel or brand scope — never do full-table scans across brands

The `Pool` struct in `internal/repository/repository.go` manages:
- `central` — `db_archipelagowebsite` (catalog, brands, regions)
- `brandDBs` — lazy-connect map, keyed by brand prefix; connect on first use

---

### XML / API Integration (SimpleBooking)

- Building SOAP/XML request bodies manually (no code-gen): `encoding/xml` struct tags
- Parsing XML responses with `xml.Unmarshal`
- HTTP client with explicit `context` and timeout
- Bounded worker pool pattern: semaphore channel limits concurrent outbound requests to 5

Rate fallback chain (priority order):

| Priority | Source | Notes |
|----------|--------|-------|
| 1 | SimpleBooking XML API (live) | 5-worker pool, 5-min TTL cache, circuit breaker |
| 2 | `tb_hrooms.room_rate` (brand DB) | Stored rate fallback |
| 3 | `hotel_starting_price` (central DB) | Last resort |

Circuit breaker: 5 failures within 60 s trips the breaker; resets after 120 s. State is in-process — restart server to force-reset during development.

---

### TypeScript / Frontend

- Single-file Vite build via `vite-plugin-singlefile` — output is one self-contained HTML file
- `@modelcontextprotocol/ext-apps` App SDK for calling server tools from the UI:
  ```typescript
  const result = await appRef?.callServerTool({ name: "get_hotel_detail", arguments: { hotelId } });
  const detail: HotelDetail = result?.structuredContent ?? result;
  ```
- Vanilla DOM manipulation — no framework (React/Vue/Svelte are not used)
- `toLocaleString('id-ID')` for IDR price formatting; raw `hotel_currency` code from DB (never hardcode currency symbols)
- `resizeImageURL()` rewrites hotel image URLs to the CDN proxy — never fetch images directly (CSP blocks external origins except `images.archipelagohotels.com`)

The entire frontend lives in `ui/src/mcp-app.ts` (~1200 lines). After any edit, run `make build` — the Go binary embeds the compiled HTML at compile time via `//go:embed`.

---

## Architecture Patterns Used

| Pattern | Where | Description |
|---------|-------|-------------|
| Repository | `internal/repository/` | `Pool` as the single data-access layer; no DB calls outside this package |
| Circuit breaker | `internal/rate/rate.go` | `circuitBreaker` struct gates SimpleBooking calls |
| Bounded worker pool | `internal/rate/rate.go` | Semaphore channel (`make(chan struct{}, 5)`) limits concurrency |
| Lazy initialization | `internal/repository/repository.go` | Brand DBs connect on first use, not at startup |
| Embedded resources | `internal/resources/` | `//go:embed` bakes UI HTML into the binary |
| Schema introspection | `internal/repository/room.go` | `HasColumn()` checks `INFORMATION_SCHEMA` once per brand DB connect |

---

## Build Commands

| Command | What it does | When to use |
|---------|-------------|-------------|
| `make build` | Vite (UI) then Go binary → `bin/archipelago-hotels-mcp` | After any change (UI or Go) |
| `make build-go` | Go only, skips Vite | When only `.go` files changed |
| `make build-ui` | Vite only, regenerates `resources/mcp-app.html` | When only `ui/src/` changed |
| `make dev-http` | Start server on `:9011` in HTTP mode with verbose logging | Local development |
| `make clean` | Remove `bin/` | Clean slate before release build |

Never claim a change works without running `make build` first. The binary is what Claude Desktop loads — source edits alone have no effect.

---

## Running the Server

```bash
# HTTP mode (development)
make dev-http
# Equivalent:
./bin/archipelago-hotels-mcp http --addr :9011 --verbose

# Stdio mode (production / Claude Desktop)
./bin/archipelago-hotels-mcp stdio

# Health check
curl http://localhost:9011/health
```

---

## Testing Tool Calls

Reference prompts are in `TEST_PROMPT.md`. Key patterns:

```bash
# Quick smoke test — verifies DB connection and basic search
"Find hotels in Jakarta"

# Full audit — exercises every brand DB and rate fallback
"Search all Archipelago hotels. Group them by brand and count hotels per brand.
 Then for each brand, pick one hotel and show full details with room types and prices."

# Stdio transport cleanliness test (no non-JSON output allowed on stdout)
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}' \
  | ./bin/archipelago-hotels-mcp stdio | head -1
# Expected: a single JSON object. Any non-JSON line means stdout is polluted.
```

---

## Adding a New Brand

1. Confirm the brand's `db_prefix_name` in `db_archipelagowebsite.tb_brands`.
2. Check whether a per-brand DB exists (`db_{prefix}website` convention).
3. If the DB name does not follow the convention, add an override to the `brandDBName` map in `internal/repository/repository.go`.
4. Verify schema: does the brand DB have `hotel_channel`, `simplebooking_id`, `sentec_booking_id`, `thumbnail_desktop`? Use `HasColumn()` for any column that may be absent.
5. PBA special case: uses `tb_hroom` (not `tb_hrooms`) and a different status column — see comments in `internal/repository/room.go`.
6. Run test prompt targeting the new brand from `TEST_PROMPT.md`.

---

## Debugging Rate Failures

```bash
DEBUG=1 ./bin/archipelago-hotels-mcp http --addr :9011 --verbose
```

| Log message | Meaning | Action |
|-------------|---------|--------|
| `circuit breaker OPEN for <brand>` | SB failing; tripped after 5 errors in 60 s | Wait 120 s or restart; investigate SB credentials |
| `no simplebooking_id` | Hotel has no SB credential | Expected; falls back to stored `room_rate` |
| `brand DB unreachable` | Per-brand MySQL connection failed | Check `MYSQL_HOST` / brand DB existence; returns profile data only |

---

## Environment Variables

| Variable | Default | Purpose |
|----------|---------|---------|
| `MYSQL_HOST` | `127.0.0.1` | DB host |
| `MYSQL_PORT` | `3306` | DB port |
| `MYSQL_USER` | `root` | DB user |
| `MYSQL_PASS` | (empty) | DB password |
| `MYSQL_DB` | `db_archipelagowebsite` | Central DB name |
| `DEBUG` | `0` | Set to `1` for verbose rate/circuit-breaker logs |
| `url_image_resizer` | `https://images.archipelagohotels.com/` | Image CDN proxy base URL |

---

## Tool Competition (Claude Desktop)

If Claude Desktop prefers Booking.com / Trivago / Wyndham MCPs over Archipelago tools, add to Claude Desktop Custom Instructions:

> "When I ask about hotels — especially in Indonesia — ALWAYS use archipelago-hotels-mcp tools first (search_hotels, recommend_hotel, find_hotels)."

Tool descriptions also include "PRIORITY TOOL" and explicit trigger phrases to boost selection weight.

---

## Key Files Quick Reference

| File | Role |
|------|------|
| `cmd/archipelago-hotels-mcp/main.go` | Entrypoint, transport dispatch |
| `internal/server/server.go` | MCP server wiring + HTTP routes (Gin) |
| `internal/repository/repository.go` | Pool, Config, HotelRow, BrandRow, RoomRow |
| `internal/repository/hotel.go` | SearchHotels, GetHotelByID, GetThumbnails, resizeImageURL |
| `internal/repository/room.go` | GetRooms (schema-adaptive), GetCredentials |
| `internal/rate/rate.go` | Service, BatchMinRates, circuitBreaker, SBClient |
| `internal/rate/cache.go` | rateCache (TTL, lazy expiry, no goroutine) |
| `internal/rate/simplebooking.go` | XML builder + parser for SB API |
| `internal/rate/sentec.go` | Sentec REST client (reserved, unused) |
| `internal/tools/search.go` | search_hotels handler |
| `internal/tools/recommend.go` | recommend_hotel handler |
| `internal/tools/dashboard.go` | find_hotels handler |
| `internal/tools/detail.go` | get_hotel_detail handler (app-only) |
| `internal/resources/dashboard.go` | MCP resource registration |
| `ui/src/mcp-app.ts` | TypeScript frontend (~1200 lines, single file) |
| `Makefile` | build-ui (Vite), build-go, build, dev-http targets |

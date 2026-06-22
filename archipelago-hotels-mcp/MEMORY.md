# MEMORY.md — Memory Index for AI Agent Sessions

Authoritative record of architecture decisions, known bugs, code patterns, and environment quirks for the archipelago-hotels-mcp project. Update this file when new decisions are made or new bugs are found.

---

## Project: archipelago-hotels-mcp

Go MCP server for Archipelago Hotels & Resorts (Sentec Tech product). Searches 279+ hotels across 13 brands.

### Stack

- Go 1.25, github.com/modelcontextprotocol/go-sdk v1.6.1
- Transport: stdio (Claude Desktop) | Streamable HTTP via Gin v1.11.0 on :9011
- DB: Central MySQL db_archipelagowebsite + 8 per-brand DBs (lazy-connect Pool)
- UI: Vite+TypeScript single-file app, //go:embed into mcp-app.html, served as MCP App resource

### MCP Tools

| Name | Visibility | Purpose |
|------|-----------|---------|
| search_hotels | public | Search by city/brand/query; returns hotel list + prices |
| recommend_hotel | public | Rank hotels by vibe/budget/purpose |
| find_hotels | public | Browse/book all hotels with optional city/brand filter |
| get_hotel_detail | app-only (visibility: app) | Full hotel detail + room types + bookingUrl; called by UI only |
| open_booking_url | app-only (visibility: app) | Opens booking URL in system browser via exec.Command — bypasses Electron iframe sandbox |

### Data Architecture

- Pool.central → db_archipelagowebsite (catalog, brands, regions)
- Pool.brandDBs → lazy-connect per brand prefix (aston, neo, fave, alana, harper, kamuela, quest, pba)
- Column introspection via INFORMATION_SCHEMA on connect (schema varies per brand)
- PBA special case: uses tb_hroom (not tb_hrooms), different status column

### Rate Fallback Chain (priority order)

1. SimpleBooking XML API (live) — OTA_HotelAvailRQ, 5-worker bounded pool, 5-min TTL cache, circuit breaker (5 failures → 120s cooldown)
2. tb_hrooms.room_rate (stored) — per-brand DB fallback
3. hotel_starting_price (central DB) — last resort

### HTTP Endpoints (HTTP mode)

- POST/GET /mcp — MCP Streamable HTTP
- GET /dashboard — Standalone HTML dashboard
- GET /api/hotels?city=&brand= — JSON hotel list
- GET /api/brands — JSON brand list
- GET /api/regions — JSON region list
- GET /health — { status, version, db }

---

## Environment Variables

| Var | Default | Purpose |
|-----|---------|---------|
| MYSQL_HOST | 127.0.0.1 | DB host (all brand DBs on same host) |
| MYSQL_PORT | 3306 | DB port |
| MYSQL_USER | root | DB user |
| MYSQL_PASS | (empty) | DB password |
| MYSQL_DB | db_archipelagowebsite | Central DB name (not MYSQL_DB_ARCHIPELAGO) |
| DEBUG | 0 | Set to 1 for verbose rate-fetch and circuit-breaker logging |
| url_image_resizer | https://images.archipelagohotels.com/ | Image CDN proxy base URL; if unset, raw CDN URLs are used and may be CSP-blocked |
| SIMPLEBOOKING_API_URL | https://xml.simplebooking.it/xmlservice.asmx/HotelAvailRQ | SB XML endpoint |
| SENTEC_API_URL | https://api.booking.sentec.io/sm/api/availability/search | Sentec REST (reserved, 0 hotels active) |

---

## Key Files

| File | Purpose |
|------|---------|
| cmd/archipelago-hotels-mcp/main.go | Entrypoint, transport dispatch |
| internal/server/server.go | MCP server wiring + HTTP routes (Gin) |
| internal/repository/repository.go | Pool, Config, HotelRow, BrandRow, RoomRow |
| internal/repository/hotel.go | SearchHotels, GetHotelByID, GetThumbnails, resizeImageURL |
| internal/repository/room.go | GetRooms (schema-adaptive), GetCredentials |
| internal/rate/rate.go | Service, BatchMinRates, circuitBreaker, SBClient |
| internal/rate/cache.go | rateCache (TTL, lazy expiry, no goroutine) |
| internal/rate/simplebooking.go | XML builder + parser for SB API |
| internal/rate/sentec.go | Sentec REST client (reserved) |
| internal/tools/search.go | search_hotels handler |
| internal/tools/recommend.go | recommend_hotel handler |
| internal/tools/dashboard.go | find_hotels handler |
| internal/tools/detail.go | get_hotel_detail handler (app-only); emits bookingUrl |
| internal/tools/open_url.go | open_booking_url handler — exec.Command("open", url) per OS |
| internal/resources/dashboard.go | MCP resource registration |
| ui/src/mcp-app.ts | TypeScript frontend (1200+ lines, single file) |
| Makefile | build-ui (Vite), build-go, build, dev-http targets |

---

## Architecture Decisions

### ADR-1: Go over Python
Single binary deployment, no runtime dependencies, superior concurrency for parallel brand DB queries, stdio transport cleanliness — no accidental stdout from an interpreter.

### ADR-2: Multi-DB Pool with lazy connect
Each brand DB connects on first access, guarded by RWMutex. `scanColumns` via INFORMATION_SCHEMA runs once per brand after connection is stored under the write lock. Do not call `connectBrand` or `scanColumns` outside the write lock.

### ADR-3: Rate fallback chain
Priority: SimpleBooking XML (live) → tb_hrooms.room_rate (stored) → hotel_starting_price (central). Circuit breaker trips at 5 failures, 120s cooldown. Cache TTL is 5 minutes.

### ADR-4: Gin for HTTP transport
Gin handles /mcp, /dashboard, /api/* and /health. MCP Streamable HTTP and dashboard share the same listener on :9011.

### ADR-5: MCP Apps ext-apps protocol for UI
UI protocol: MIMEType `text/html;profile=mcp-app`, resource URI `ui://hotel-dashboard`. Tools expose `_meta.ui.resourceUri` + `resourceDomains`. Tool call from UI uses `app.callServerTool({ name, arguments })`, not `app.tools.call()`.

### ADR-6: resizeImageURL for CSP-safe thumbnails
`resizeImageURL(rawURL, width)` rewrites CDN URLs to the approved image resizer host (url_image_resizer env var). This is a pure string transform — no HTTP fetch, no base64 encoding. Base64 proxying bloats responses by ~33% and moves CDN latency into the Go server.

### ADR-7: Raw hotel_currency code
Always return the raw `hotel_currency` column value from the DB row. Never hardcode `"Rp"` or any currency symbol in Go source. The UI handles display formatting via `fmtPrice(v, currency)` with `toLocaleString("id-ID")` for IDR.

---

## Known Pitfalls — Never Do These

1. **Never use a base64 image proxy.** Use `resizeImageURL` + `resourceDomains` instead.
2. **Never hardcode `"Rp"` or any currency symbol.** Always use `hotel_currency` from the DB row.
3. **Never omit `resourceDomains` from tool Meta** when the UI renders external images. Without it, the ext-apps CSP blocks the image silently.
4. **Never skip `hotel_id` filtering in brand DB queries.** Tables are partitioned by hotel_id. A missing WHERE clause scans all partitions.
5. **Never call `connectBrand` or `scanColumns` outside the write lock.** Race condition causes double-initialization.
6. **Never write to stdout in the Go binary.** Claude Desktop stdio transport reads stdout as JSON-RPC. Any stray `fmt.Print` or `log.Print` to stdout (not stderr) corrupts the protocol stream.
7. **Never work directly on the `main` branch.** Use feature branches.
8. **Never call `app.tools.call(name, args)` in the UI.** The correct call is `app.callServerTool({ name, arguments })`.
9. **Never fetch images server-side from DB-sourced URLs.** This is an SSRF risk. Use `resizeImageURL` (pure string rewrite only).

---

## Session Patterns

| Operation | Command |
|-----------|---------|
| Build after UI change | `make build` (runs Vite then go build) |
| Build after Go-only change | `make build-go` (skips Vite) |
| Verify build | `go vet ./...` |
| Run dev server | `make dev-http` |
| Health check | `curl http://localhost:9011/health` |

---

## Recurring Code Patterns

### GateGuard — required before every file edit
Per AGENTS.md, state 4 facts before editing:
1. What file is being changed and why (list all importers of any changed symbol).
2. What the current behavior is.
3. What the new behavior will be.
4. What could break.

### RegisterXxx tool pattern
Every tool lives in its own file under `internal/tools/`. Registration function:
```go
func RegisterSearch(s *mcp.Server, db *repository.Pool, rateSvc *rate.Service) {
    mcp.AddTool(s, &mcp.Tool{
        Name:        "search_hotels",
        Description: "...",
        InputSchema: mcp.MustSchema[SearchArgs](),
        Annotations: mcp.ToolAnnotations{Title: "..."},
    }, handler(db, rateSvc))
}
```
Wire the new `RegisterXxx` call in `internal/server/server.go` `New()`.

### BrandDB safe access
```go
db := pool.BrandDB(ctx, prefix)          // lazy connect, RWMutex guarded
ok := pool.HasColumn(prefix, "tb_hotels", "thumbnail_desktop")
```
Always check `HasColumn` before accessing schema-variable columns. Never assume a column exists across all brand DBs.

### Parallel brand queries
`repository.GetThumbnails()` fans out one goroutine per brand prefix, collects into `map[int]string` (hotel_id → URL), returns after a fixed timeout. Copy this pattern for any operation spanning multiple brand DBs.

### Panic recovery in handlers
Every MCP tool handler has a deferred `recover()` at the top. A nil pointer from a missing DB column or unreachable brand DB must not crash the server. Return a partial result with an error annotation instead.

---

## Brand DB Special Cases

### Booking URL lookup (GetBookingURL)
`hotel.go:GetBookingURL(ctx, prefix, apiHotelID)` queries the brand DB:
1. `HasColumn(prefix, "tb_hotels", "hotel_channel")` — PBA lacks this column; fallback to `hotel_simplebooking` directly
2. `hotel_channel = 'SENTEC'` → query `hotel_sentec_booking`
3. `hotel_channel = 'SB'` → query `hotel_simplebooking`
4. Returns `""` if channel unknown or URL column empty/missing
Called only in `detail.go` — never in `SearchHotels` (would require N brand DB queries per list call).

### Open booking URL (open_booking_url tool)
`tools/open_url.go` — app-only MCP tool. Validates http/https scheme, then:
- macOS: `exec.Command("open", url)`
- Linux: `exec.Command("xdg-open", url)`
- Windows: `exec.Command("rundll32", "url.dll,FileProtocolHandler", url)`
UI calls this via `appRef.callServerTool(...)` — the MCP server process runs outside Claude Desktop's iframe sandbox so it can open system browser.

### PBA (Powered By Archi)
- Table: `tb_hroom` (singular, no `s`)
- Primary key: UUID `CHAR(36)`, not `INT`
- Status column: `"status"` (not `"room_status"`), active value `"1"` (not `"Y"`)
- Lacks `hotel_channel`; `GetBookingURL` falls back to `hotel_simplebooking` for PBA

### brandDBName map (repository/repository.go)
The `db_prefix_name` column in `tb_brands` does not always match the literal DB name:
- `"fave"` → `"favewebsite"` (not `"favehotel"`)
- `"pba"` → `"pba"` (not `"pbawebsite"`)

Verify this map when adding a new brand.

### Bali city search
City filter must check both `hotel_address LIKE '%bali%'` AND `r.region_name LIKE '%bali%'` (JOIN on regions table). Bali hotels store the region in region_name, not the address field.

---

## UI Protocol (ext-apps)

- MIMEType: `text/html;profile=mcp-app`
- Resource URI: `ui://hotel-dashboard`
- Tools link via `_meta.ui.resourceUri` + `resourceDomains: ["images.archipelagohotels.com"]`
- `fmtPrice(v, currency)`: uses raw currency code from `hotel_currency` + `toLocaleString("id-ID")` for IDR
- Thumbnails: `resizeImageURL()` rewrites CDN → images.archipelagohotels.com (pure string transform, no HTTP fetch)
- Modal positioning: `openOverlay(cardEl, hotelId)` — right of card (+12px), fallback left, fallback center; mobile ≤600px uses CSS bottom-sheet via `!important` overrides

---

## Brands

Aston, Grand Aston, The Alana, favehotels, Hotel Neo, Kamuela Villas, Quest, Harper, Huxley, Nordic, Four Corners, PBA (Powered By Archi)

Brand gradient styles are defined in the `imageStyle` map in `repository/repository.go`.

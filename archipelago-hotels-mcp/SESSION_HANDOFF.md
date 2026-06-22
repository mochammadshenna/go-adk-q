# Session Handoff — Archipelago Hotels MCP

## Project

Go MCP server with embedded UI (ext-apps SDK) for Archipelago Hotels & Resorts (Sentec Tech product).
Searches 279+ hotels across 13 brands.

- Binary: `bin/archipelago-hotels-mcp`
- Build: `make build-ui && make build-go` (Vite singlefile → embeds HTML → compiles Go)
- Transport: stdio (Claude Desktop) | Streamable HTTP via Gin v1.11.0 on :9011
- Stack: Go 1.25, github.com/modelcontextprotocol/go-sdk v1.6.1

---

## Architecture

```
cmd/archipelago-hotels-mcp/   — main entrypoint, transport dispatch
internal/
  server/
    server.go                 — MCP server wiring + HTTP routes (Gin)
  repository/
    repository.go             — Pool, Config, HotelRow, BrandRow, RoomRow, scanColumns()
    hotel.go                  — SearchHotels(), GetHotelByID(), GetThumbnails(), resizeImageURL()
    room.go                   — GetRooms() (schema-adaptive), GetCredentials()
  tools/
    search.go                 — search_hotels tool
    recommend.go              — recommend_hotel tool
    dashboard.go              — find_hotels tool
    detail.go                 — get_hotel_detail tool (app-only, visibility: app)
  resources/
    dashboard.go              — MCP resource registration (ui://hotel-dashboard)
    mcp-app.html              — built by Vite (DO NOT edit directly)
  rate/
    rate.go                   — Service, BatchMinRates, circuitBreaker, SBClient
    cache.go                  — rateCache (TTL, lazy expiry, no goroutine)
    simplebooking.go          — XML builder + parser for SB API
    sentec.go                 — Sentec REST client (reserved, 0 hotels use it)
ui/src/mcp-app.ts             — entire frontend (TypeScript, vanilla, 1200+ lines)
Makefile                      — build-ui (Vite), build-go, build, dev-http targets
```

---

## MCP Tools (CURRENT — critical)

| Tool Name | File | Visibility | Purpose | Previously Was |
|-----------|------|------------|---------|---------------|
| `search_hotels` | `tools/search.go` | public | Search by city/brand/query; returns hotel list + prices | `find_hotels` |
| `recommend_hotel` | `tools/recommend.go` | public | Rank hotels by vibe/budget/purpose | (unchanged) |
| `find_hotels` | `tools/dashboard.go` | public | Browse/book all hotels with optional city/brand filter | `hotel_booking` |
| `get_hotel_detail` | `tools/detail.go` | app | Full hotel detail + room types; called by UI only | (unchanged) |

---

## Data Architecture

- `Pool.central` → `db_archipelagowebsite` (catalog, brands, regions)
- `Pool.brandDBs` → lazy-connect per brand prefix (aston, neo, fave, alana, harper, kamuela, quest, pba)
- Column introspection via `INFORMATION_SCHEMA` on connect (schema varies per brand)
- PBA special case: uses `tb_hroom` (not `tb_hrooms`), different status column, UUID hotel_id
- Hotels partitioned by hotel_id — always filter explicitly

### Rate Fallback Chain (priority order)

1. SimpleBooking XML API (live) — `OTA_HotelAvailRQ`, 5-worker bounded pool, 5-min TTL cache, circuit breaker (5 failures → 120s cooldown)
2. `tb_hrooms.room_rate` (stored) — per-brand DB fallback
3. `hotel_starting_price` (central DB) — last resort

---

## HTTP Endpoints (HTTP mode)

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/mcp` | POST/GET | MCP Streamable HTTP |
| `/dashboard` | GET | Standalone HTML dashboard |
| `/api/hotels` | GET | JSON hotel list (`?city=&brand=`) |
| `/api/brands` | GET | JSON brand list |
| `/api/regions` | GET | JSON region list |
| `/health` | GET | `{ status, version, db }` |

---

## UI Protocol

- MIMEType: `text/html;profile=mcp-app`
- Resource URI: `ui://hotel-dashboard`
- Tools link via `_meta.ui.resourceUri` + `resourceDomains: ["images.archipelagohotels.com"]`
- `fmtPrice(v, currency)`: raw currency code from `hotel_currency` + `toLocaleString(id-ID for IDR)` → renders as "IDR 406.000"
- Thumbnails: `resizeImageURL()` rewrites CDN → `images.archipelagohotels.com` (no HTTP fetch, CSP-safe)

---

## Environment Variables

| Var | Default | Purpose |
|-----|---------|---------|
| `MYSQL_HOST` | `127.0.0.1` | DB host |
| `MYSQL_PORT` | `3306` | DB port |
| `MYSQL_USER` | `root` | DB user |
| `MYSQL_PASS` | (empty) | DB password |
| `MYSQL_DB` | `db_archipelagowebsite` | Central DB name |
| `DEBUG` | `0` | Set to `1` for debug logging |
| `url_image_resizer` | `https://images.archipelagohotels.com/` | Image CDN proxy base URL |

---

## Session Summary — 2026-06-21

### Accomplished

- Renamed tool `find_hotels` → `search_hotels` (search functionality)
- Renamed tool `hotel_booking` → `find_hotels` (dashboard/booking UI tool)
- Added `resourceDomains` to all 3 public tools that render images (`search_hotels`, `find_hotels`, `get_hotel_detail`) so CDN thumbnail URLs pass the MCP iframe CSP
- Removed hardcoded `"Rp"` currency symbol; all tools now return raw `hotel_currency` from DB
- Rebuilt binary at 23:20 — `bin/archipelago-hotels-mcp` is current
- Updated `server.go` `ServerOptions.Instructions` to match new tool names

### Files Changed This Session

| File | Change |
|------|--------|
| `internal/tools/search.go` | Renamed tool `find_hotels` → `search_hotels`; added `resourceDomains` to tool Meta |
| `internal/tools/dashboard.go` | Renamed tool `hotel_booking` → `find_hotels`; added `resourceDomains` to tool Meta |
| `internal/tools/detail.go` | Added `resourceDomains` to tool Meta; replaced hardcoded `"Rp"` with `hotel.Currency` |
| `internal/tools/recommend.go` | Replaced hardcoded `"Rp"` with `hotel.Currency` |
| `internal/server/server.go` | Updated tool descriptions in `ServerOptions.Instructions` to match new tool names |
| `bin/archipelago-hotels-mcp` | Rebuilt (make build, 2026-06-21 23:20) |

---

## Brands

Aston, Grand Aston, The Alana, favehotels, Hotel Neo, Kamuela Villas, Quest, Harper, Huxley, Nordic, Four Corners, PBA (Powered By Archi)

Brand DB prefixes: `aston`, `neo`, `fave`, `alana`, `harper`, `kamuela`, `quest`, `pba`

---

## ADR History

1. Go + go-sdk for MCP (not Node.js)
2. Multi-DB Pool with lazy connect (not single DB)
3. Rate fallback chain (SB → stored → starting_price)
4. Gin for HTTP transport
5. MCP Apps ext-apps protocol for UI
6. `resizeImageURL` for CSP-safe thumbnails (no base64 proxy)
7. Raw `hotel_currency` code (not hardcoded symbol map)

---

## Known Issues / Blockers

- **Claude Desktop restart required**: Binary was rebuilt; Claude Desktop must be restarted to pick up the new tool names (`search_hotels`, `find_hotels`). Without restart, old tool names may still be cached.
- **Tool competition**: Booking.com, Trivago, and Wyndham MCPs in Claude Desktop compete with our tools. User must set Custom Instructions to prioritise archipelago-hotels-mcp tools. Instructions should reference the new tool names.
- **PBA rate data**: `db_pba` has no `hotel_channel` column and uses UUID `hotel_id`. Rate fetching for PBA hotels falls back to `hotel_starting_price` only.
- **Sentec API dead path**: Zero hotels currently have `hotel_channel = 'SENTEC'`. The Sentec client is implemented but the code path is never exercised. No action needed until a hotel is migrated.

---

## Pending Tasks

1. **Verify thumbnails load** — test in Claude Desktop after restart; expected: hotel card images appear instead of gradient fallback color.
2. **Verify currency format** — confirm UI renders "IDR 406.000" (raw code + id-ID locale), not blank or "Rp".
3. **Validate tool names in Claude Desktop** — run TEST_PROMPT.md #1 and #6 to confirm `search_hotels` and `find_hotels` are resolved correctly after restart.
4. **Update Claude Desktop Custom Instructions** — if still referencing old tool names (`find_hotels` for search, `hotel_booking`), update to current names.
5. **Phase 5: Caching** — LRU cache for hotel profiles and rate responses (see PLAN.md §4 Phase 5). Not started.
6. **Phase 6: Observability** — structured slog logging with request IDs (see PLAN.md §4 Phase 6). Not started.
7. **Integration tests** — no automated test suite exists; manual only via TEST_PROMPT.md.

---

## Build Commands

```bash
# Full rebuild (UI + Go binary)
make build-ui && make build-go

# Or combined
make build

# HTTP dev mode (port 9011)
make dev-http
```

After rebuilding, **restart Claude Desktop** to reload the MCP server binary.

---

## Environment State

| Item | State |
|------|-------|
| Branch | `main` |
| Binary | Built — `bin/archipelago-hotels-mcp` (2026-06-21 23:20) |
| Build status | Passing (`make build` succeeded) |
| Test status | Manual only via TEST_PROMPT.md (no automated suite) |
| DB connection | Requires local MySQL on `127.0.0.1:3306` with `db_archipelagowebsite` |

---

## Next Session Checklist

1. `git status && git branch` — confirm working tree state.
2. Read `MEMORY.md` — architecture context and known pitfalls.
3. Restart Claude Desktop if not already done since last binary build.
4. Run TEST_PROMPT.md #10 (stdio cleanliness) — confirm binary is healthy.
5. Run TEST_PROMPT.md #1 (full brand audit) — confirm thumbnails load and currency shows correctly.
6. Proceed to highest-priority pending task above.

# APPEND_SYSTEM.md — Appended System Instructions for AI Agents

These instructions are appended to the system prompt when an AI agent works on this project. They override general defaults where there is a conflict.

---

## Project: archipelago-hotels-mcp

Go MCP server for Archipelago Hotels & Resorts (Sentec Tech product). Searches 279+ hotels across 13 brands.

### Stack

- Go 1.25, `github.com/modelcontextprotocol/go-sdk` v1.6.1
- Transport: stdio (Claude Desktop) | Streamable HTTP via Gin v1.11.0 on `:9011`
- DB: Central MySQL `db_archipelagowebsite` + 8 per-brand DBs (lazy-connect Pool)
- UI: Vite+TypeScript single-file app, `//go:embed` into `mcp-app.html`, served as MCP App resource

### MCP Tools

| Name | Visibility | Purpose |
|------|-----------|---------|
| `search_hotels` | public | Search by city/brand/query; returns hotel list + prices |
| `recommend_hotel` | public | Rank hotels by vibe/budget/purpose |
| `find_hotels` | public | Browse/book all hotels with optional city/brand filter |
| `get_hotel_detail` | app-only (`visibility: app`) | Full hotel detail + room types; called by UI only |

### Data Architecture

- `Pool.central` → `db_archipelagowebsite` (catalog, brands, regions)
- `Pool.brandDBs` → lazy-connect per brand prefix (`aston`, `neo`, `fave`, `alana`, `harper`, `kamuela`, `quest`, `pba`)
- Column introspection via `INFORMATION_SCHEMA` on connect (schema varies per brand)
- PBA special case: uses `tb_hroom` (singular), different status column

### Rate Fallback Chain (priority order)

1. SimpleBooking XML API (live) — `OTA_HotelAvailRQ`, 5-worker bounded pool, 5-min TTL cache, circuit breaker (5 failures → 120s cooldown)
2. `tb_hrooms.room_rate` (stored) — per-brand DB fallback
3. `hotel_starting_price` (central DB) — last resort

### HTTP Endpoints (HTTP mode)

- `POST/GET /mcp` — MCP Streamable HTTP
- `GET /dashboard` — Standalone HTML dashboard
- `GET /api/hotels?city=&brand=` — JSON hotel list
- `GET /api/brands` — JSON brand list
- `GET /api/regions` — JSON region list
- `GET /health` — `{ status, version, db }`

### Environment Variables

| Var | Default | Purpose |
|-----|---------|---------|
| `MYSQL_HOST` | `127.0.0.1` | DB host |
| `MYSQL_PORT` | `3306` | DB port |
| `MYSQL_USER` | `root` | DB user |
| `MYSQL_PASS` | (empty) | DB password |
| `MYSQL_DB` | `db_archipelagowebsite` | Central DB name |
| `DEBUG` | `0` | Set to `1` for debug logging |
| `url_image_resizer` | `https://images.archipelagohotels.com/` | Image CDN proxy base URL |

### UI Protocol

- MIMEType: `text/html;profile=mcp-app`
- Resource URI: `ui://hotel-dashboard`
- Tools link via `_meta.ui.resourceUri` + `resourceDomains: ["images.archipelagohotels.com"]`
- `fmtPrice(v, currency)`: raw currency code from `hotel_currency` + `toLocaleString(id-ID for IDR)`
- Thumbnails: `resizeImageURL()` rewrites CDN → `images.archipelagohotels.com` (no HTTP fetch)

### Key Files

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
| `internal/rate/sentec.go` | Sentec REST client (reserved, 0 hotels use it) |
| `internal/tools/search.go` | `search_hotels` handler |
| `internal/tools/recommend.go` | `recommend_hotel` handler |
| `internal/tools/dashboard.go` | `find_hotels` handler |
| `internal/tools/detail.go` | `get_hotel_detail` handler (app-only) |
| `internal/resources/dashboard.go` | MCP resource registration |
| `ui/src/mcp-app.ts` | TypeScript frontend (1200+ lines, single file) |
| `Makefile` | `build-ui` (Vite), `build-go`, `build`, `dev-http` targets |

### Brands

Aston, Grand Aston, The Alana, favehotels, Hotel Neo, Kamuela Villas, Quest, Harper, Huxley, Nordic, Four Corners, PBA (Powered By Archi)

### ADR History (Do Not Reverse Without Discussion)

1. Go + go-sdk for MCP transport (not Node.js)
2. Multi-DB Pool with lazy connect (not a single shared DB)
3. Rate fallback chain (SB live → stored → starting_price)
4. Gin for HTTP transport layer
5. MCP Apps ext-apps protocol for the embedded UI
6. `resizeImageURL()` for CSP-safe thumbnails (no base64 proxy, no server-side image fetch)
7. Raw `hotel_currency` code passed to frontend (not a hardcoded symbol map)

---

## CRITICAL RULES FOR AI AGENTS IN THIS REPO

### Mandatory Pre-Work

Before making any changes:

1. Read `AGENTS.md` — GateGuard hook rules that govern all file edits.
2. Read `MEMORY.md` — architecture decisions, known bugs, and resolved pitfalls. Do not re-introduce them.
3. Read `SESSION_HANDOFF.md` — current project state and pending tasks.

The GateGuard hook requires you to state 4 facts before editing any file:

- What file is being changed and why.
- What the current behavior is.
- What the new behavior will be.
- What could break.

### Before Editing ANY Go File

You MUST present these facts in your response **before** the Edit tool call:

1. List ALL files that import this file (use Grep).
2. List the public functions/types affected.
3. State if this file reads/writes data files (show field names if yes).
4. Quote the user's current instruction verbatim.

### Before Editing ANY File

You MUST Read the file first in the current session before any Edit call. Do not Edit a file you have not Read in this session.

### Build Process (TWO STEPS — both required after UI changes)

```sh
make build-ui   # Vite build → ui/dist/index.html → internal/resources/mcp-app.html
make build-go   # Go binary embeds the HTML
```

After Go-only changes: `make build-go` only. When in doubt: `make build` (full build).

Never claim that a change works without running the appropriate build first. The Go binary is what Claude Desktop loads — source edits have no effect until the binary is rebuilt.

The user must restart Claude Desktop after the binary is updated.

### NEVER DO

- Make HTTP requests to URLs sourced from database fields (SSRF risk).
- Hardcode currency symbols — use the raw `hotel_currency` code from the DB. The string `"Rp"` must never appear in Go source code.
- Access brand DB columns without a `HasColumn()` check (schema varies per brand).
- Import packages before writing the functions that use them.
- Write to stdout in production code paths (`fmt.Print*`, `fmt.Println`, `os.Stdout.Write`). The stdio transport uses stdout for JSON-RPC; anything else corrupts the stream. Use `log.Print*` (writes to stderr) for all logging.
- Commit directly to the `main` branch. All work goes on a feature branch.

### Rate Fallback Chain — Do Not Break

```
SimpleBooking live API → tb_hrooms.room_rate → hotel_starting_price
```

Modifying any step requires verifying all three steps still function correctly.

### Tool Naming (Current — Do Not Rename)

```
search_hotels | recommend_hotel | find_hotels | get_hotel_detail (app-only)
```

### Schema Safety

- Always guard per-brand DB column access with `HasColumn()`.
- PBA uses `tb_hroom` (singular), not `tb_hrooms`. Its status column differs from other brands.
- The central DB (`db_archipelagowebsite`) schema is stable; brand DBs are not.

### Database Partitioning

All queries against `db_archipelagowebsite.tb_hotels` and all per-brand `tb_hotels` tables must include an explicit `hotel_id` filter. These tables are partitioned by `hotel_id`. A query without a WHERE clause on `hotel_id` will scan all partitions and is a performance bug.

### Tool Registration Pattern

New tools are registered in `internal/server/server.go` inside the `New()` function. Each tool lives in its own file under `internal/tools/`. The `RegisterXxx` function takes `(s *mcp.Server, db *repository.Pool, rateSvc *rate.Service)`. Do not store the DB pool or rate service as package-level globals.

### Image Handling Rules

Thumbnail images from CDN domains are blocked by the MCP iframe's Content Security Policy unless explicitly allowed. The correct approach:

1. Use `resizeImageURL(rawURL, width)` to rewrite the CDN URL through the approved resizer host.
2. Include the CDN domain(s) in the tool's `resourceDomains` metadata so the CSP allowlist is updated.

Do not fetch images server-side and re-encode them as base64 data URIs. That approach is explicitly rejected — see `MEMORY.md` for the reasoning.

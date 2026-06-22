# PRODUCT.md — Archipelago Hotels MCP Server

> Sentec Tech product. Bridges Sentec MySQL data with Claude AI for natural language hotel discovery across all Archipelago Hotels & Resorts brands.

---

## Project Overview

**Project**: archipelago-hotels-mcp
**Owner**: Sentec Tech (hospitality software division)
**Operator**: Archipelago Hotels & Resorts
**Primary Interface**: Claude Desktop (stdio MCP)

Go MCP server enabling natural language hotel search, recommendation, and browsing across 279+ hotels in 13 Archipelago brands.

---

## Product Vision

Enable natural language hotel discovery across all Archipelago Hotels & Resorts brands via Claude — allowing staff and integrators to search, compare, and surface hotel data conversationally, without SQL or manual lookups.

---

## Business Context

| Entity | Role |
|--------|------|
| **Archipelago Hotels & Resorts** | Hotel management company. Operates the hotel portfolio. |
| **Sentec Tech** | Hospitality software company. Owns trademark "Sentec". Builds Sentec PMS, Sentec Booking Engine, Sentec EMS, and this MCP server. |

This server connects Sentec's MySQL hotel catalog and per-brand room databases to Claude's tool-calling infrastructure. It is a read-only integration — no booking transactions are initiated through this server.

---

## Target Users

### Internal — Archipelago Hotels Staff

Archipelago staff using Claude Desktop for hotel portfolio lookups, competitive analysis, and guest-facing research support.

### Technical — Sentec Tech Developers

Developers building AI-assisted hospitality workflows who integrate hotel search and recommendation capabilities into Claude-based pipelines.

---

## Key User Stories

1. **Search by city and brand**
   > "Show me all Aston hotels in Bali with prices"
   - Calls `search_hotels` with city=Bali, brand=Aston
   - Returns hotel list with rate data from the fallback chain

2. **Recommendation by purpose**
   > "Recommend a luxury hotel in Jakarta for a business trip"
   - Calls `recommend_hotel` with vibe=luxury, purpose=business, city=Jakarta
   - Ranks results and surfaces top picks with rationale

3. **Visual dashboard browse**
   > "Browse all Archipelago hotels"
   - Calls `find_hotels` which opens the MCP App UI
   - User browses the embedded Vite+TypeScript dashboard with filters

4. **Hotel detail (UI-initiated)**
   > User clicks a hotel card in the dashboard
   - UI calls `get_hotel_detail` (app-only visibility)
   - Returns full hotel detail including room types and rates

---

## Stack

| Layer | Technology |
|-------|------------|
| Language | Go 1.25 |
| MCP SDK | github.com/modelcontextprotocol/go-sdk v1.6.1 |
| HTTP Framework | Gin v1.11.0 |
| Transport (Claude Desktop) | stdio |
| Transport (HTTP) | Streamable HTTP on :9011 |
| Central DB | MySQL — db_archipelagowebsite |
| Brand DBs | 8 per-brand MySQL databases (lazy-connect Pool) |
| Frontend | Vite + TypeScript (single-file, `//go:embed` into mcp-app.html) |

---

## MCP Tools

| Name | Visibility | Purpose |
|------|-----------|---------|
| `search_hotels` | public | Search by city/brand/query; returns hotel list + prices |
| `recommend_hotel` | public | Rank hotels by vibe/budget/purpose |
| `find_hotels` | public | Browse all hotels with optional city/brand filter; opens UI |
| `get_hotel_detail` | app-only (`visibility: app`) | Full hotel detail + room types + booking URL; called by UI only |
| `open_booking_url` | app-only (`visibility: app`) | Open hotel booking URL in system browser; called by UI "Book Now" button |

---

## Data Architecture

- `Pool.central` — db_archipelagowebsite (catalog, brands, regions)
- `Pool.brandDBs` — lazy-connect per brand prefix: `aston`, `neo`, `fave`, `alana`, `harper`, `kamuela`, `quest`, `pba`
- Column introspection via `INFORMATION_SCHEMA` on connect (schema varies per brand)
- PBA special case: uses `tb_hroom` (not `tb_hrooms`), different status column

### Rate Fallback Chain (priority order)

1. **SimpleBooking XML API** (live) — OTA_HotelAvailRQ, 5-worker bounded pool, 5-min TTL cache, circuit breaker (5 failures → 120s cooldown)
2. **tb_hrooms.room_rate** (stored) — per-brand DB fallback
3. **hotel_starting_price** (central DB) — last resort

---

## HTTP Endpoints (HTTP mode)

| Method | Path | Description |
|--------|------|-------------|
| POST/GET | `/mcp` | MCP Streamable HTTP |
| GET | `/dashboard` | Standalone HTML dashboard |
| GET | `/api/hotels` | JSON hotel list (`?city=&brand=`) |
| GET | `/api/brands` | JSON brand list |
| GET | `/api/regions` | JSON region list |
| GET | `/health` | `{ status, version, db }` |

---

## Environment Variables

| Variable | Default | Purpose |
|----------|---------|---------|
| `MYSQL_HOST` | `127.0.0.1` | DB host |
| `MYSQL_PORT` | `3306` | DB port |
| `MYSQL_USER` | `root` | DB user |
| `MYSQL_PASS` | (empty) | DB password |
| `MYSQL_DB` | `db_archipelagowebsite` | Central DB name |
| `DEBUG` | `0` | Set to `1` for debug logging |
| `url_image_resizer` | `https://images.archipelagohotels.com/` | Image CDN proxy base URL |

---

## UI Protocol

- **MIMEType**: `text/html;profile=mcp-app`
- **Resource URI**: `ui://hotel-dashboard`
- Tools link via `_meta.ui.resourceUri` + `resourceDomains: ["images.archipelagohotels.com"]`
- `fmtPrice(v, currency)`: raw currency code from `hotel_currency` + `toLocaleString(id-ID)` for IDR
- Thumbnails: `resizeImageURL()` rewrites CDN to `images.archipelagohotels.com` (no HTTP fetch, CSP-safe)

---

## Brands

Aston, Grand Aston, The Alana, favehotels, Hotel Neo, Kamuela Villas, Quest, Harper, Huxley, Nordic, Four Corners, PBA (Powered By Archi)

Brand-specific `imageStyle` gradients are defined in `internal/repository/repository.go`.

---

## Key Files

| File | Purpose |
|------|---------|
| `cmd/archipelago-hotels-mcp/main.go` | Entrypoint, transport dispatch |
| `internal/server/server.go` | MCP server wiring + HTTP routes (Gin) |
| `internal/repository/repository.go` | Pool, Config, HotelRow, BrandRow, RoomRow |
| `internal/repository/hotel.go` | SearchHotels, GetHotelByID, GetThumbnails, resizeImageURL |
| `internal/repository/room.go` | GetRooms (schema-adaptive), GetCredentials |
| `internal/rate/rate.go` | Service, BatchMinRates, circuitBreaker, SBClient |
| `internal/rate/cache.go` | rateCache (TTL, lazy expiry, no goroutine) |
| `internal/rate/simplebooking.go` | XML builder + parser for SB API |
| `internal/rate/sentec.go` | Sentec REST client (reserved, 0 hotels use it) |
| `internal/tools/search.go` | search_hotels handler |
| `internal/tools/recommend.go` | recommend_hotel handler |
| `internal/tools/dashboard.go` | find_hotels handler |
| `internal/tools/detail.go` | get_hotel_detail handler (app-only) |
| `internal/tools/open_url.go` | open_booking_url handler (app-only; opens booking URL via exec.Command) |
| `internal/resources/dashboard.go` | MCP resource registration |
| `ui/src/mcp-app.ts` | TypeScript frontend (1200+ lines, single file) |
| `Makefile` | `build-ui` (Vite), `build-go`, `build`, `dev-http` targets |

---

## Product Constraints

- **Read-only**: No booking transactions are initiated through this server
- **Offline resilience**: Graceful DB degradation through rate fallback chain; must function without live SB API
- **Claude Desktop primary**: stdio transport is the primary interface; HTTP mode is secondary
- **CSP compliance**: All image URLs rewritten via `resizeImageURL()` — no external HTTP fetches from the UI
- **Indonesian-first**: Primary currency IDR with `id-ID` locale formatting; future roadmap includes USD/VND/PHP

---

## Success Metrics

| Metric | Description |
|--------|-------------|
| Tool call success rate | Percentage of MCP tool calls returning valid hotel data |
| UI open rate | Rate at which `find_hotels` triggers the dashboard UI |
| Rate data coverage | Percentage of hotels with live or stored rate data available |

---

## Architecture Decision Records

| ADR | Decision |
|-----|----------|
| 1 | Go + go-sdk for MCP (not Node.js) |
| 2 | Multi-DB Pool with lazy connect (not single DB) |
| 3 | Rate fallback chain: SB → stored → starting_price |
| 4 | Gin for HTTP transport |
| 5 | MCP Apps ext-apps protocol for UI |
| 6 | resizeImageURL for CSP-safe thumbnails (no base64 proxy) |
| 7 | Raw hotel_currency code (not hardcoded symbol map) |
| 8 | open_booking_url via exec.Command — Electron iframe sandbox blocks window.open(); server-side OS command is the only reliable cross-platform path |

---

## Roadmap

| Phase | Item | Status |
|-------|------|--------|
| Phase 3 | Sentec API integration (`internal/rate/sentec.go` reserved) | Planned |
| Phase 5 | Caching improvements | Planned |
| Future | Non-Indonesian hotel support (USD, VND, PHP currencies) | Backlog |
| Future | Booking transaction capability | Not in scope (requires separate authorization) |

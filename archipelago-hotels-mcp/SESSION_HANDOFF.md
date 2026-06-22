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

| Tool Name | File | Visibility | Purpose |
|-----------|------|------------|---------|
| `search_hotels` | `tools/search.go` | public | Search by city/brand/query; returns hotel list + prices |
| `recommend_hotel` | `tools/recommend.go` | public | Rank hotels by vibe/budget/purpose |
| `find_hotels` | `tools/dashboard.go` | public | Browse/book all hotels with optional city/brand filter |
| `get_hotel_detail` | `tools/detail.go` | app | Full hotel detail + room types; called by UI only |

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
- Tools link via `_meta.ui.resourceUri` + `_meta.ui.csp.resourceDomains` (MUST be nested under `csp` key)
- `fmtPrice(v, currency)`: raw currency code from `hotel_currency` + `toLocaleString(id-ID for IDR)`
- Thumbnails: `resizeImageURL()` rewrites CDN → `images.archipelagohotels.com` (no HTTP fetch, CSP-safe)

### CSP Structure (critical — do not regress)

All tools and the dashboard resource must use this exact nesting:

```go
Meta: mcp.Meta{
    "ui": map[string]any{
        "resourceUri": resources.ResourceURI,   // tools only
        "csp": map[string]any{
            "resourceDomains": pool.ImageDomains(), // MUST be under "csp"
        },
    },
}
```

`resourceDomains` at `_meta.ui.resourceDomains` (wrong level) is silently ignored by Claude Desktop → images broken.
`ImageDomains()` returns `https://`-prefixed origins (bare hostnames are invalid CSP directives).

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

Developers with matching local MySQL defaults need no `env{}` block in Claude Desktop config.

---

## Brands

Aston, Grand Aston, The Alana, favehotels, Hotel Neo, Kamuela Villas, Quest, Harper, Huxley, Nordic, Four Corners, PBA (Powered By Archi)

Brand DB prefixes: `aston`, `neo`, `fave`, `alana`, `harper`, `kamuela`, `quest`, `pba`

Special case: `astonhotelsinternational` bucket → remapped to `astoninternational` in `resizeImageURL()`

---

## Rate & Room Pipeline (current)

### Key structs

```go
// rate/rate.go
type SBRate struct {
    RoomName           string
    RoomID             string  // RoomTypeCode from SB XML — matches tb_hrooms.sb_id
    BeforeDiscountRate float64 // Base.AmountBeforeTax — rack rate before discount
    TotalAfterTax      float64 // Total.AmountAfterTax  — final price
}

type RoomRate struct {
    Name         string  `json:"name"`
    RatePerNight float64 `json:"ratePerNight"`
    BaseRate     float64 `json:"baseRate,omitempty"`
    Source       string  `json:"source"`
    RoomImage    string  `json:"roomImage,omitempty"` // from tb_hrooms
}

// rate/sentec.go — future use
type SentechDayRate struct {
    BaseRate  float64 `json:"base_rate"`
    Discount  float64 `json:"discount"`
    FinalRate float64 `json:"final_rate"`
    TaxPrice  float64 `json:"tax_price"`
}
```

### trySB() room enrichment flow
1. SB XML API → `[]SBRate` with `RoomID` = `RoomTypeCode` attribute
2. `GetRooms()` → `[]RoomRow` with `SBID` + `RoomImage`
3. Match: `strconv.ParseInt(SBRate.RoomID)` == `RoomRow.SBID.Int64`
4. Copy `RoomImage` + fallback room name into `RoomRate`

### Room image column selection (room.go)
- **PBA** (`brandPrefix == "pba"`): `COALESCE(thumbnail_desktop, '')` — no `room_image` column in `tb_room`
- **Others**: `COALESCE(room_image, thumbnail_desktop, '')` — both guarded by `HasColumn`

### PBA table
`tb_room` (not `tb_hrooms`, not `tb_hroom`) — status col = `status`, val = `1`

---

## UI — Pricing & Display (current)

- "START FROM" label on cards: `var(--text-2)`, 11px, weight 600
- Starting-from price (green): `#2BC14B`
- Rooms & Suites prices: fixed `#00215b` (dark blue)
- Strikethrough condition: `(room.baseRate ?? 0) > room.pricePerNight`
- Original price: red line-through `#e53e3e`, 12px
- Brand badge: top-left of modal hero via `.overlay-hero-top { position:absolute; top:12px; left:14px }`
- Badge colour: pastel via `pastel(c) = round(c*0.15 + 255*0.85)`
- Room card thumbnail: `<img class="room-card-img">` 120px height, validated `https://` or `/` prefix

---

## ADR History

1. Go + go-sdk for MCP (not Node.js)
2. Multi-DB Pool with lazy connect (not single DB)
3. Rate fallback chain (SB → stored → starting_price)
4. Gin for HTTP transport
5. MCP Apps ext-apps protocol for UI
6. `resizeImageURL` for CSP-safe thumbnails (no base64 proxy)
7. Raw `hotel_currency` code (not hardcoded symbol map)
8. `resourceDomains` under `_meta.ui.csp` (not `_meta.ui`) — ext-apps spec requirement

---

## Known Issues / Blockers

- **Strikethrough base rate unverified**: `Base.AmountBeforeTax` in SB XML may be 0 or pre-tax (less than final). Run `DEBUG=1 make dev-http`, open hotel modal, check logs for `sb rate fields room=...` — confirm `parsed_before > parsed_total`. If always 0, need different SB XML field.
- **Room images untested end-to-end**: `room_image` / `thumbnail_desktop` column presence unverified per brand. `HasColumn` guards prevent crashes but images may silently not appear.
- **Tool competition**: Booking.com, Trivago, and Wyndham MCPs compete with our tools in Claude Desktop. Set Custom Instructions to prioritise archipelago-hotels-mcp.
- **PBA rate data**: `db_pba` has no `hotel_channel` column. Rate fetching falls back to `hotel_starting_price` only.
- **Sentec API dead path**: Zero hotels use Sentec. `SentechDayRate` struct ready; client not implemented.
- **City filter not auto-selected**: UI shows "All Cities" even when `recommend_hotel`/`search_hotels` returns a destination.

---

## Pending Tasks

1. **Verify strikethrough base rate** — run with `DEBUG=1`, check `sb rate fields` log. If `Base.AmountBeforeTax` is always 0, find correct SB XML rack-rate field.
2. **Test room images** — open PBA hotel modal (thumbnail_desktop), non-PBA hotel (room_image or thumbnail_desktop fallback).
3. **Fix city filter auto-select** — `ui/src/mcp-app.ts` → `showDashboard()` after `populateFilters()`:
   ```ts
   const autoCity = (data as any).destination || (data as any).city || "";
   if (autoCity && citySel) { state.cityFilter = autoCity; citySel.value = autoCity; }
   applyFilters();
   ```
4. **Phase 5: Caching** — LRU for hotel profiles and rates. Not started.
5. **Phase 6: Observability** — structured slog with request IDs. Not started.
6. **Integration tests** — no automated suite; manual only.

---

## Session Changes — 2026-06-22

### This session (room images + rate pipeline)

| File | Change |
|------|--------|
| `repository/repository.go` | `RoomRow` + `RoomImage string` |
| `repository/room.go` | PBA table `tb_hroom` → `tb_room`; brand-aware image column selection; scanRoom 6th target |
| `rate/rate.go` | `RoomRate` + `RoomImage`; `SBRate` + `RoomID`; `trySB()` room enrichment via sb_id match; `tryStored()` passes RoomImage; added `strconv` |
| `rate/simplebooking.go` | `RoomRateXML` + `RoomTypeCode` attr; `SBRate.RoomID` populated; debug log for rate fields; added `log/slog` |
| `rate/sentec.go` | `SentechDayRate` struct added |
| `tools/detail.go` | `roomImage` key in room map |
| `ui/src/mcp-app.ts` | `RoomType.roomImage?`; room card thumbnail; `.room-card-img` CSS; URL validation (`https://` or `/`) |

### Earlier same day (UI pricing + badge)

| File | Change |
|------|--------|
| `rate/simplebooking.go` | `AmountBeforeTax` added to `RateAmountXML`; `parseAmountAttr` → `parseAttr(string)`; `parseRatesBlock` uses `Base.AmountBeforeTax` |
| `rate/rate.go` | `SBRate`: `AmountAfterTax` → `BeforeDiscountRate` + `TotalAfterTax` |
| `ui/src/mcp-app.ts` | Badge top-left; pastel badge colour; room price `#00215b`; strikethrough pricing; "START FROM" 11px visible |
| `DESIGN.md` | §5.7 modal price colour scheme |

### Previous session (CSP / thumbnail fix)

| File | Change |
|------|--------|
| `repository/repository.go` | `ImageDomains()` returns `https://`-prefixed origins |
| `tools/dashboard.go` `tools/recommend.go` `tools/search.go` `tools/detail.go` `resources/dashboard.go` | `resourceDomains` moved under `csp` key |

---

## Build Commands

```bash
make build          # full rebuild — required after any mcp-app.ts edit
make build-go       # Go only (fast)
make dev-http       # HTTP mode :9011
DEBUG=1 make dev-http  # with verbose debug logging
```

After rebuilding, restart Claude Desktop to reload the binary.

---

## Environment State

| Item | State |
|------|-------|
| Branch | `main` |
| Binary | `bin/archipelago-hotels-mcp` — built 2026-06-22 20:30 |
| Build | Passing |
| Tests | Manual only |
| DB | Local MySQL `127.0.0.1:3306`, `db_archipelagowebsite` |

---

## Next Session Checklist

1. `git status && git branch`
2. Read `SESSION_HANDOFF.md` + `MEMORY.md`
3. `DEBUG=1 make dev-http` → open hotel modal → check `sb rate fields` logs for base rate values
4. Test room images in modal (PBA and non-PBA)
5. Fix city filter auto-select if needed

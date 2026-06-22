# Archipelago Hotels MCP — Design Document

> **Product**: Sentec PMS / Archipelago Hotels MCP Server
> **Maintainer**: Sentinel Tech
> **Last Updated**: 2026-06-22
> **Status**: Active

---

## 1. Design Philosophy

Four principles shape every decision in this codebase.

**1.1 Minimal dependencies, single binary.**
The server compiles to one binary (`archipelago-hotels-mcp`) with no runtime requirements beyond a MySQL host. The UI is embedded via `//go:embed`. There is no Node runtime, no container orchestration, no sidecar. The server runs as a Claude Desktop MCP process (stdio) or a standalone HTTP service, and both modes exit from the same `main.go`.

**1.2 Graceful degradation over hard failures.**
The rate fallback chain (SimpleBooking → stored → starting price) means a tool handler never returns an error solely because pricing is unavailable. Hotel list results without prices are still valid and useful. Brand DBs that are unreachable are silently bypassed; only the central DB is load-bearing for any response to succeed.

**1.3 Read-only data access.**
Every SQL query is a `SELECT`. There are no writes, no transactions, no DDL. This eliminates an entire class of data-safety risk and makes the server safe to point at a production replica.

**1.4 Schema variance is a first-class problem.**
The brand databases were built independently and have diverged. Rather than writing defensive SQL for every known variant, `Pool.scanColumns` introspects `INFORMATION_SCHEMA` at connection time and builds a per-brand column map. Query construction then consults this map instead of assuming column existence.

---

## 2. API Design

### 2.1 Tool naming conventions

Public tools follow `verb_noun` (imperative commands for an AI agent): `search_hotels`, `recommend_hotel`, `find_hotels`. The app-only tool is named `get_hotel_detail`, following the same convention but hidden from Claude's tool list via `"visibility": ["app"]` in the tool `Meta` field. The dashboard UI calls it directly over MCP; Claude does not see it in its available tool list.

### 2.2 Input schema patterns

All tool inputs are fully optional except `destination` in `recommend_hotel`. Claude's function-calling behaviour works best when tools do not reject calls that omit optional fields. A search with no filters returns all hotels — a valid and useful response. Required fields are kept to the minimum needed to produce a non-ambiguous result.

| Tool | Required | Optional |
|---|---|---|
| `search_hotels` | (none) | city, country, brand, query |
| `recommend_hotel` | destination | vibe, budget, purpose |
| `find_hotels` | (none) | city, brand |
| `get_hotel_detail` | hotelId | (none) |

### 2.3 Response patterns

All tool handlers return two values: `*mcp.CallToolResult` (always `nil` — the go-sdk derives the text content from the structured value) and a typed struct as the second return value (`searchResult`, `recommendResult`, `map[string]any`). The go-sdk serialises the second return as `structuredContent`. Claude receives both a text rendering and a machine-readable JSON object.

The `_meta.ui` field on public tools carries `resourceUri` and `resourceDomains`. This is the MCP Apps ext-apps protocol: it signals to a supporting client that when this tool's result is displayed, it should render the linked MCP App resource (`ui://hotel-dashboard`) in a side panel. `resourceDomains` pre-authorises the CSP for image loading from `images.archipelagohotels.com`.

### 2.4 Error handling in handlers

Every handler wraps its body in `defer recover()`. A nil pointer from a missing brand DB or a malformed DB row must not crash the server process. Panics are logged at `ERROR` level and converted to a structured error return, which the go-sdk sends as a tool error result to the client. Tool errors use `fmt.Errorf` with `%w` wrapping throughout, so the error chain is always preserved in logs.

---

## 3. Data Layer Design

### 3.1 Pool as the single connection manager

`repository.Pool` is the single dependency injected into every tool handler and the rate service. It holds:

- `central *sql.DB` — the connection to `db_archipelagowebsite`
- `brandDBs map[string]*sql.DB` — lazily populated per brand prefix
- `brandCols map[string]map[string]map[string]bool` — column introspection results (`brand → table → column → true`)
- `brands map[int]BrandRow` — brand catalog cached at startup
- `mu sync.RWMutex` — read-write lock protecting all mutable state

The `Pool` is constructed once in `main.go`, shared across all tools, and closed on process exit. There is no per-request connection management.

### 3.2 Lazy brand DB connections

Brand DB connections are not opened at startup. `Pool.BrandDB(ctx, prefix)` checks the `brandDBs` map under a read lock first (hot path). On a miss, it calls `connectBrand` outside the lock (slow path: TCP connect + ping, up to 3 seconds), then re-acquires a write lock to store the result. A double-check inside the write lock prevents two goroutines from both connecting and leaking one connection.

If `connectBrand` fails (DB unreachable, wrong credentials, DB not provisioned for that brand), it returns `nil` and stores `nil` in the map. Subsequent calls for that prefix immediately return `nil` without retrying, avoiding thundering-herd reconnect attempts on every request.

All callers handle a `nil` brand DB by returning empty results or falling back to central DB data. The nil-safety invariant is: **no method on `Pool` panics when a brand DB is nil**.

### 3.3 INFORMATION_SCHEMA column introspection

`Pool.scanColumns` runs once per brand DB immediately after connection, inside the write lock:

```sql
SELECT TABLE_NAME, COLUMN_NAME
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_SCHEMA = (SELECT DATABASE())
  AND TABLE_NAME IN ('tb_hotels', 'tb_hrooms', 'tb_hroom')
```

The result is stored as `brandCols[prefix][table][column] = true`. Query builders call `pool.HasColumn(prefix, table, column)` to decide which columns to include in a `SELECT`. This replaces a complex matrix of per-brand SQL variants with a single adaptive path.

The PBA special case (`tb_hroom` singular, UUID primary key, `hotel_simplebooking` column) is handled entirely by this introspection — PBA's schema differences appear as column presence/absence in the map rather than bespoke code branches.

### 3.4 Brand-to-DB mapping

`tb_brands.db_prefix_name` provides the prefix for most brands. Two brands require an explicit override in the `brandDBName` static map:

- `favehotel` → `db_favewebsite` (prefix drops the trailing `s`)
- `pba` → `db_pba` (no `website` suffix)

All others follow `db_{prefix}website`. Sub-brands (Grand Aston, The Royal Alana) resolve to their parent brand's prefix via `Pool.BrandPrefix`, which walks the `parent_brand_id` chain one level.

### 3.5 Central DB queries

`SearchHotels` queries `db_archipelagowebsite.tb_hotels` joined to `tb_brands` and `tb_region`. Filtering uses `LIKE '%value%'` on `region_name` (city), `brand_name` (brand), and a multi-column `LIKE` OR clause on `hotel_name`/`region_name`/`brand_name` for free-text queries. Every query includes `WHERE h.hotel_status = 1` and a hard `LIMIT` (default 50, max 100) to prevent unbounded scans.

`GetHotelByID` is a point lookup on `hotel_id` (primary key) and is always a single-row scan regardless of table size.

---

## 4. Rate Service Design

### 4.1 Fallback chain rationale

Three data sources exist for pricing, in priority order:

| Priority | Source | Notes |
|---|---|---|
| 1 | SimpleBooking XML API (live) | OTA_HotelAvailRQ; reflects today's availability-adjusted pricing |
| 2 | `tb_hrooms.room_rate` (stored) | Loaded by hotels team; may be weeks or months stale |
| 3 | `hotel_starting_price` (central DB) | Marketing starting price; used in `BatchMinRates` as last resort |

The fallback chain is implemented in `rate.Service.GetRates` for single-hotel lookups and `BatchMinRates` for batch operations. A `nil` result from one source triggers the next; an error is logged and treated as `nil` (not propagated to the caller).

### 4.2 Circuit breaker

SimpleBooking is a third-party SOAP/XML service with variable reliability. The circuit breaker in `SBClient` uses a threshold model: 5 consecutive failures open the circuit for 120 seconds. During the open state, `cb.Allow()` returns `false` and the SB path is skipped entirely, falling immediately to the stored-rate fallback.

A simple time-window breaker is used rather than a sliding-window. A sliding-window requires a ring buffer and is harder to reason about. The SimpleBooking endpoint either works or does not for extended periods — a 120-second cooldown is sufficient to avoid cascading failures. The code marks the upgrade path if false positives become a problem.

### 4.3 Bounded worker pool

`BatchMinRates` runs one goroutine per hotel, limited to 5 concurrent workers via a semaphore channel (`chan struct{}`). Rationale:

- Too few workers (1–2) makes a 50-hotel search too slow (SB latency is 300ms–2s per call).
- Too many workers creates a connection storm against the SimpleBooking endpoint.
- 5 workers processes 50 hotels in ~10 batches; acceptable latency for a tool result.

Each goroutine respects `ctx.Done()` via the semaphore acquire `select`, so context cancellation stops all pending goroutines promptly.

### 4.4 Rate cache

`rateCache` is a simple `sync.RWMutex`-protected map with TTL expiry. Key design decisions:

- **No background goroutine.** Expiry is lazy: checked on `Get`, evicted there. This avoids a leaked ticker goroutine when the server is embedded in a test or shut down quickly.
- **5-minute TTL.** Hotel pricing changes intraday but not per-minute. A `BatchMinRates` call for 50 hotels that warms the cache means subsequent calls within 5 minutes are served entirely from cache.
- **Cache key: `prefix:hotelID`.** Unique across all brand databases. Not keyed by check-in/check-out dates — the server always fetches today/tomorrow availability as a baseline price indicator. Adding date-specific caching would require the key to include the date range.

---

## 5. UI Design

### 5.1 Single TypeScript file

`ui/src/mcp-app.ts` is a single 1200+ line TypeScript file. No component framework (React, Vue, Svelte). Rationale:

- The UI runs inside an MCP client host (Claude Desktop), not a browser served by a dev server. There is no HMR, no dynamic module loading, no CDN. Everything must be inline.
- A component framework adds build-time dependencies and runtime overhead for a single-page, single-concern view.
- The UI's state machine is simple: load hotels, filter, show detail. This is well within what vanilla TypeScript with DOM manipulation handles without abstraction overhead.

### 5.2 Vite single-file plugin

The build target is `vite-plugin-singlefile`, which inlines all JS and CSS into a single `index.html`. This output is copied to `internal/resources/mcp-app.html` and embedded into the Go binary at compile time via `//go:embed mcp-app.html`. The server binary contains the complete UI with no external file dependencies.

The Makefile `build-ui` target runs Vite, then `build-go` links the result. The `dev-http` target starts the Go server in HTTP mode for local development without needing a running MCP client.

### 5.3 Brand theming

Gradient CSS classes for hotel cards are generated in Go (`brandImageStyle` in `repository.go`) and returned as part of each `HotelRow.ImageStyle`. The UI renders them as Tailwind CSS `bg-gradient-to-br` classes. This keeps brand colour management in one place (Go) rather than duplicating the map in TypeScript.

### 5.4 Image handling

Thumbnails are returned as paths from `pool.GetThumbnails`. `resizeImageURL` rewrites these paths to the `images.archipelagohotels.com` CDN proxy base URL before the data reaches the UI. The UI only sets `src` attributes on `<img>` tags; it never makes fetch/XHR calls to retrieve images. The MCP App CSP allows `images.archipelagohotels.com` via `resourceDomains`, so the browser can load thumbnails without a CSP violation.

### 5.5 Currency formatting

Currency is returned as the raw ISO code from `hotel_currency` (e.g. `IDR`, `USD`). The UI function `fmtPrice(value, currency)` calls `Number.toLocaleString('id-ID', { style: 'currency', currency })` for IDR and a standard format for other currencies. There is no hardcoded symbol map — `Intl.NumberFormat` handles new currencies without any code change.

---

## 6. Error Handling Design

### 6.1 Tool handlers

Each handler follows the same pattern:

1. `defer recover()` converts panics to structured errors.
2. Input validation (e.g. numeric hotel ID parsing) returns an explicit error before any DB call.
3. Errors from the central DB are returned as tool errors — the tool cannot function without catalog data.
4. Errors from brand DBs are logged and treated as empty results — the tool degrades gracefully.
5. Rate errors are always swallowed — pricing unavailability is not a failure condition for the catalog tools.

### 6.2 Database layer

`Pool.BrandDB` returns `nil` on failure, never an error. The error is logged at connect time; callers do not re-handle the same error on every invocation. The failure mode is "no room data, no live rates" — not "tool error". `Pool.Health` is the only place that surfaces brand DB errors explicitly, for the `/health` endpoint.

### 6.3 Rate service

`rate.Service.GetRates` has the signature `([]RoomRate, error)` but in practice never returns a non-nil error for recoverable failures — it logs and returns `(nil, nil)`. The error return is reserved for hard failures such as a cancelled context. This simplifies all callers: they check `len(rates) == 0` rather than branching on errors.

The circuit breaker's `Failure()` is called on both HTTP transport errors and XML parse errors. `Success()` resets the failure counter on a clean response, even if the response contains zero available rooms (empty availability is data, not an API error).

---

## 7. HTTP Mode Design

When started with `-http`, the server registers both the MCP Streamable HTTP handler at `/mcp` and a set of REST API endpoints for the standalone dashboard. The REST endpoints (`/api/hotels`, `/api/brands`, `/api/regions`) exist so the dashboard HTML can fetch data when accessed directly in a browser (outside a Claude Desktop context), without requiring an MCP client.

Gin provides middleware chaining (CORS, recovery, logger) cleanly. The CORS middleware is permissive (`*`) because the MCP client may be on any origin.

Input lengths for `city` and `brand` query parameters are capped at 100 characters before use in DB queries, preventing oversized inputs from reaching the SQL layer.

---

## 8. Future Design Considerations

### 8.1 Sentec REST API (Phase 3, reserved)

`internal/rate/sentec.go` exists but the code path is dead — zero hotels currently have `hotel_channel = 'SENTEC'` in any brand DB. When Sentec-channel hotels are provisioned:

- `rate.Service.trySB` will need a parallel `trySentec` branch checking `BrandCredentials.HotelChannel`.
- The Sentec client will authenticate via `SentecBookingID` against `https://api.booking.sentec.io/sm/api/availability/search`.
- The fallback chain becomes: Sentec → SimpleBooking → stored → starting price, with `hotel_channel` driving the first choice.

### 8.2 Profile caching

Hotel profiles from `db_archipelagowebsite` are fetched on every tool call. For high-traffic deployments, an in-memory LRU cache with a 5-minute TTL would reduce central DB load substantially. The natural insertion point is `Pool.SearchHotels` and `Pool.GetHotelByID`. The brand metadata (`brands` map) is already cached at startup; extending this pattern to hotel rows is straightforward.

### 8.3 Check-in/check-out date parameters

The rate service defaults to today/tomorrow for all availability queries. Adding explicit date parameters requires: (a) exposing `checkIn`/`checkOut` in tool input schemas, (b) including the date pair in the rate cache key, and (c) communicating the booking window in tool descriptions.

### 8.4 Multi-country expansion

All hotels currently hardcode `country = "Indonesia"`. The central DB has country and province data in `tb_region`. Expanding to non-Indonesia Archipelago properties requires removing the hardcoded country default and passing country through the full data path from `SearchHotels` to the tool response.

### 8.5 Schema-drift monitoring

Brand DB schemas change without coordination notice. `Pool.scanColumns` logs discovered columns at `INFO` level. A future improvement is a startup validation step that compares the discovered column set against a known-good baseline and logs a `WARN` for unexpected drift, without failing the startup.

---

## 9. Key Invariants

These invariants must hold across all code changes:

1. `Pool.BrandDB` never panics; it returns `nil` for any missing or unreachable brand DB.
2. Tool handlers never return an error solely because rates are unavailable.
3. All SQL queries are read-only `SELECT` statements.
4. The central DB (`db_archipelagowebsite`) is the only hard dependency; the server fails to start if it is unreachable.
5. `//go:embed` embeds the compiled UI — `make build-ui` must run before `make build-go` to avoid a stale or missing embed target.
6. The rate cache key does not include date parameters; today/tomorrow availability is always implied.
7. `hotel_currency` is passed through raw; the server never converts or normalises currency codes.

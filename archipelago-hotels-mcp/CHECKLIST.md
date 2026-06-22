# Archipelago Hotels MCP — Implementation Checklist

> **Status**: ▢ Not Started | ◐ In Progress | ✅ Complete | ❌ Blocked

---

## Phase 0: Foundation — Replace Hardcoded Data

### 0.1 Dependency Setup
- [x] `go get github.com/go-sql-driver/mysql`
- [x] MySQL DSN env vars in Makefile
- [x] `.env.example` for local development (env vars documented)

### 0.2 DB Connection Layer — `internal/repository/`
- [x] `repository.go` — connection pool manager
  - [x] Central `db_archipelagowebsite` connection
  - [x] 8 brand DB connection pools (lazy-init)
  - [x] `INFORMATION_SCHEMA.COLUMNS` introspection for safe column access
  - [x] Health check function
  - [x] Graceful shutdown (close pools on SIGTERM)
  - [x] Brand DB name mapping (`favehotel`→`db_favewebsite`, `pba`→`db_pba`)
  - [x] Negative caching for unreachable brand DBs
- [x] `hotel.go` — hotel profile queries
  - [x] `SearchHotels(params)` — SELECT with optional filters (city, brand, query)
  - [x] `GetHotelByID(hotelID)` — single hotel by central DB ID
  - [x] `GetHotelByAPIID(apiHotelID, brandDB)` — per-brand lookup
  - [x] `ListRegions()` — for dashboard filter options
- [x] `room.go` — room type + credentials queries
  - [x] `GetRooms(apiHotelID, brandDB)` — query `tb_hrooms` or `tb_hroom` depending on brand
  - [x] Column introspection for safe `hotel_id`/`old_id` selection
  - [x] `GetCredentials(apiHotelID, brandDB)` — SimpleBooking credentials
  - [x] PBA: separate code path (`tb_hroom` not `tb_hrooms`)

### 0.3 Delete Old Packages
- [x] `internal/db/` package deleted (replaced by `internal/repository/`)
- [x] `internal/tools/data.go` deleted (597 lines of hardcoded data)
- [x] Old tool types removed from `tools.go`

### 0.4 Server Lifecycle — `internal/server/server.go`
- [x] DB pool created at startup, passed to tools
- [x] Graceful shutdown (signal handler → pool.Close() → server close)
- [x] Health endpoint checks DB connectivity
- [x] `RunStdio` and `RunHTTP` pass pool + rateSvc to tools

---

## Phase 1: Per-Brand Room Queries

### 1.1 Brand DB Router
- [x] Brand → DB prefix map from `tb_brands.db_prefix_name`
- [x] `parent_brand_id != 0` → use parent's DB (Grand Aston → aston)
- [x] No matching DB → skip live rate (nil-safe)
- [x] PBA: separate code path (`tb_hroom` not `tb_hrooms`)
- [x] Brand DB name mismatch: `favehotel`→`db_favewebsite`, `pba`→`db_pba`

### 1.2 Room Data Mapping
- [x] Unify room schema differences via column introspection
  - [x] `tb_hrooms`: `room_name, room_rate (DECIMAL), sb_id (INT), room_status (ENUM)`
  - [x] `tb_hroom` (PBA): `room_name, room_rate (INT), sb_id (INT), status (TINYINT)`
- [x] Map to common `RoomRate` struct in `internal/rate/`

### 1.3 Room Query Strategy
- [x] Primary: query brand DB for room types
- [x] Fallback (no brand DB): return empty room list, use `hotel_starting_price`

---

## Phase 2: SimpleBooking API Integration

### 2.1 SimpleBooking Client — `internal/rate/simplebooking.go`
- [x] `OTA_HotelAvailRQ` XML builder per SimpleBooking spec
- [x] `OTA_HotelAvailRS` XML response parser (namespace-stripped)
- [x] Extract `<Total AmountAfterTax>` — lowest rate across all room stays
- [x] Error handling:
  - [x] XML parse errors
  - [x] Missing `<RoomStays>` element
  - [x] Network timeouts (15s timeout)
  - [x] Authentication errors (bad SB credentials)

### 2.2 Circuit Breaker
- [x] 5 errors in 60s → skip SB for 120s
- [x] Log all SB errors with hotel_id + reason

### 2.3 Rate Orchestrator — `internal/rate/rate.go`
- [x] `GetRates()` — single hotel rate with cache check
- [x] `BatchMinRates()` — parallel rate fetching for multiple hotels
  - [x] Bounded goroutine pool (max 5 concurrent via channel semaphore)
  - [x] Context propagation (no leaked goroutines)
  - [x] Returns `map[hotelID]minRate` with StartingPrice fallback
- [x] Cache first (TTL 5 min, lazy expiry)

### 2.4 Rate Fallback Chain
```
✅ 1. SimpleBooking live rate → success → return {price, source: "live"}
✅ 2. SimpleBooking fails → use tb_hrooms.room_rate → return {price, source: "stored"}
✅ 3. No room rate → use hotel_starting_price → return {price, source: "starting_price"}
✅ 4. Nothing available → return {price: 0, source: "unavailable"}
```

---

## Phase 3: Sentec API Integration (Reserve)

- [x] `internal/rate/sentec.go` created (stub)
- [ ] REST client for `POST /sm/api/availability/search` (future)
- [ ] JSON request/response parsing (future)
- [ ] Note: Zero hotels currently use Sentec → deferred

---

## Phase 4: Update MCP Tools

### 4.1 `search_hotels` — `internal/tools/search.go`
- [x] Uses `repository.SearchHotels()` for DB queries
- [x] Batch-min-rate via `rateSvc.BatchMinRates()` for all matched hotels
- [x] Rates merged into `HotelSummary`; `StartingPrice` used when rates unavailable

### 4.2 `get_hotel_detail` — `internal/tools/detail.go`
- [x] Queries central DB for hotel profile
- [x] Queries brand DB for room types via `GetRooms()`
- [x] Rate data per room (stored `room_rate` or live SB)
- [x] Rate min for starting price

### 4.3 `recommend_hotel` — `internal/tools/recommend.go`
- [x] Recommendation algorithm unchanged (vibe/budget/purpose scoring)
- [x] Data source: DB + `BatchMinRates()` for live prices
- [x] Budget matching uses actual prices, not hardcoded

### 4.4 `hotel_dashboard` — `internal/tools/dashboard.go`
- [x] Dashboard HTML (static, `//go:embed`)
- [x] Dashboard API (`/api/hotels`) reads from DB
- [x] `hotel_dashboard` tool returns data + message count

### 4.5 Remove dead code
- [x] `FilteredHotels()` — removed with `data.go`
- [x] Old hardcoded rate calculation — replaced by `rate.Service`

---

## Phase 5: Caching & Performance

### 5.1 In-Memory Cache — `internal/rate/cache.go`
- [x] Thread-safe TTL cache via `sync.RWMutex`
- [x] Key: `{dbPrefix}:{apiHotelID}`
- [x] TTL: 5 minutes (configurable)
- [x] Lazy expiry on read — no background goroutines/tickers
- [x] Nil entries cached (negative caching for failed lookups)

### 5.2 Concurrent Query Optimisation
- [x] Goroutine pool (max 5 via channel semaphore) for parallel rate fetching
- [x] `context.Context` propagated to all goroutines
- [x] Bounded DB connections per brand: `MaxOpenConns=3`
- [x] ~2500ms cold → **~10ms cached** (250× improvement)

### 5.3 Cache Invalidation
- [x] TTL-based expiry only (all data is read-only)
- [x] No background GC — lazy expiry on read

---

## Phase 6: Error Handling & Observability

### 6.1 Structured Logging
- [x] `slog` with text handler writing to stderr
- [x] `DEBUG=1` env var toggles `slog.Debug` level
- [x] Log levels: ERROR (DB down), WARN (connection fail), INFO (connections), DEBUG (unreachable)

### 6.2 Tool Handler Resilience
- [ ] `recover()` wrapper for panics → MCP error response
- [x] Partial results: brand DB fail returns profile without rates
- [x] Each tool has context timeout from HTTP handler

### 6.3 Monitoring Endpoints
- [x] `/health` → status + DB connectivity + version
- [ ] `/debug/cache` → cache keys + TTL (verbose mode only)

### 6.4 Graceful Degradation Matrix

| Failure | search_hotels | get_hotel_detail | recommend_hotel | hotel_dashboard |
|---------|:------------:|:----------------:|:---------------:|:---------------:|
| Central DB down | ❌ Error | ❌ Error | ❌ Error | ❌ Error |
| Brand DB down | ✅ Partial (no rates) | ✅ Partial (no rooms) | ✅ Partial (no rates) | ✅ Partial |
| SimpleBooking down | ✅ Fallback rates | ✅ Fallback rates | ✅ Fallback rates | ✅ N/A |
| Single hotel SB fails | ✅ Skip that hotel | ✅ Fallback rate | ✅ Skip that hotel | ✅ N/A |

---

## Integration Testing

### MCP Protocol Tests
- [x] `tools/list` returns 4 tools
- [x] `tools/call search_hotels` without params returns all hotels
- [x] `tools/call search_hotels` with city="Jakarta" filters correctly (20 hotels)
- [x] `tools/call search_hotels` with city="Bali" → 15 hotels
- [x] `tools/call get_hotel_detail` returns rooms with rates (4 room types)
- [x] `tools/call recommend_hotel` returns recommendations with prices
- [x] `tools/call hotel_dashboard` returns 200 hotels with message

### Database Tests
- [x] Central DB connection works
- [x] All brand DBs connect successfully (8 prefixes)
- [x] Column introspection works on each DB
- [x] `api_hotel_id → hotel_id` mapping correct
- [x] No N+1 query pattern (batch rate fetching)

### Rate API Tests
- [x] SimpleBooking returns valid XML (hotel 6750 verified)
- [x] Lowest rate extraction works
- [x] Invalid credentials → graceful nil
- [x] Timeout → graceful fallback (15s timeout)
- [x] Rate fallback chain works: SB → stored → starting_price

### Edge Cases
- [x] Hotel with `room_rate = 0` excluded from min calculation
- [x] Brand with no per-brand DB uses `hotel_starting_price`
- [x] City with zero matches returns empty results (not error)
- [x] Currencies shown correctly (IDR)
- [x] PBA hotels: separate code path (`tb_hroom`, UUID PK)

---

## Go CLI Flags

### `http` subcommand
```
-addr string    HTTP listen address (default ":9011")
-verbose        Enable verbose logging and Gin debug mode
```

### `stdio` subcommand
*(no flags, runs on stdin/stdout)*

### Global env vars
```
MYSQL_HOST       MySQL host (default "127.0.0.1")
MYSQL_PORT       MySQL port (default "3306")
MYSQL_USER       MySQL user (default "root")
MYSQL_PASS       MySQL password
MYSQL_DB         Central database (default "db_archipelagowebsite")
```

---

## Final Verification

- [x] Binary builds cleanly (`go build ./...`)
- [x] `go vet` passes with no issues
- [x] Stdio transport: no stdout noise (`init()` fix)
- [x] HTTP transport: verified health + MCP endpoints
- [x] No concurrent map writes (`scanColumns` race fixed)
- [x] All 4 tools return correct data (verified via test script)
- [x] Cache makes second requests 250× faster
- [ ] Claude Desktop config points to correct binary → restart required
- [ ] Pi Agent config matches → restart required

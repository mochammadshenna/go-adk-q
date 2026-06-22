# Archipelago Hotels MCP — Implementation Plan

> **Status**: Planning Phase
> **Architect**: Senior Principal Backend & AI Infra
> **Target**: Production-grade MCP server querying 9 MySQL databases + 2 live booking APIs

---

## §1 System Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                        Claude Desktop / Pi Agent                    │
└──────────────────────────┬──────────────────────────────────────────┘
                           │ MCP (stdio/HTTP)
                           ▼
┌─────────────────────────────────────────────────────────────────────┐
│                  archipelago-hotels-mcp (Go binary)                 │
│                                                                      │
│  ┌─────────────────────────────────────────────────────────────────┐ │
│  │  MCP Layer (mcp.NewServer + tools)                              │ │
│  │  search_hotels | get_hotel_detail | recommend_hotel             │ │
│  └──────────┬──────────────────────────────────────────────────────┘ │
│             │                                                       │
│  ┌──────────▼──────────────────────────────────────────────────────┐ │
│  │  Service Layer                                                  │ │
│  │                                                                  │ │
│  │  ┌────────────────────┐  ┌──────────────────────┐               │ │
│  │  │  ProfileService    │  │  RateService          │               │ │
│  │  │  (hotel catalog)   │  │  (live pricing)       │               │ │
│  │  └────────┬───────────┘  └──────────┬────────────┘               │ │
│  └───────────┼─────────────────────────┼────────────────────────────┘ │
│              │                         │                             │
│  ┌───────────▼──────────┐  ┌──────────▼──────────────────────────┐  │
│  │  MySQL Queries        │  │  SimpleBooking XML API             │  │
│  │                       │  │  (xml.simplebooking.it)            │  │
│  │  • db_archipelagowebsite (profile)  │  OTA_HotelAvailRQ →        │  │
│  │  • db_astonwebsite    │  │  parse <Total AmountAfterTax>      │  │
│  │  • db_neowebsite      │  └────────────────────────────────────┘  │
│  │  • db_favewebsite     │                                          │
│  │  • db_alanawebsite    │  ┌────────────────────────────────────┐  │
│  │  • db_harperwebsite   │  │  Sentec REST API (reserve)         │  │
│  │  • db_kamuelawebsite  │  │  POST availability/search          │  │
│  │  • db_questwebsite    │  │  → parse final_rate                │  │
│  │  • db_pba             │  └────────────────────────────────────┘  │
│  └───────────────────────┘                                          │
└─────────────────────────────────────────────────────────────────────┘
```

### Two core data paths:

**Path A: Hotel Search (Profile)**
```
SELECT from db_archipelagowebsite.tb_hotels
  JOIN tb_brands → brand_name, db_prefix_name
  JOIN tb_region → region_name (= city)
  WHERE region LIKE '%Jakarta%'
  → {hotel_id, api_hotel_id, hotel_name, brand, region, rating, lat, lng}
```

**Path B: Live Rates (Pricing)**
```
1. Map brand → per-brand DB via db_prefix_name
2. Query db_{brand}.tb_hotels WHERE hotel_id = api_hotel_id
   → {simplebooking_id, channel, sentec_booking_id}
3. IF channel = 'SENTEC' → Sentec REST API → final_rate
   IF channel = 'SB'/'null' → SimpleBooking XML API → AmountAfterTax
4. Return {before_discount, after_discount} per hotel
```

---

## §2 Database Topology

### 2.1 Central Catalog: `db_archipelagowebsite`

| Table | Purpose | Key Columns |
|-------|---------|-------------|
| `tb_hotels` (279 active) | Master hotel list | `hotel_id, api_hotel_id, brand_id, region_id, hotel_name, hotel_starting_price, hotel_currency, latitude, longtitude, hotel_status` |
| `tb_brands` (19) | Brand definitions | `brand_id, brand_name, db_prefix_name, parent_brand_id` |
| `tb_region` (68) | Geographic regions | `region_id, region_name, country_id, province_id` |
| `tb_hotel_rates` (1,113) | Cached rate history | `rate, before_discount, date` |

### 2.2 Per-Brand Databases

| DB | Brand(s) | Active Hotels | Rooms | SimpleBooking IDs |
|----|----------|:------------:|:-----:|:-----------------:|
| `db_astonwebsite` | Aston, Grand Aston, Aston Collection | 84 | 435 | 108 |
| `db_neowebsite` | Hotel Neo | 22 | 79 | 30 |
| `db_favewebsite` | favehotels | 53 | 190 | 70 |
| `db_alanawebsite` | The Alana, The Royal Alana | 11 | 31 | 11 |
| `db_harperwebsite` | Harper | 16 | 52 | 15 |
| `db_kamuelawebsite` | Kamuela Villas | 3 | 13 | 4 |
| `db_questwebsite` | Quest Hotels | 9 | 47 | 11 |
| `db_pba` | Powered By Archi (3rd party) | 158 | 238 | N/A (tb_hroom UUID) |

### 2.3 Primary Key Mapping

```
db_archipelagowebsite.tb_hotels.api_hotel_id
    ↓
db_[brand]website.tb_hotels.hotel_id  (INTEGER)
    OR
db_pba.tb_hotels.hotel_id             (INTEGER, different IDs)
```

---

## §3 Risks & Mitigations

### 🔴 Risk 1: Column Naming Inconsistency (HIGH)

| Brand DB | `hotel_channel` | `sentec_booking_id` | `simplebooking_id` |
|----------|:--------------:|:------------------:|:------------------:|
| astonwebsite | ✅ | ✅ (as `sentec_booking_id`, `_idsentec_booking`, `hotel_sentec_booking`) | ✅ |
| alanawebsite | ✅ | ✅ (as `sentec_booking_id`, `hotel_sentec_booking`) | ✅ |
| neowebsite | ✅ | ❌ Missing | ✅ |
| favewebsite | ✅ | ❌ Missing | ✅ |
| harperwebsite | ✅ | ❌ Missing | ✅ |
| kamuelawebsite | ✅ | ❌ Missing | ✅ |
| questwebsite | ✅ | ❌ Missing | ✅ |
| pba | ❌ (uses `hotel_simplebooking`) | ❌ | ✅ (as `simplebooking_id`) |

**Mitigation**: Introspect each DB's `INFORMATION_SCHEMA.COLUMNS` on startup to build a column map. Use safe defaults — if column doesn't exist, treat as nil.

### 🔴 Risk 2: Brand-to-DB Mapping Gaps (HIGH)

Brands with no per-brand DB: **Huxley, Four Corners, NORDIC, Avanika, Powered By Archi**

Per-brand DBs exist for: Aston, Neo, Favehotel, Alana, Harper, Kamuela, Quest

**Mitigation**:
- Brands without per-brand DB → fallback to `hotel_starting_price` from `db_archipelagowebsite`
- PBA uses different schema (`tb_hroom` with UUIDs, no channel) → separate query path
- Log warning on startup for unmapped brands

### 🔴 Risk 3: SimpleBooking XML API Reliability (MEDIUM)

- SOAP/XML endpoint, not REST
- Requires valid SB credentials stored per-hotel in brand DB
- Returns different XML structure on error vs success
- Network timeout possible

**Mitigation**: 
- Timeout: 10s connect, 15s request
- Retry: 1 retry on transient errors (timeout, 5xx)
- Circuit breaker: 5 failures in 60s → skip SB for 120s
- XML response validation before parsing
- Fallback: `room_rate` from `tb_hrooms` as base price if SB fails

### 🔴 Risk 4: Room Data Schema Inconsistency (MEDIUM)

- Brand DBs use `tb_hrooms` with `sb_id` (INT)
- PBA uses `tb_hroom` with `sb_id` (INT) but UUID primary key
- Aston queries by `hotel_id`, other brands by `old_id` (from PHP model analysis)

**Mitigation**: Always query per-brand DB using `hotel_id = api_hotel_id`. For PBA, note that `hotel_id` is UUID string (CHAR(36)), not integer.

### 🟡 Risk 5: Rate Staleness (MEDIUM)

- `hotel_rate_expired` in archipelagowebsite shows dates like 2023
- `room_rate` in `tb_hrooms` is stored price, may be weeks/months old
- Only SimpleBooking API gives truly live rates

**Mitigation**: Always attempt SimpleBooking first. If SB fails or returns null, use `room_rate` from `tb_hrooms` as fallback. Annotate response with `rate_source: "live" | "fallback"`.

### 🟡 Risk 6: Connection Pool Management (MEDIUM)

- 7+ MySQL database connections simultaneously
- Each connection needs pool sizing
- Wrong pool settings → connection storms or deadlocks

**Mitigation**: Use `database/sql` with `sql.DB.SetMaxOpenConns(5)` per DB. Single shared `*sql.DB` per brand database, created at startup. Never open/close per-request.

### 🟢 Risk 7: Rate Limiting (LOW)

- SimpleBooking XML API may have rate limits
- Sentec API may throttle

**Mitigation**: Cache SB/Sentec results for 300s (5 min). Use per-hotel TTL so distributed requests don't all hit the same hotel simultaneously.

### 🟢 Risk 8: Currency Conversion (LOW)

- All rates currently IDR
- Future: some PBA hotels use USD, VND, PHP

**Mitigation**: Return rates in hotel's native currency + include `currency` field. Do not convert — let the client handle FX.

---

## §4 Implementation Phases

### Phase 0: Foundation — REPLACE hardcoded data.go

**Goal**: Remove all 39 fake hotels, replace with real MySQL queries

Files to modify:
- `internal/server/server.go` — add DB connection initialization
- `internal/tools/data.go` — DELETE 597 lines of hardcoded data, replace with `HotelsData()` function that queries MySQL
- `go.mod` — add `github.com/go-sql-driver/mysql` dependency

Key decisions:
- Read-only queries only (SELECT)
- Connection string from environment variable `MYSQL_ARCHIPELAGO_DSN`
- Fail fast on startup if DB unreachable
- All brand DBs on same host (verified: all use `$host` in config.php)

### Phase 1: Per-Brand Room Queries

**Goal**: Fetch room types + SimpleBooking credentials from each brand DB

New files:
- `internal/db/db.go` — `BrandDB` pool manager, lazy-init per brand
- `internal/db/hotel.go` — hotel profile queries
- `internal/db/room.go` — room type queries

Key logic:
- Brand DB map: `db_prefix_name → sql.DB`
- Column introspection for safety
- PBA special case

### Phase 2: SimpleBooking API Integration

**Goal**: Live rate fetching via SimpleBooking XML API

New files:
- `internal/rate/simplebooking.go` — SB XML request builder + response parser
- `internal/rate/rate.go` — rate fetching orchestrator (SB first, fallback to cached)

Key logic:
- Build `OTA_HotelAvailRQ` XML payload
- Parse `<Total AmountAfterTax>` from response
- Find lowest rate across room types
- Circuit breaker for failures

### Phase 3: Sentec API Integration (Reserve)

**Goal**: Sentec REST API for hotels with `hotel_channel = 'SENTEC'`

New files:
- `internal/rate/sentec.go` — Sentec REST client

Note: Currently ZERO hotels use Sentec channel. This is future-proofing. The API is implemented but the code path is dead unless a hotel has `hotel_channel = 'SENTEC'`.

### Phase 4: Update MCP Tools

**Goal**: Wire real DB + rate data into MCP tool handlers

Files to modify:
- `internal/tools/search.go` — query MySQL instead of filtering hardcoded list
- `internal/tools/detail.go` — query MySQL + brand DB for rooms
- `internal/tools/recommend.go` — same as search + rate data

### Phase 5: Caching & Performance

**Goal**: Sub-second response times

Add:
- In-memory LRU cache for hotel profiles (TTL: 300s)
- Rate cache keyed by `hotel_id` (TTL: 300s, invalidatable)
- Profile data caching: startup warm-up

### Phase 6: Error Handling & Observability

**Goal**: Graceful degradation, meaningful errors

Add:
- Structured logging (slog) with request IDs
- Each tool handler wraps in error recovery
- Missing data → partial results with warnings
- Health check endpoint reflects DB connectivity

---

## §5 Tool Handler Behaviour Matrix

| Tool | Action | DB Query | Rate API Call | Response |
|------|--------|----------|--------------|----------|
| `search_hotels` | List hotels by city/brand | `db_archipelagowebsite.tb_hotels` | Batch call for rates | Hotels + lowest price |
| `get_hotel_detail` | Full hotel detail + rooms | Both central + brand DB | Single hotel rate | Hotel + room types + prices |
| `recommend_hotel` | AI recommendations | Same as search | Batch call for rates | Ranked hotels + prices |
| `hotel_dashboard` | Interactive HTML dashboard | Same as search | None (uses static fallback) | HTML with city/brand filters |

---

## §6 Failure Mode Strategy

```
Rate Unavailable
├── SimpleBooking timeout → use tb_hrooms.room_rate as fallback
├── SimpleBooking error → log error, return null rate
├── No SimpleBooking ID → return null rate (hotel can't book)
├── Brand DB unreachable → return profile without rates
└── Central DB unreachable → MCP tool returns error (cannot function)
```

---

## §7 Configuration

```
# Environment variables for the Go MCP server
MYSQL_HOST=127.0.0.1
MYSQL_PORT=3306
MYSQL_USER=root
MYSQL_PASS=
MYSQL_DB_ARCHIPELAGO=db_archipelagowebsite

# SimpleBooking API
SB_API_URL=https://xml.simplebooking.it/xmlservice.asmx/HotelAvailRQ

# Sentec API (reserve)
SENTEC_API_URL=https://api.booking.sentec.io/sm/api/availability/search

# Cache TTLs (seconds)
CACHE_PROFILE_TTL=300
CACHE_RATE_TTL=300
CACHE_STALE_TTL=3600
```

---

## §8 Branching Strategy

```
main ← stable, deployable
├── feature/db-integration      ← Phase 0-1 (DB queries)
├── feature/rate-api            ← Phase 2 (SimpleBooking)
├── feature/sentec-api          ← Phase 3 (Sentec reserve)
├── feature/tool-refactor       ← Phase 4 (wire tools)
├── feature/caching             ← Phase 5
└── feature/observability       ← Phase 6
```

Each phase must pass `go build ./...` and `go vet ./...` before merging forward.

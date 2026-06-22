# Reference

> **Information-oriented.** Precise, complete technical specifications.
> For background on *why* things work this way, see [Explanation](../explanation/README.md).

---

## MCP Tools {#tools}

### `search_hotels`

Search the hotel catalog by city, brand, or free text.

**Input schema:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `query` | string | yes | City name, region, brand name, or free-text |
| `brand` | string | no | Filter by brand slug (e.g. `aston`, `neo`) |
| `max_price` | number | no | Upper bound on nightly rate (hotel's native currency) |
| `limit` | integer | no | Max results to return (default 10, max 50) |

**Output:** JSON array of hotel objects (see [HotelRow](#hotelrow)).

---

### `recommend_hotel`

Rank hotels against a traveller's stated preferences using scoring heuristics.

**Input schema:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `query` | string | yes | Natural-language preference ("beachfront under USD 100") |
| `city` | string | no | Restrict to a city or region |
| `brand` | string | no | Restrict to a brand |
| `limit` | integer | no | Max results (default 5) |

**Output:** JSON array of hotel objects with an added `score` field.

---

### `find_hotels`

Browse all hotels with optional filters. Entry point for the ext-app UI.

**Input schema:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `city` | string | no | Filter by city/region name |
| `brand` | string | no | Filter by brand slug |
| `page` | integer | no | Page number, 1-based (default 1) |
| `per_page` | integer | no | Page size (default 20, max 100) |

**Output:** JSON object `{ hotels: HotelRow[], total: int, page: int }`.

---

### `get_hotel_detail`

Full hotel detail including room types and live rates.

> **Note:** `visibility: app` — registered for MCP App resource calls only;
> not surfaced to the LLM directly.

**Input schema:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `hotel_id` | integer | yes | `hotel_id` from the central catalog |

**Output:** JSON object combining [HotelRow](#hotelrow) with a `rooms` array of
[RoomRow](#roomrow) objects.

---

## Data types {#data-types}

### HotelRow {#hotelrow}

```jsonc
{
  "hotel_id": 42,
  "api_hotel_id": 1337,
  "hotel_name": "Aston Priority Simatupang",
  "brand_name": "Aston",
  "db_prefix": "aston",
  "region_name": "Jakarta",
  "country": "Indonesia",
  "star_rating": 4,
  "latitude": -6.2927,
  "longitude": 106.7897,
  "thumbnail_url": "https://images.archipelagohotels.com/...",
  "min_rate": 850000,
  "currency": "IDR",
  "rate_source": "live"   // "live" | "stored" | "starting_price"
}
```

### RoomRow {#roomrow}

```jsonc
{
  "room_id": 5,
  "room_name": "Superior Room",
  "room_type": "Superior",
  "bed_type": "King",
  "max_occupancy": 2,
  "room_rate": 850000,
  "currency": "IDR",
  "simplebooking_id": 91234,
  "sb_rate": 920000,      // null if SB unavailable
  "rate_source": "live"
}
```

---

## Environment variables {#env-vars}

### Database

| Variable | Default | Description |
|----------|---------|-------------|
| `MYSQL_HOST` | `127.0.0.1` | MySQL hostname |
| `MYSQL_PORT` | `3306` | MySQL port |
| `MYSQL_USER` | `root` | MySQL username |
| `MYSQL_PASS` | *(empty)* | MySQL password |
| `MYSQL_DB` | `db_archipelagowebsite` | Central catalog database |
| `MYSQL_MAX_OPEN` | `5` | Max open connections per DB |
| `MYSQL_MAX_IDLE` | `5` | Max idle connections per DB |

### Rate APIs

| Variable | Default | Description |
|----------|---------|-------------|
| `SB_API_URL` | `https://xml.simplebooking.it/xmlservice.asmx/HotelAvailRQ` | SimpleBooking endpoint |
| `SB_DISABLED` | `0` | Set to `1` to skip live rates entirely |
| `SB_REQUEST_TIMEOUT` | `15` | Per-request timeout in seconds |
| `SB_CB_THRESHOLD` | `5` | Circuit breaker failure threshold |
| `SB_CB_WINDOW` | `60` | Circuit breaker observation window (s) |
| `SB_CB_TIMEOUT` | `120` | Circuit breaker open duration (s) |
| `SENTEC_API_URL` | `https://api.booking.sentec.io/sm/api/availability/search` | Sentec REST endpoint |

### Cache

| Variable | Default | Description |
|----------|---------|-------------|
| `CACHE_PROFILE_TTL` | `300` | Hotel profile cache TTL (seconds) |
| `CACHE_RATE_TTL` | `300` | Rate cache TTL (seconds) |

### Server

| Variable | Default | Description |
|----------|---------|-------------|
| `HTTP_PORT` | `9011` | HTTP listener port (HTTP mode only) |
| `DEBUG` | `0` | Verbose debug logging when `1` |
| `url_image_resizer` | `https://images.archipelagohotels.com/` | Image CDN proxy base URL |

---

## HTTP endpoints {#http-endpoints}

Available only when running with `--http` flag.

| Method | Path | Query params | Response |
|--------|------|-------------|---------|
| `POST`/`GET` | `/mcp` | — | MCP Streamable HTTP (JSON-RPC) |
| `GET` | `/api/hotels` | `city`, `brand`, `page`, `per_page` | `{ hotels, total, page }` |
| `GET` | `/api/brands` | — | `[ { brand_id, brand_name, db_prefix_name } ]` |
| `GET` | `/api/regions` | — | `[ { region_id, region_name, country_id } ]` |
| `GET` | `/dashboard` | — | Standalone HTML dashboard |
| `GET` | `/health` | — | `{ status, version, db, rate_circuit_breaker }` |

---

## Database schema reference {#schema}

### `db_archipelagowebsite.tb_hotels` (central catalog)

Key columns used by this server:

| Column | Type | Notes |
|--------|------|-------|
| `hotel_id` | INT PK | Internal catalog ID |
| `api_hotel_id` | INT | Foreign key into brand DB `tb_hotels.hotel_id` |
| `brand_id` | INT FK | → `tb_brands.brand_id` |
| `region_id` | INT FK | → `tb_region.region_id` |
| `hotel_name` | VARCHAR | Display name |
| `hotel_starting_price` | DECIMAL | Fallback price if no live rate |
| `hotel_currency` | VARCHAR(10) | ISO 4217 currency code |
| `latitude` | DECIMAL(10,7) | |
| `longtitude` | DECIMAL(10,7) | Note: legacy typo in schema |
| `hotel_status` | TINYINT | 1 = active |

### `db_archipelagowebsite.tb_brands`

| Column | Type | Notes |
|--------|------|-------|
| `brand_id` | INT PK | |
| `brand_name` | VARCHAR | Display name |
| `db_prefix_name` | VARCHAR | Used to construct brand DB name: `db_{prefix}website` |
| `parent_brand_id` | INT | NULL for top-level brands |

### Per-brand `tb_hotels`

Schema varies by brand. Columns detected via `INFORMATION_SCHEMA.COLUMNS`:

| Column | Present in | Notes |
|--------|-----------|-------|
| `hotel_id` | all | Matches `api_hotel_id` in central DB |
| `hotel_channel` | all except PBA | `'SENTEC'` or `'SB'`/NULL |
| `simplebooking_id` | all | Integer SB property ID |
| `sentec_booking_id` | aston, alana | May also appear as `hotel_sentec_booking` |
| `hotel_simplebooking` | PBA only | PBA's alternate column name for SB ID |

### Per-brand `tb_hrooms` / `tb_hroom` (PBA)

| Column | Type | Notes |
|--------|------|-------|
| `room_id` | INT / UUID (PBA) | PK |
| `hotel_id` | INT | FK to `tb_hotels.hotel_id` |
| `room_name` | VARCHAR | |
| `room_rate` | DECIMAL | Stored fallback rate |
| `sb_id` | INT | SimpleBooking room type ID |

---

## SimpleBooking XML format {#simplebooking-xml}

### Request (`OTA_HotelAvailRQ`)

```xml
<OTA_HotelAvailRQ>
  <AvailRequestSegments>
    <AvailRequestSegment>
      <HotelSearchCriteria>
        <Criterion>
          <HotelRef HotelCode="91234"/>
        </Criterion>
      </HotelSearchCriteria>
      <StayDateRange Start="2026-07-01" End="2026-07-02"/>
      <RoomStayCandidates>
        <RoomStayCandidate Quantity="1">
          <GuestCounts><GuestCount AgeQualifyingCode="10" Count="2"/></GuestCounts>
        </RoomStayCandidate>
      </RoomStayCandidates>
    </AvailRequestSegment>
  </AvailRequestSegments>
</OTA_HotelAvailRQ>
```

### Parsed field

`//RoomStay/Total/@AmountAfterTax` — the server takes the minimum value across
all returned room stays as the `min_rate`.

---

## Go package index {#packages}

| Package | Responsibility |
|---------|---------------|
| `cmd/archipelago-hotels-mcp` | Entry point, transport dispatch (stdio vs HTTP) |
| `internal/server` | MCP server wiring, Gin HTTP routes |
| `internal/repository` | DB pool, hotel/brand/room queries, thumbnail rewriting |
| `internal/rate` | Rate service, BatchMinRates, circuit breaker, TTL cache |
| `internal/rate/simplebooking` | SimpleBooking XML builder and parser |
| `internal/rate/sentec` | Sentec REST client |
| `internal/tools` | MCP tool handler functions |
| `internal/resources` | MCP resource registration (ext-app) |

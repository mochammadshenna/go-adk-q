# How to Add a New Hotel Brand / Database

This guide covers every code location you must touch to onboard a new Archipelago brand into the MCP server. Follow the steps in order — each step depends on the previous one compiling cleanly.

---

## Prerequisites

- The new brand's database exists on the same MySQL host as the other brand DBs.
- The brand has a row in `tb_brands` in the central `db_archipelagowebsite` database, with `db_prefix_name` set to the correct prefix.
- You have read access to the new brand's DB to confirm table and column names.

---

## Step 1 — Register the DB name (if it does not follow the default pattern)

**File:** `internal/repository/repository.go`

By default, `connectBrand()` derives the database name as `db_{prefix}website`. If the new brand's actual database name differs from that pattern, add an entry to the `brandDBName` map.

```go
// brandDBName maps prefix differences between db_prefix_name and actual database name.
// ponytail: static map — not every brand DB follows the {prefix}website pattern.
var brandDBName = map[string]string{
    "favehotel": "db_favewebsite",
    "pba":       "db_pba",
    // Add your new brand here if its DB name does not follow db_{prefix}website:
    "nordic":    "db_nordicwebsite",  // example: omit if the default already works
}
```

If the default pattern works (e.g., the prefix is `harper` and the DB is `db_harperwebsite`), you do **not** need an entry here.

**Verification:** After adding, confirm `connectBrand()` resolves the correct name by checking the startup log:

```
INFO brand DB connected prefix=nordic db=db_nordicwebsite
```

---

## Step 2 — Add the brand gradient for the UI dashboard

**File:** `internal/repository/repository.go`

The `brandImageStyle()` function returns a Tailwind gradient class used on hotel cards in the browser dashboard. Look up the brand's primary colour and add a matching entry. The key is the **lowercase brand name** as it appears in `tb_brands.brand_name`.

```go
func brandImageStyle(brandName string) string {
    styles := map[string]string{
        "aston":        "bg-gradient-to-br from-blue-800 to-sky-700",
        "grand aston":  "bg-gradient-to-br from-indigo-900 to-purple-900",
        // ... existing entries ...

        // Add new brand below — key must match brand_name lowercased:
        "nordic":       "bg-gradient-to-br from-sky-800 to-indigo-700",
        "four corners": "bg-gradient-to-br from-lime-700 to-green-600",
        "my new brand": "bg-gradient-to-br from-teal-700 to-cyan-600",
    }
    if s, ok := styles[strings.ToLower(brandName)]; ok {
        return s
    }
    // Falls through to the default grey — acceptable but not ideal.
    return "bg-gradient-to-br from-gray-700 to-gray-600"
}
```

If you omit this step the brand still works; cards just render with the default grey gradient.

---

## Step 3 — Handle schema deviations in the room query

**File:** `internal/repository/room.go`

`GetRooms()` queries either `tb_hrooms` (all standard brands) or `tb_hroom` (PBA). If your new brand uses a non-standard table name or a different `status` column, add a special case alongside the existing PBA block.

**Standard brand (nothing to do):**

```go
// GetRooms already defaults to tb_hrooms / room_status / "Y".
// Standard brands require no special handling.
```

**New brand with a different table (PBA-style special case):**

```go
func (p *Pool) GetRooms(ctx context.Context, brandPrefix string, apiHotelID int) ([]RoomRow, error) {
    // ...
    table := "tb_hrooms"
    statusCol := "room_status"
    statusVal := "Y"

    // PBA uses a different table and column naming.
    if brandPrefix == "pba" {
        table = "tb_hroom"
        statusCol = "status"
        statusVal = "1"
    }

    // Add your brand if it also deviates:
    if brandPrefix == "mybrand" {
        table = "tb_room_types"   // actual table name in their DB
        statusCol = "is_active"
        statusVal = "1"
    }
    // ...
}
```

The remaining column detection (`room_rate`, `sb_id`, `sentec_id`) is already handled generically via `HasColumn()`, so those adapt automatically once `scanColumns()` runs on connect.

**PBA special case reference — what it looks like in practice:**

| Attribute    | Standard brands       | PBA              |
|--------------|-----------------------|------------------|
| Table        | `tb_hrooms`           | `tb_hroom`       |
| Status col   | `room_status`         | `status`         |
| Active value | `"Y"`                 | `"1"`            |

---

## Step 4 — Verify lazy connect and column introspection

`BrandDB()` connects lazily (first call) and `scanColumns()` runs immediately after a successful ping. No code change is needed for this step — confirm it works:

```bash
# Run the server in HTTP mode with DEBUG=1
DEBUG=1 ./bin/archipelago-hotels-mcp --http

# Hit the detail endpoint for a hotel belonging to the new brand
curl "http://localhost:9011/api/hotels?brand=My+New+Brand" | jq '.[0].hotel_id'

# Confirm in logs:
# INFO  brand DB connected  prefix=mybrand db=db_mybrandwebsite
# (no WARN about column scan failure)
```

If you see `WARN brand DB unreachable` the DSN is wrong — re-check Step 1 or verify network/credentials.

If you see `WARN column scan failed` the DB user lacks `SELECT` on `INFORMATION_SCHEMA.COLUMNS`. Grant it:

```sql
GRANT SELECT ON INFORMATION_SCHEMA.* TO 'your_user'@'%';
```

To inspect what columns were discovered at runtime, add a temporary debug log in `scanColumns()`:

```go
slog.Debug("column scan result", "prefix", prefix, "tables", tables)
```

---

## Step 5 — Verify the rate fallback chain

The rate service (`internal/rate/rate.go`) derives everything it needs from `BrandCredentials`, which `GetCredentials()` returns. Credentials are read from `tb_hotels` in the brand DB via `HasColumn()` introspection — no code change is needed unless the new brand uses a non-standard credentials table.

Test the three-level fallback in order:

**Level 1 — SimpleBooking live rate (preferred)**

`GetCredentials()` must return a non-nil result with `SimpleBookingID > 0` and valid `XMLUser`/`XMLPass`. If the new brand has no SimpleBooking contract, this level is skipped automatically and the fallback continues.

```bash
# A hotel detail call exercises all three levels:
curl "http://localhost:9011/api/hotels?brand=My+New+Brand" | jq '.[0]'
# Check logs for:
# DEBUG simplebooking rate fetched  hotel_id=12345 rate=450000
# or
# DEBUG simplebooking circuit open  hotel_id=12345   (fallback triggered)
```

**Level 2 — `tb_hrooms.room_rate` stored rate**

Populated by `GetRooms()`. Requires `room_rate` column to exist (detected by `HasColumn()`).

**Level 3 — `hotel_starting_price` from central DB**

Always available as a last resort; no brand-specific action needed.

---

## Step 6 — Update the MCP server instructions string

**File:** `internal/server/server.go`

The `Instructions` field is returned to MCP clients (e.g., Claude) to describe the server's capabilities. Update the brand list to include the new brand so the LLM is aware of it.

```go
&mcp.ServerOptions{
    Instructions: `# Archipelago Hotels MCP Server

Search, recommend, and explore hotels across 14 Archipelago brands (Aston, Grand Aston,
The Alana, Harper, NEO, favehotels, Kamuela, Quest, Huxley, Nordic, Four Corners,
PBA, My New Brand, and more).

## Tools
...
`,
},
```

Keep the brand count in sync with the actual number of distinct brands in `tb_brands`.

---

## Step 7 — Smoke test

Run the full test sequence against a hotel from the new brand:

```bash
# 1. Start the server
make dev-http

# 2. Confirm the brand appears in the brand list
curl http://localhost:9011/api/brands | jq '.[] | select(.brand == "My New Brand")'

# 3. Search for hotels in that brand
curl "http://localhost:9011/api/hotels?brand=My+New+Brand" | jq 'length'

# 4. Confirm rooms load for a specific hotel (pick an api_hotel_id from step 3)
# Use the MCP tool directly via the dashboard at http://localhost:9011/dashboard
```

Expected log sequence on first hotel detail request:

```
INFO  brand DB connected      prefix=mybrand db=db_mybrandwebsite
INFO  simplebooking rate fetch hotel_id=9999 rooms=3
```

---

## Checklist

- [ ] `brandDBName` entry added (if DB name deviates from `db_{prefix}website`)
- [ ] Gradient added to `brandImageStyle()` map
- [ ] Special table/status handling added to `GetRooms()` (if schema deviates from standard)
- [ ] `BrandDB()` lazy connect confirmed in logs (no `brand DB unreachable` warning)
- [ ] `scanColumns()` ran cleanly (no `column scan failed` warning)
- [ ] Rate fallback chain exercised — at minimum Level 3 (starting price) returns a value
- [ ] Brand name added to `Instructions` string in `server.go`
- [ ] Smoke test: brand visible in `/api/brands`, hotels visible in `/api/hotels`, rooms visible via dashboard

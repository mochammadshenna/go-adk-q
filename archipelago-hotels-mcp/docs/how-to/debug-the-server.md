# How to debug common issues

## Enable debug logging

Set `DEBUG=1` before starting the server:

```sh
DEBUG=1 archipelago-hotels-mcp stdio
DEBUG=1 archipelago-hotels-mcp http -addr :9011
```

All log output goes to **stderr**, which Claude Desktop captures in its MCP server log. In HTTP mode you can watch it directly in the terminal.

Debug level adds:

- Per-brand DB connection attempts and outcomes
- Brand DB column scan results
- SimpleBooking request details and circuit breaker state
- Cache hits and misses for room rates

---

## No hotels returned

**Symptom:** `search_hotels` returns an empty list or an error.

**Check 1: MYSQL_DB is correct.**

The default is `db_archipelagowebsite`. If your central database has a different name, set `MYSQL_DB` accordingly:

```sh
MYSQL_DB=db_archipelago_prod archipelago-hotels-mcp stdio
```

**Check 2: Central DB is reachable.**

At startup the server pings the central DB. If it fails you will see:

```
level=ERROR msg="database connection failed" error="central ping: ..."
level=WARN  msg="starting in DEGRADED mode — database unavailable"
```

Verify network connectivity and credentials. In HTTP mode, call the health endpoint:

```sh
curl http://localhost:9011/health
```

A healthy response:

```json
{"status":"ok","version":"dev","db":true}
```

A degraded response:

```json
{"status":"degraded","version":"dev","db":false}
```

**Check 3: `tb_brands` has `db_prefix_name` set.**

Hotels without a resolvable brand DB prefix are returned without live rates but should still appear in search results. If the central DB query itself is failing, check MySQL logs on the server.

---

## No prices shown

**Symptom:** Hotels appear but all prices are zero or `null`.

**Check 1: SimpleBooking circuit breaker.**

The circuit breaker opens after five consecutive failures and stays open for 120 seconds. Look for this log line:

```
level=WARN msg="simplebooking: circuit breaker opened" failures=5
```

While open, all live rate calls are skipped. The server falls back to stored `tb_hrooms.room_rate` values, then to `hotel_starting_price` from the central DB. If all three sources return zero, the price is omitted.

**Check 2: Brand DB credentials.**

SimpleBooking requires `SimpleBookingID`, `SimpleBookingUser`, and `SimpleBookingPass` from the brand DB. With `DEBUG=1`, a missing-credentials hotel logs:

```
level=WARN msg="rate: credentials fetch failed" prefix=aston hotel_id=42
```

Check the `tb_hrooms` or equivalent credentials table in the brand DB.

**Check 3: Stored rates are zero.**

If `tb_hrooms.room_rate` is 0 for all rooms and `hotel_starting_price` is also 0 in the central DB, no price will be shown. This is a data issue, not a server issue.

---

## No thumbnails in the dashboard

**Symptom:** Hotel cards show gradient placeholders instead of images.

**Check 1: `url_image_resizer` is set.**

If your images are served through a CDN or resizer, the `url_image_resizer` environment variable must be set to the base URL. Without it, the server uses the raw URL from the database, which may be blocked by the MCP Apps content security policy.

```sh
url_image_resizer=https://img.archipelagohotels.com archipelago-hotels-mcp stdio
```

**Check 2: `thumbnail_desktop` column exists.**

The server only queries `thumbnail_desktop` if `HasColumn` returns true for the brand. Confirm the column exists in the brand's `tb_hotels` table:

```sql
SELECT COLUMN_NAME
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_SCHEMA = 'db_astonwebsite'
  AND TABLE_NAME = 'tb_hotels'
  AND COLUMN_NAME = 'thumbnail_desktop';
```

If the column is missing, the gradient fallback is intentional — no code change is needed, but the database schema may need updating.

---

## Dashboard UI not opening in Claude Desktop

**Symptom:** The `find_hotels` tool runs but no visual dashboard appears.

**Check 1: The tool's `Meta.resourceUri` is set.**

In `internal/tools/dashboard.go`, the tool registration includes:

```go
Meta: mcp.Meta{
    "ui": map[string]any{
        "resourceUri":     resources.ResourceURI,
        "resourceDomains": []string{"images.archipelagohotels.com"},
    },
},
```

If this was accidentally removed, Claude Desktop has no signal to open the UI panel. Restore it and rebuild.

**Check 2: Claude Desktop was restarted after the binary was rebuilt.**

Claude Desktop caches the tool manifest at startup. After any `make build-go`, restart Claude Desktop fully — quitting from the menu bar, not just closing the window.

---

## Health endpoint

In HTTP mode, a quick sanity check:

```sh
curl -s http://localhost:9011/health | jq .
```

Response fields:

| Field | Type | Meaning |
|---|---|---|
| `status` | string | `"ok"` or `"degraded"` |
| `version` | string | Binary version (set at build time via `-ldflags`) |
| `db` | bool | `true` if central DB ping succeeded |

A `"degraded"` status with `"db": false` means the central database is unreachable. All hotel queries will return errors until connectivity is restored.

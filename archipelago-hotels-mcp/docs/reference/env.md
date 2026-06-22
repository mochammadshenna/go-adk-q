# Environment Variables Reference

This document covers every configuration knob for `archipelago-hotels-mcp`.

---

## Runtime Environment Variables

These are read at process startup via `repository.ConfigFromEnv()` and `os.Getenv` calls
in the `hotel.go` image-rewriter. None are required — the server starts without any of them
set, using the defaults listed below.

| Variable | Type | Default | Description |
|---|---|---|---|
| `MYSQL_HOST` | string | `127.0.0.1` | Hostname or IP of the MySQL server for both the central database and all per-brand databases. |
| `MYSQL_PORT` | string | `3306` | TCP port of the MySQL server. |
| `MYSQL_USER` | string | `root` | MySQL username. Must have `SELECT` access on `db_archipelagowebsite` and on every per-brand database (`db_astonwebsite`, `db_neowebsite`, `db_favewebsite`, etc.). |
| `MYSQL_PASS` | string | _(empty)_ | MySQL password. Leave empty for passwordless local dev; set in production. |
| `MYSQL_DB` | string | `db_archipelagowebsite` | Central database name. Overriding this is rare; only do it if the central catalog lives under a different schema name in your environment. |
| `DEBUG` | string | `0` | Set to `1` to enable `slog.LevelDebug` output on stderr. Includes SQL query tracing, brand-DB connection events, and SimpleBooking request/response details. Any other value (including unset) leaves the log level at `Info`. |
| `url_image_resizer` | string | `https://images.archipelagohotels.com/` | Base URL of the Archipelago image-resize proxy. Hotel thumbnail URLs from brand databases are rewritten through this proxy so that the MCP App UI can display them without violating the Claude Desktop Content Security Policy. Must end with a trailing slash. |

### Notes

**`MYSQL_HOST` / `MYSQL_PORT`** apply to all database connections. The server uses a single
MySQL endpoint; the central database and all per-brand databases must be reachable on the same
host and port.

Per-brand database names are derived automatically from the `db_prefix_name` column in
`tb_brands`:

```
db_{prefix}website     — default pattern (e.g. db_astonwebsite)
db_favewebsite         — hardcoded override for prefix "favehotel"
db_pba                 — hardcoded override for prefix "pba"
```

**`DEBUG=1`** is safe in production for short-lived debugging sessions. Log output goes to
`stderr`; MCP stdio transport uses `stdin`/`stdout` and is not affected.

**`url_image_resizer`** mirrors the `url_image_resizer` PHP config key from the Sentec
platform. The rewrite logic is a pure string transform (no HTTP fetch at rewrite time).

---

## SimpleBooking Provider Credentials

The SimpleBooking XML API requires two provider-level credentials in addition to each hotel's
own XMLHotelAgent username/password. These are **compile-time constants** in
`internal/rate/simplebooking.go`, not environment variables:

```go
// internal/rate/simplebooking.go
const (
    providerUsername = "Xmsfttgad33"
    providerPassword = "XMLfegg423!.33"
)
```

They are the same across every hotel (ported from the PHP source) and are injected into the
`<provider Name="..." Pwd="..."/>` element of every `OTA_HotelAvailRQ` XML request.

Per-hotel SimpleBooking credentials (`SimpleBookingID`, `SimpleBookingUser`,
`SimpleBookingPass`) are stored in each brand's database and fetched at runtime via
`repository.GetCredentials()`.

If the provider credentials need to change, rebuild the binary after editing the constants.
There is no environment-variable override path for them today.

---

## HTTP Transport Options (CLI flags, not env vars)

When running in `http` mode, the listen address and verbosity are set via CLI flags rather
than environment variables:

```
archipelago-hotels-mcp http                     # :9011, release mode
archipelago-hotels-mcp http -addr :8080         # custom port
archipelago-hotels-mcp http -verbose            # Gin request logging to stderr
```

`-verbose` is independent of `DEBUG`; it enables Gin's access log (one line per HTTP
request). `DEBUG=1` controls Go's structured `slog` output.

---

## .env.example

Copy this to `.env` and source it before running locally, or paste values directly into your
shell / process manager.

```dotenv
# .env.example — Archipelago Hotels MCP Server
# Copy to .env, fill in values, then: source .env

# ── MySQL ────────────────────────────────────────────────────
MYSQL_HOST=127.0.0.1
MYSQL_PORT=3306
MYSQL_USER=root
MYSQL_PASS=

# Central catalog database (rarely needs changing)
MYSQL_DB=db_archipelagowebsite

# ── Logging ──────────────────────────────────────────────────
# Set to 1 to enable debug-level log output on stderr
DEBUG=0

# ── Image CDN ────────────────────────────────────────────────
# Trailing slash required
url_image_resizer=https://images.archipelagohotels.com/
```

---

## claude_desktop_config.json Snippet

Add this to your Claude Desktop configuration (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS) to connect via stdio:

```json
{
  "mcpServers": {
    "archipelago-hotels": {
      "command": "/path/to/archipelago-hotels-mcp",
      "args": ["stdio"],
      "env": {
        "MYSQL_HOST": "127.0.0.1",
        "MYSQL_PORT": "3306",
        "MYSQL_USER": "root",
        "MYSQL_PASS": "",
        "MYSQL_DB": "db_archipelagowebsite",
        "DEBUG": "0",
        "url_image_resizer": "https://images.archipelagohotels.com/"
      }
    }
  }
}
```

Replace `/path/to/archipelago-hotels-mcp` with the absolute path to the compiled binary
(e.g. the output of `make build`, which writes to `bin/archipelago-hotels-mcp`).

The `env` block is optional if the defaults are correct for your setup. You can omit any key
whose default value is acceptable.

For HTTP transport (e.g. connecting a remote client to the server), run the binary separately:

```bash
MYSQL_HOST=db.internal \
MYSQL_USER=archipelago \
MYSQL_PASS=secret \
./bin/archipelago-hotels-mcp http -addr :9011
```

Then configure your MCP client to point at `http://localhost:9011/mcp`.

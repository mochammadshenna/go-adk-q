# How-To Guides

> **Task-oriented.** Each guide solves one specific real-world problem.
> Assumes you already have a working build — see [Tutorials](../tutorials/README.md) first.

---

## Configure database connections {#configure-db}

All database settings are read from environment variables at startup.

| Variable | Default | Notes |
|----------|---------|-------|
| `MYSQL_HOST` | `127.0.0.1` | Hostname or IP |
| `MYSQL_PORT` | `3306` | Port |
| `MYSQL_USER` | `root` | Username |
| `MYSQL_PASS` | *(empty)* | Password |
| `MYSQL_DB` | `db_archipelagowebsite` | Central catalog DB |

Brand databases are discovered automatically from `tb_brands.db_prefix_name` in
the central DB. They must all reside on the same host.

**Using a `.env` file with `make`:**

```bash
cp .env.example .env
# Edit .env, then:
make dev-http   # Makefile exports .env vars before running the binary
```

**Connection pool sizing** — defaults are 5 max open / 5 max idle connections
per database. Tune via `MYSQL_MAX_OPEN` and `MYSQL_MAX_IDLE` if needed.

---

## Run the HTTP server and ext-app {#run-http-server}

By default the binary uses **stdio** transport (for Claude Desktop). To run as
an HTTP server instead:

```bash
./bin/archipelago-hotels-mcp --http
# Listens on :9011
```

Or with the Makefile hot-reload target:

```bash
make dev-http
```

**Endpoints available in HTTP mode:**

| Method | Path | Purpose |
|--------|------|---------|
| `POST`/`GET` | `/mcp` | MCP Streamable HTTP |
| `GET` | `/api/hotels` | JSON hotel list |
| `GET` | `/api/brands` | JSON brand list |
| `GET` | `/api/regions` | JSON region list |
| `GET` | `/dashboard` | Standalone HTML dashboard |
| `GET` | `/health` | Health check JSON |

**Register as HTTP MCP server in Claude Desktop:**

```jsonc
{
  "mcpServers": {
    "archipelago-hotels": {
      "url": "http://localhost:9011/mcp"
    }
  }
}
```

---

## Add a new hotel brand database {#add-brand-db}

When a new brand database (e.g. `db_nuviwebsite`) is provisioned:

1. **Insert the brand row** in `db_archipelagowebsite.tb_brands`:

   ```sql
   INSERT INTO tb_brands (brand_name, db_prefix_name, parent_brand_id)
   VALUES ('Nuvi Hotels', 'nuvi', NULL);
   ```

2. **Ensure `tb_hotels` columns exist** in the new brand DB. The repository
   uses `INFORMATION_SCHEMA.COLUMNS` introspection at first query, so missing
   columns degrade gracefully to `nil` rather than causing a query error.

3. **Restart the server** — the pool manager lazy-connects on first use, so no
   code change is required if the schema follows the standard layout.

4. **Verify:** call `search_hotels` with a city that has Nuvi hotels and confirm
   the `db_prefix` field in the response shows `nuvi`.

If the new brand uses Sentec booking channel, set `hotel_channel = 'SENTEC'` in
its `tb_hotels` rows and ensure `SENTEC_API_URL` is configured.

---

## Override the rate fallback strategy {#rate-fallback}

The default fallback chain is:

```
SimpleBooking XML API (live)
    → tb_hrooms.room_rate (per-brand DB)
        → hotel_starting_price (central DB)
```

**Disable live rate fetching entirely** (use stored prices only):

```bash
SB_DISABLED=1 ./bin/archipelago-hotels-mcp
```

**Change the rate cache TTL** (default 300 s):

```bash
CACHE_RATE_TTL=60 ./bin/archipelago-hotels-mcp   # 1-minute TTL
```

**Force fallback for a specific brand** — set `hotel_channel` to an empty string
in that brand's `tb_hotels`. The rate service treats missing/empty channel as
"no live API available" and skips straight to stored prices.

All responses include a `rate_source` field (`"live"`, `"stored"`, or
`"starting_price"`) so callers can surface data-freshness warnings to users.

---

## Enable the circuit breaker for SimpleBooking {#circuit-breaker}

The circuit breaker opens after 5 consecutive failures within 60 seconds and
stays open for 120 seconds before allowing a probe request.

Tune via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `SB_CB_THRESHOLD` | `5` | Failures before opening |
| `SB_CB_WINDOW` | `60` | Observation window (seconds) |
| `SB_CB_TIMEOUT` | `120` | Open duration before probe (seconds) |
| `SB_REQUEST_TIMEOUT` | `15` | Per-request timeout (seconds) |

When the breaker is open, `rate_source` in the response will be `"stored"` or
`"starting_price"`. Check `/health` for current breaker state:

```bash
curl http://localhost:9011/health | jq '.rate_circuit_breaker'
```

---

## Run the test suite {#run-tests}

```bash
go test ./...                   # all packages
go test ./internal/rate/...     # rate service only
go test -run TestSearch ./...   # single test by name
go vet ./...                    # static analysis
```

Integration tests that hit MySQL require the `MYSQL_*` env vars to be set. Unit
tests use in-memory fakes and have no external dependencies.

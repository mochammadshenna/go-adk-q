# Environment Configuration

This guide covers every environment variable recognized by the Archipelago Hotels MCP server, plus ready-to-use configuration snippets for common deployment scenarios.

---

## Environment Variables Reference

All variables are optional. The server starts with the defaults shown below; set only what you need to override.

| Variable | Type | Default | Required | Purpose |
|---|---|---|---|---|
| `MYSQL_HOST` | string | `127.0.0.1` | No | MySQL server hostname or IP |
| `MYSQL_PORT` | string | `3306` | No | MySQL server port |
| `MYSQL_USER` | string | `root` | No | MySQL login user |
| `MYSQL_PASS` | string | _(empty)_ | No | MySQL login password |
| `MYSQL_DB` | string | `db_archipelagowebsite` | No | Central catalog database name |
| `DEBUG` | string | `0` | No | Set to `1` to enable debug-level structured logging |
| `url_image_resizer` | string | `https://images.archipelagohotels.com/` | No | Image CDN proxy base URL for hotel thumbnails |

> **Note on `url_image_resizer` casing.** This variable uses lowercase to match the legacy PHP/CMS convention from which the image URL scheme was inherited. The Go server reads it with `os.Getenv("url_image_resizer")` — the exact casing must be preserved in your shell or container environment.

---

## Variable Details

### Database Variables (`MYSQL_*`)

These five variables map directly to the `repository.Config` struct populated by `repository.ConfigFromEnv()`.

| Variable | Notes |
|---|---|
| `MYSQL_HOST` | DNS name or IP. Use `localhost` only if the server runs in the same container/process namespace as MySQL; otherwise use the real hostname or service name. |
| `MYSQL_PORT` | Port number as a string. Overriding is useful when using an SSH tunnel or a non-standard port. |
| `MYSQL_USER` | The user needs SELECT on `db_archipelagowebsite` and on all eight per-brand databases (`aston_*`, `neo_*`, `fave_*`, `alana_*`, `harper_*`, `kamuela_*`, `quest_*`, `pba_*`). |
| `MYSQL_PASS` | Leave empty only in local development with a password-less root account. Never leave empty in staging or production. |
| `MYSQL_DB` | The central catalog database. Change this only if you have renamed or mirrored the central DB. Per-brand DBs are discovered dynamically; they are not configured here. |

The server uses a lazy-connect pool. Brand databases are opened on first use, not at startup. A startup failure on the central DB causes the server to start in **degraded mode** (MCP tools return errors; HTTP health endpoint reports the failure).

### `DEBUG`

Controls the `slog` level for the entire process.

| Value | Behavior |
|---|---|
| `0` (default) | `INFO` level — operational events only (server start, connection errors, rate API calls) |
| `1` | `DEBUG` level — all of the above plus SQL queries, rate cache hits/misses, circuit-breaker state transitions, HTTP request details |

Logs are written to **stderr** in `slog` text format. In stdio mode this means debug output goes to stderr while MCP messages travel on stdout — Claude Desktop captures both independently.

### `url_image_resizer`

The server rewrites hotel thumbnail URLs through a CDN proxy so that images are served from a single allowed domain (`images.archipelagohotels.com`). This keeps the MCP App within its CSP `resourceDomains` declaration without requiring any HTTP fetch from the server process itself.

The value must end with a trailing slash.

**When to change it:**

- **Local development** — set to an empty string (`url_image_resizer=`) to disable rewriting; raw CDN URLs from the database are returned as-is.
- **Staging** — set to your staging CDN proxy, e.g. `https://images-staging.archipelagohotels.com/`.
- **Self-hosted mirror** — set to your own reverse-proxy base URL if you have mirrored the image CDN.

If the variable is unset the default production CDN is used, which is correct for production deployments.

---

## `.env.example`

Copy this file to `.env` in the project root and fill in the values for your environment. The server does not load `.env` files automatically — use `export`, `direnv`, Docker `--env-file`, or your CI/CD platform's secret injection.

```dotenv
# .env.example — Archipelago Hotels MCP Server
# Copy to .env and fill in values. Never commit .env to version control.

# ── Database ──────────────────────────────────────────────────────────────────
MYSQL_HOST=127.0.0.1
MYSQL_PORT=3306
MYSQL_USER=archipelago_ro
MYSQL_PASS=change_me
MYSQL_DB=db_archipelagowebsite

# ── Logging ───────────────────────────────────────────────────────────────────
# Set to 1 to enable debug-level logging (SQL, cache, circuit-breaker events)
DEBUG=0

# ── Image CDN ─────────────────────────────────────────────────────────────────
# Base URL of the image resizer proxy. Must end with a trailing slash.
# Leave unset to use the production default: https://images.archipelagohotels.com/
# Set to empty string to disable URL rewriting (returns raw DB image URLs).
url_image_resizer=https://images.archipelagohotels.com/
```

---

## Claude Desktop Configuration

### stdio mode (recommended for Claude Desktop)

Edit `~/Library/Application Support/Claude/claude_desktop_config.json` (macOS) or `%APPDATA%\Claude\claude_desktop_config.json` (Windows).

```json
{
  "mcpServers": {
    "archipelago-hotels": {
      "command": "/usr/local/bin/archipelago-hotels-mcp",
      "args": ["stdio"],
      "env": {
        "MYSQL_HOST": "127.0.0.1",
        "MYSQL_PORT": "3306",
        "MYSQL_USER": "archipelago_ro",
        "MYSQL_PASS": "your_password_here",
        "MYSQL_DB": "db_archipelagowebsite",
        "url_image_resizer": "https://images.archipelagohotels.com/",
        "DEBUG": "0"
      }
    }
  }
}
```

**Paths:**

| Platform | Binary path example |
|---|---|
| macOS (Homebrew prefix) | `/usr/local/bin/archipelago-hotels-mcp` |
| macOS (project build) | `/Users/you/go/bin/archipelago-hotels-mcp` |
| Linux | `/usr/local/bin/archipelago-hotels-mcp` |
| Windows | `C:\Users\you\go\bin\archipelago-hotels-mcp.exe` |

Replace the `command` value with the actual path produced by `make build` or `go install`. Claude Desktop spawns the process directly — it does not use your shell's `PATH`.

> **Security note.** Passwords placed in `claude_desktop_config.json` are stored in plaintext. On macOS, consider storing the password in Keychain and reading it via a wrapper shell script instead.

### Using a wrapper script (for secret injection)

```json
{
  "mcpServers": {
    "archipelago-hotels": {
      "command": "/usr/local/bin/archipelago-hotels-mcp-wrapper.sh",
      "args": []
    }
  }
}
```

`/usr/local/bin/archipelago-hotels-mcp-wrapper.sh`:

```bash
#!/usr/bin/env bash
# Reads DB password from macOS Keychain; falls back to env if already set.
export MYSQL_HOST=127.0.0.1
export MYSQL_PORT=3306
export MYSQL_USER=archipelago_ro
export MYSQL_PASS=$(security find-generic-password -a archipelago_ro -s archipelago-mcp -w 2>/dev/null)
export MYSQL_DB=db_archipelagowebsite
export url_image_resizer=https://images.archipelagohotels.com/
exec /usr/local/bin/archipelago-hotels-mcp stdio
```

---

## HTTP Mode Configuration

HTTP mode exposes the MCP Streamable HTTP endpoint alongside the REST dashboard API on port `:9011` (default).

### Running with a custom address

```bash
export MYSQL_HOST=db.internal.archipelagohotels.com
export MYSQL_PORT=3306
export MYSQL_USER=archipelago_ro
export MYSQL_PASS=your_password_here
export MYSQL_DB=db_archipelagowebsite
export url_image_resizer=https://images.archipelagohotels.com/

archipelago-hotels-mcp http -addr :9011
```

### Available HTTP mode flags

| Flag | Default | Purpose |
|---|---|---|
| `-addr` | `:9011` | Listen address in `host:port` form |
| `-verbose` | false | Equivalent to `DEBUG=1`; enables Gin debug output in addition to slog debug |

These are CLI flags, not environment variables. They are parsed from `os.Args` in the `http` subcommand.

### HTTP endpoints summary

| Method | Path | Purpose |
|---|---|---|
| `POST` / `GET` | `/mcp` | MCP Streamable HTTP transport |
| `GET` | `/dashboard` | Standalone HTML hotel dashboard |
| `GET` | `/api/hotels` | JSON hotel list (`?city=&brand=`) |
| `GET` | `/api/brands` | JSON brand list |
| `GET` | `/api/regions` | JSON region list |
| `GET` | `/health` | `{ "status": "ok", "version": "...", "db": "..." }` |

---

## Debug Mode

Set `DEBUG=1` to enable verbose logging. All output goes to **stderr**.

```bash
DEBUG=1 archipelago-hotels-mcp stdio
# or
DEBUG=1 archipelago-hotels-mcp http -verbose
```

What becomes visible at `DEBUG` level:

- SQL queries issued against the central and brand databases
- Per-brand DB lazy-connect events (first connection per brand)
- INFORMATION_SCHEMA column introspection results
- Rate service: cache hits, cache misses, TTL expiry
- SimpleBooking XML API: request/response round-trips, 5-worker pool activity
- Circuit breaker state transitions (closed → open → half-open → closed)
- HTTP request/response details (HTTP mode only, via Gin debug)
- MCP tool invocation parameters and timing

**Do not enable `DEBUG=1` in production.** Debug logs may include hotel room rates, partial XML API responses, and DB query parameters.

---

## `url_image_resizer` Deep Dive

### What it does

`resizeImageURL()` in `internal/repository/hotel.go` rewrites raw image URLs from the database into a form that passes through the image CDN proxy:

```
raw DB URL:    https://old-cdn.example.com/hotels/123/photo.jpg
rewritten:     https://images.archipelagohotels.com/hotels/123/photo.jpg
```

The rewrite strips the origin and prepends the resizer base URL. No HTTP request is made by the server; the rewrite is purely a string operation. The browser (or MCP App frontend) fetches the rewritten URL directly.

### Why this matters

The MCP App UI is served as a `text/html;profile=mcp-app` resource and runs under a strict Content Security Policy. Images must come from a domain listed in the tool's `resourceDomains` declaration (`images.archipelagohotels.com`). Without the rewrite, raw CDN URLs from the database would be blocked by the CSP.

### When to override

| Scenario | Value to set |
|---|---|
| Production (default) | `https://images.archipelagohotels.com/` (or leave unset) |
| Staging | `https://images-staging.archipelagohotels.com/` |
| Local dev (bypass rewrite) | `url_image_resizer=` (empty string) |
| Self-hosted CDN | `https://your-cdn.example.com/` |

---

## Production Hardening Checklist

Before deploying to a production or customer-facing environment:

- [ ] **MYSQL_PASS is set** — never run production with an empty DB password.
- [ ] **MYSQL_USER is a read-only account** — the MCP server only reads data; grant SELECT only. Do not use root.
- [ ] **DEBUG=0** (or unset) — debug logging can expose rate API payloads and query parameters.
- [ ] **`url_image_resizer` points to the production CDN** — confirm the trailing slash is present.
- [ ] **DB user has SELECT on all brand databases** — missing grants cause individual brand lookups to fail silently with a rate fallback to `hotel_starting_price`.
- [ ] **Firewall: MySQL port not publicly exposed** — the MCP server should be the only process that can reach the DB host on port 3306.
- [ ] **HTTP mode behind a reverse proxy** — if running in HTTP mode, place Nginx or a load balancer in front; do not expose `:9011` directly to the internet.
- [ ] **TLS termination at the proxy layer** — the built-in Gin HTTP server does not terminate TLS.
- [ ] **Graceful shutdown** — the server handles `SIGINT`/`SIGTERM`; ensure your process manager (systemd, Docker, Kubernetes) sends `SIGTERM` and allows at least 10 seconds for in-flight rate API calls to complete.
- [ ] **Health check wired** — configure your load balancer or container orchestrator to poll `GET /health` and remove the instance from rotation if the DB reports unhealthy.
- [ ] **Log shipping** — stderr output should be captured and forwarded to your log aggregator; the slog text format is line-oriented and easy to parse.

---

## Docker / Container Example

### `docker-compose.yml`

```yaml
version: "3.9"

services:
  archipelago-mcp:
    image: ghcr.io/archipelagohotels/archipelago-hotels-mcp:latest
    restart: unless-stopped
    ports:
      - "9011:9011"
    environment:
      MYSQL_HOST: db
      MYSQL_PORT: "3306"
      MYSQL_USER: archipelago_ro
      MYSQL_PASS: "${MYSQL_PASS}"          # inject from host .env or CI secret
      MYSQL_DB: db_archipelagowebsite
      url_image_resizer: "https://images.archipelagohotels.com/"
      DEBUG: "0"
    command: ["http", "-addr", ":9011"]
    depends_on:
      db:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:9011/health"]
      interval: 30s
      timeout: 5s
      retries: 3

  db:
    image: mysql:8.4
    environment:
      MYSQL_ROOT_PASSWORD: "${MYSQL_ROOT_PASSWORD}"
      MYSQL_DATABASE: db_archipelagowebsite
    volumes:
      - mysql_data:/var/lib/mysql
    healthcheck:
      test: ["CMD", "mysqladmin", "ping", "-h", "localhost"]
      interval: 10s
      timeout: 5s
      retries: 6

volumes:
  mysql_data:
```

### Dockerfile snippet (multi-stage build)

```dockerfile
FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/archipelago-hotels-mcp ./cmd/archipelago-hotels-mcp

FROM alpine:3.21
RUN apk add --no-cache ca-certificates wget
COPY --from=builder /out/archipelago-hotels-mcp /usr/local/bin/
ENTRYPOINT ["archipelago-hotels-mcp"]
CMD ["http", "-addr", ":9011"]
```

### Passing secrets safely

Avoid hardcoding `MYSQL_PASS` in `docker-compose.yml`. Use one of:

```bash
# Option 1 — host .env file (development only)
echo "MYSQL_PASS=secret" >> .env
docker compose up

# Option 2 — Docker secret (Swarm / production)
echo "secret" | docker secret create mysql_pass -
# then reference via secrets: in compose file and read from /run/secrets/mysql_pass

# Option 3 — CI/CD platform secret injection
# Set MYSQL_PASS as a masked environment variable in your pipeline;
# the value is injected at container start, never stored in the image.
```

---

## Quick Reference

```bash
# Minimal local run (stdio, no password)
MYSQL_HOST=127.0.0.1 archipelago-hotels-mcp stdio

# Local run with debug logging
DEBUG=1 MYSQL_PASS=secret archipelago-hotels-mcp stdio

# HTTP mode on default port
MYSQL_HOST=db.prod.internal MYSQL_USER=ro MYSQL_PASS=secret archipelago-hotels-mcp http

# HTTP mode on custom port
MYSQL_HOST=db.prod.internal MYSQL_USER=ro MYSQL_PASS=secret archipelago-hotels-mcp http -addr :8080

# Load from .env file (requires direnv or manual export)
set -a && source .env && set +a
archipelago-hotels-mcp stdio
```

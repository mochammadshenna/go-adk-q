# CLI Reference — archipelago-hotels-mcp

Archipelago Hotels MCP Server: search and explore hotels across Archipelago brands (279+ hotels, 13 brands).

---

## Synopsis

```
archipelago-hotels-mcp <command> [flags]
```

Running the binary with no arguments prints usage and exits with code 1.

---

## Commands

### `stdio`

Run the MCP server on **stdio transport**, suitable for Claude Desktop and any MCP host that spawns the process and communicates over stdin/stdout.

```
archipelago-hotels-mcp stdio
```

**Flags:** none

**Behavior:**
- Reads JSON-RPC MCP messages from stdin; writes responses to stdout.
- All log output goes to stderr (never pollutes the MCP stream).
- Log level is `INFO` by default; set `DEBUG=1` for verbose output.
- Handles `SIGINT` and `SIGTERM` for graceful shutdown.
- If the database is unreachable at startup the server starts in **degraded mode** — MCP tool calls that require DB data will return an error, but the process does not abort.

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Clean shutdown (signal received) |
| 1 | Fatal error (stdio transport failure) |

**Example — Claude Desktop `claude_desktop_config.json`:**

```json
{
  "mcpServers": {
    "archipelago-hotels": {
      "command": "/usr/local/bin/archipelago-hotels-mcp",
      "args": ["stdio"],
      "env": {
        "MYSQL_HOST": "db.internal",
        "MYSQL_USER": "mcp_reader",
        "MYSQL_PASS": "secret",
        "MYSQL_DB": "db_archipelagowebsite"
      }
    }
  }
}
```

**Example — quick smoke test:**

```sh
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}' \
  | archipelago-hotels-mcp stdio
```

---

### `http`

Run the MCP server on **Streamable HTTP transport** (MCP over HTTP, as defined in the MCP spec). Also exposes a REST dashboard API and a standalone HTML dashboard.

```
archipelago-hotels-mcp http [flags]
```

**Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-addr` | `string` | `:9011` | TCP address the HTTP server listens on. Accepts any `host:port` form accepted by Go's `net/http`. |
| `-verbose` | `bool` (switch) | `false` | Enable debug-level logging for every HTTP request. Equivalent to setting `DEBUG=1` in the environment but scoped to HTTP mode. |

**Behavior:**
- Starts a [Gin](https://github.com/gin-gonic/gin) HTTP server on the specified address.
- Handles `SIGINT` and `SIGTERM` with graceful HTTP shutdown.
- Degraded-mode startup (DB unavailable) behaves the same as in `stdio` mode.
- Gin access logs go to stdout; application/MCP logs go to stderr.

**HTTP endpoints:**

| Method | Path | Description |
|--------|------|-------------|
| `POST` / `GET` | `/mcp` | MCP Streamable HTTP endpoint (JSON-RPC) |
| `GET` | `/dashboard` | Standalone HTML dashboard (no MCP client needed) |
| `GET` | `/api/hotels` | JSON hotel list; accepts `?city=` and `?brand=` query params |
| `GET` | `/api/brands` | JSON list of all brands |
| `GET` | `/api/regions` | JSON list of all regions |
| `GET` | `/health` | JSON health check: `{ "status", "version", "db" }` |

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Clean shutdown (signal received) |
| 1 | Fatal error (HTTP server failure or bind error) |

**Examples:**

```sh
# Default — listen on :9011
archipelago-hotels-mcp http

# Custom port
archipelago-hotels-mcp http -addr :8080

# Custom port with debug logging
archipelago-hotels-mcp http -addr :8080 -verbose

# Health check (server already running)
curl http://localhost:9011/health
```

---

## Environment Variables

These variables are read at startup by both `stdio` and `http` commands.

| Variable | Default | Description |
|----------|---------|-------------|
| `MYSQL_HOST` | `127.0.0.1` | MySQL host |
| `MYSQL_PORT` | `3306` | MySQL port |
| `MYSQL_USER` | `root` | MySQL username |
| `MYSQL_PASS` | *(empty)* | MySQL password |
| `MYSQL_DB` | `db_archipelagowebsite` | Central catalog database name |
| `DEBUG` | `0` | Set to `1` to enable debug-level structured logging to stderr |
| `url_image_resizer` | `https://images.archipelagohotels.com/` | Image CDN proxy base URL used by `resizeImageURL()` |

---

## Makefile Targets

The repository `Makefile` wraps common development tasks. Run `make help` for a summary.

```
make [target]
```

### Build targets

| Target | Description |
|--------|-------------|
| `build` | Full build: runs `build-ui` then `build-go`. Produces `bin/archipelago-hotels-mcp`. |
| `build-ui` | Compiles the TypeScript MCP App UI with Vite (`ui/` → `ui/dist/index.html`), then copies the result to `internal/resources/mcp-app.html` so it is embedded into the Go binary at compile time. Requires `npm`. |
| `build-go` | Compiles the Go binary only to `bin/archipelago-hotels-mcp`. Does not rebuild the UI. |
| `build-fast` | Alias for `build-go`. Skips the UI Vite step; uses whatever `internal/resources/mcp-app.html` is already present. Suitable when the UI has not changed. |
| `install` | Runs `go install ./cmd/archipelago-hotels-mcp`, placing the binary in `$GOPATH/bin`. |

### Run targets

| Target | Description |
|--------|-------------|
| `run-stdio` | Runs `build-fast` then starts the server in `stdio` mode. |
| `dev-http` | Runs `build-fast` then starts the server in HTTP mode at `:9011` with `-verbose` (debug logging). |

### Verification targets

| Target | Description |
|--------|-------------|
| `lint` | Runs `go vet ./...`. |

### Utility targets

| Target | Description |
|--------|-------------|
| `clean` | Removes `bin/`, `ui/dist/`, `ui/node_modules/`, and Go build cache artifacts. |
| `help` | Prints a summary of all available Makefile targets. |

**Examples:**

```sh
# First-time full build (UI + binary)
make build

# Rebuild binary only after a Go change (UI unchanged)
make build-fast

# Local HTTP development session
make dev-http

# Install to GOPATH/bin for use as a system command
make install

# Clean everything before a release build
make clean && make build
```

---

## MCP Tools

Exposed by both transports.

| Tool | Visibility | Description |
|------|-----------|-------------|
| `search_hotels` | public | Search hotels by city, brand, or freetext query; returns hotel list with live or stored prices |
| `recommend_hotel` | public | Rank hotels by vibe, budget, or purpose |
| `find_hotels` | public | Browse all hotels with optional city/brand filter |
| `get_hotel_detail` | app-only (`visibility: app`) | Full hotel detail and room types; intended for the embedded MCP App UI only |

---

## MCP App Resource

The embedded single-page UI is served as an MCP resource when a supporting MCP client requests it.

| Property | Value |
|----------|-------|
| URI | `ui://hotel-dashboard` |
| MIME type | `text/html;profile=mcp-app` |
| Resource domains | `images.archipelagohotels.com` |

---

## Notes

- **Degraded mode:** if the database is unreachable at startup, the server logs a warning and continues. Tool calls requiring DB data return an error response; the process does not crash. This allows the binary to be registered in Claude Desktop before the database is reachable.
- **Flag parsing:** the `http` subcommand uses a minimal custom flag loop (not `flag.FlagSet`). Flags must appear after the `http` subcommand: `archipelago-hotels-mcp http -addr :8080 -verbose`. Order between `-addr` and `-verbose` does not matter.
- **Logging:** structured logs use Go's `log/slog` with a text handler writing to stderr. In stdio mode this keeps the MCP stream on stdout clean.

# Build and Deploy — archipelago-hotels-mcp

This guide walks through building the server from source, configuring the environment, and deploying it in both stdio (Claude Desktop) and HTTP (server) modes.

---

## 1. Prerequisites

| Requirement | Minimum version | Notes |
|-------------|----------------|-------|
| Go | 1.25 | `go version` |
| Node.js | 20 LTS | Required only for UI build (`make build-ui`) |
| npm | bundled with Node.js | `npm --version` |
| MySQL | 5.7 / 8.x | Central DB + per-brand DBs |
| make | any | GNU Make or BSD Make |

Verify before starting:

```bash
go version        # go version go1.25.x ...
node --version    # v20.x.x
npm --version     # 10.x.x
mysql --version   # optional: confirms CLI connectivity
```

---

## 2. Clone and tidy dependencies

```bash
git clone <repo-url> archipelago-hotels-mcp
cd archipelago-hotels-mcp

# Download Go modules
go mod download

# Install Node dependencies (used only for UI build)
cd ui && npm install && cd ..
```

---

## 3. `make build-ui` — build the TypeScript UI

The frontend lives in `ui/src/mcp-app.ts`. Vite bundles it into a single HTML file and the Makefile copies it into the Go embed path.

```bash
make build-ui
```

What this does:

1. Runs `npm install --silent` inside `ui/`.
2. Runs `npm run build` (Vite production build).
3. Copies `ui/dist/index.html` → `internal/resources/mcp-app.html`.

The file at `internal/resources/mcp-app.html` is compiled into the binary via `//go:embed`. You must run `make build-ui` at least once before any Go build, or whenever the TypeScript source changes.

Expected output:

```
=== Building MCP App UI ===
UI built: ui/dist/index.html
UI embedded at: internal/resources/mcp-app.html
```

---

## 4. `make build-go` — compile the Go binary

```bash
make build-go
```

Compiles `./cmd/archipelago-hotels-mcp` and writes the binary to `bin/archipelago-hotels-mcp`.

Expected output:

```
=== Building archipelago-hotels-mcp binary ===
Binary: bin/archipelago-hotels-mcp
```

---

## 5. `make build` — full build (UI + Go)

Run this for a clean, production-ready binary:

```bash
make build
```

Equivalent to running `make build-ui` then `make build-go` in sequence. Prints the binary size at the end:

```
=== Build complete ===
-rwxr-xr-x  1 user staff  12M  bin/archipelago-hotels-mcp
```

---

## 6. `make build-fast` — Go only (skip UI rebuild)

Use this during iterative backend development when the UI has not changed:

```bash
make build-fast
```

Skips the Vite step entirely. The existing `internal/resources/mcp-app.html` is embedded as-is.

> **Note:** If `internal/resources/mcp-app.html` does not exist, `build-fast` will fail at compile time with a missing embed file error. Run `make build-ui` once first.

---

## 7. Environment variables

Set these before running the binary. All variables have sensible defaults for local development.

| Variable | Default | Purpose |
|----------|---------|---------|
| `MYSQL_HOST` | `127.0.0.1` | MySQL host |
| `MYSQL_PORT` | `3306` | MySQL port |
| `MYSQL_USER` | `root` | MySQL username |
| `MYSQL_PASS` | *(empty)* | MySQL password |
| `MYSQL_DB` | `db_archipelagowebsite` | Central catalog database name |
| `DEBUG` | `0` | Set to `1` to enable debug-level logging to stderr |
| `url_image_resizer` | `https://images.archipelagohotels.com/` | Image CDN proxy base URL used by `resizeImageURL()` |

Export for the current shell session:

```bash
export MYSQL_HOST=db.internal.archipelagohotels.com
export MYSQL_PORT=3306
export MYSQL_USER=mcp_reader
export MYSQL_PASS=secret
export MYSQL_DB=db_archipelagowebsite
export DEBUG=0
export url_image_resizer=https://images.archipelagohotels.com/
```

The server starts in **degraded mode** (tools still respond, DB-dependent results are empty) if the database is unavailable at startup. This allows the binary to boot for health checks even when the DB is temporarily unreachable.

---

## 8. Deploying: copy binary and run

### Copy to target host

```bash
# Build on CI or dev machine
make build

# SCP to server
scp bin/archipelago-hotels-mcp user@server:/opt/archipelago-mcp/
```

### Run stdio transport

Used when invoked directly by Claude Desktop or a Pi Agent. The process reads from stdin and writes to stdout; all logging goes to stderr.

```bash
/opt/archipelago-mcp/archipelago-hotels-mcp stdio
```

### Run HTTP transport

Starts the Gin HTTP server on `:9011` (or a custom address).

```bash
# Default port :9011
/opt/archipelago-mcp/archipelago-hotels-mcp http

# Custom port
/opt/archipelago-mcp/archipelago-hotels-mcp http -addr :8080

# Custom port with verbose/debug logging
/opt/archipelago-mcp/archipelago-hotels-mcp http -addr :9011 -verbose
```

HTTP endpoints exposed:

| Method | Path | Description |
|--------|------|-------------|
| `POST` / `GET` | `/mcp` | MCP Streamable HTTP transport |
| `GET` | `/dashboard` | Standalone HTML dashboard |
| `GET` | `/api/hotels` | JSON hotel list (`?city=&brand=`) |
| `GET` | `/api/brands` | JSON brand list |
| `GET` | `/api/regions` | JSON region list |
| `GET` | `/health` | Health check — returns `{ status, version, db }` |

---

## 9. Claude Desktop integration

### Update `claude_desktop_config.json`

Find the config file:

- macOS: `~/Library/Application Support/Claude/claude_desktop_config.json`
- Windows: `%APPDATA%\Claude\claude_desktop_config.json`

Add or update the `mcpServers` entry:

```json
{
  "mcpServers": {
    "archipelago-hotels": {
      "command": "/opt/archipelago-mcp/archipelago-hotels-mcp",
      "args": ["stdio"],
      "env": {
        "MYSQL_HOST": "127.0.0.1",
        "MYSQL_PORT": "3306",
        "MYSQL_USER": "mcp_reader",
        "MYSQL_PASS": "secret",
        "MYSQL_DB": "db_archipelagowebsite",
        "url_image_resizer": "https://images.archipelagohotels.com/"
      }
    }
  }
}
```

### Restart Claude Desktop

Fully quit and reopen the application. Claude Desktop respawns the MCP process on startup.

Verify the server is loaded: open Claude Desktop, check the tool picker — `search_hotels`, `recommend_hotel`, and `find_hotels` should appear.

---

## 10. HTTP server deployment — systemd unit

Create `/etc/systemd/system/archipelago-mcp.service`:

```ini
[Unit]
Description=Archipelago Hotels MCP Server
After=network.target mysql.service
Wants=mysql.service

[Service]
Type=simple
User=archipelago
Group=archipelago
WorkingDirectory=/opt/archipelago-mcp
ExecStart=/opt/archipelago-mcp/archipelago-hotels-mcp http -addr :9011
Restart=on-failure
RestartSec=5

# Environment
Environment=MYSQL_HOST=127.0.0.1
Environment=MYSQL_PORT=3306
Environment=MYSQL_USER=mcp_reader
Environment=MYSQL_PASS=secret
Environment=MYSQL_DB=db_archipelagowebsite
Environment=url_image_resizer=https://images.archipelagohotels.com/
Environment=DEBUG=0

# Logging — captured by journald
StandardOutput=journal
StandardError=journal
SyslogIdentifier=archipelago-mcp

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
sudo systemctl daemon-reload
sudo systemctl enable archipelago-mcp
sudo systemctl start archipelago-mcp
sudo systemctl status archipelago-mcp
```

View live logs:

```bash
journalctl -u archipelago-mcp -f
```

---

## 11. Verifying the deployment

### Health endpoint (HTTP mode)

```bash
curl -s http://localhost:9011/health | jq .
```

Expected response:

```json
{
  "status": "ok",
  "version": "1.0.0",
  "db": "connected"
}
```

If the database is unreachable, `"db"` will reflect a degraded state but `"status"` remains `"ok"` — the server is running.

### Test prompt (stdio or HTTP)

Send a search query through Claude Desktop or any MCP client:

```
Find me a hotel in Bali under IDR 1,000,000 per night
```

Expected: `search_hotels` is called, returns a list of hotels with prices from the SimpleBooking rate fallback chain or stored rates.

### API endpoints (HTTP mode)

```bash
# List all brands
curl -s http://localhost:9011/api/brands | jq .

# Search hotels by city
curl -s "http://localhost:9011/api/hotels?city=Jakarta" | jq .

# Filter by brand
curl -s "http://localhost:9011/api/hotels?brand=aston" | jq .
```

---

## 12. Troubleshooting build failures

### `internal/resources/mcp-app.html: no such file or directory`

The Go embed directive requires the file to exist before compilation.

```bash
make build-ui   # generates the file, then retry
make build-go
```

### `npm install` fails or Vite errors

```bash
cd ui
rm -rf node_modules package-lock.json
npm install
npm run build
```

### Go module errors

```bash
go mod tidy
go mod download
make build-go
```

### MySQL connection refused at startup

The server starts in degraded mode and logs a warning:

```
WARN starting in DEGRADED mode — database unavailable
```

Check connectivity:

```bash
mysql -h "$MYSQL_HOST" -P "$MYSQL_PORT" -u "$MYSQL_USER" -p"$MYSQL_PASS" "$MYSQL_DB" -e "SELECT 1"
```

Verify the environment variables are exported in the shell where you run the binary, or set in the `[Service]` block of the systemd unit.

### Per-brand DB connection errors

Each brand DB connects lazily on first use. If a brand DB is unavailable, rate lookups for that brand fall back to `hotel_starting_price` in the central DB. Check `DEBUG=1` output for the specific brand prefix and error.

### Port already in use (HTTP mode)

```bash
lsof -i :9011        # find the process using the port
kill -9 <PID>        # stop it
# or use a different port:
./bin/archipelago-hotels-mcp http -addr :9012
```

### Binary not found after `make install`

`make install` runs `go install`, placing the binary in `$GOPATH/bin` (or `$HOME/go/bin`). Ensure that directory is on your `PATH`:

```bash
export PATH="$HOME/go/bin:$PATH"
```

---

## Quick reference

```bash
# First-time full build
make build

# Backend-only rebuild (UI unchanged)
make build-fast

# Local HTTP dev server with debug logging
make dev-http

# Install to $GOPATH/bin
make install

# Run on stdio (used by Claude Desktop)
./bin/archipelago-hotels-mcp stdio

# Run HTTP server
./bin/archipelago-hotels-mcp http -addr :9011

# Clean all build artifacts
make clean
```

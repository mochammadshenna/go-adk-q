# Build and Deployment Reference

## Build Targets

### `make build` — full pipeline

Runs `build-ui` then `build-go` in sequence. Use this for any release or before committing a change to the embedded UI.

```
make build
```

Output: `bin/archipelago-hotels-mcp`

---

### `make build-ui` — compile and embed the UI

```
make build-ui
```

Steps performed:

1. `cd ui && npm install --silent` — installs Node dependencies from `ui/package.json`.
2. `npm run build` — runs Vite; output goes to `ui/dist/index.html`.
3. `cp ui/dist/index.html internal/resources/mcp-app.html` — copies the compiled bundle into the Go embed target.

The file `internal/resources/mcp-app.html` is referenced by a `//go:embed` directive in `internal/resources/dashboard.go`. It is compiled into the binary at `go build` time — the UI is self-contained inside the binary and requires no external static file serving.

```go
//go:embed mcp-app.html
var dashboardHTML string
```

Prerequisites: Node.js and npm must be on `PATH`.

---

### `make build-go` — compile the Go binary

```
make build-go
```

Runs:

```
go build -o bin/archipelago-hotels-mcp ./cmd/archipelago-hotels-mcp
```

Output: `bin/archipelago-hotels-mcp`

This target does not rebuild the UI. Use it during backend-only development to iterate quickly (`make build-fast` is an alias that calls only `build-go`).

---

### `make install` — install to GOPATH/bin

```
make install
```

Runs `go install ./cmd/archipelago-hotels-mcp`. The binary is placed in `$(go env GOPATH)/bin/archipelago-hotels-mcp`, making it available on `PATH` if GOPATH/bin is included.

---

### `make lint`

```
make lint
```

Runs `go vet ./...`. No linter configuration file is required.

---

## Version Injection

The server version is a package-level variable in `internal/server/server.go`:

```go
// Version set at build time via -ldflags.
var Version = "dev"
```

Default is `"dev"`. To inject a release version at build time:

```
go build -ldflags "-X github.com/msw/archipelago-hotels-mcp/internal/server.Version=1.2.3" \
    -o bin/archipelago-hotels-mcp ./cmd/archipelago-hotels-mcp
```

The version appears in two places at runtime:

- The MCP server `Implementation.Version` field (reported to clients during the MCP handshake).
- The `/health` endpoint JSON response: `{"version": "1.2.3", ...}`.

---

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `MYSQL_HOST` | `127.0.0.1` | MySQL host |
| `MYSQL_PORT` | `3306` | MySQL port |
| `MYSQL_USER` | `root` | MySQL user |
| `MYSQL_PASS` | _(empty)_ | MySQL password |
| `MYSQL_DB` | `db_archipelagowebsite` | Central database name |
| `url_image_resizer` | `https://images.archipelagohotels.com/` | Image CDN base URL (see [image-pipeline.md](../explanation/image-pipeline.md)) |
| `DEBUG` | _(unset)_ | Set to `1` to enable `slog.LevelDebug` output |

---

## Claude Desktop Configuration

Add the server to `claude_desktop_config.json` under `mcpServers`:

```json
{
  "mcpServers": {
    "archipelago-hotels": {
      "command": "/path/to/archipelago-hotels-mcp",
      "args": ["stdio"],
      "env": {
        "MYSQL_HOST": "127.0.0.1",
        "MYSQL_PORT": "3306",
        "MYSQL_USER": "your_db_user",
        "MYSQL_PASS": "your_db_password",
        "MYSQL_DB": "db_archipelagowebsite"
      }
    }
  }
}
```

If the binary was installed via `make install`, the path is `$(go env GOPATH)/bin/archipelago-hotels-mcp`.

The server starts in stdio mode and communicates exclusively over stdin/stdout. No port is opened. The `url_image_resizer` env variable can be added to override the image CDN base URL; omit it to use the production default.

---

## HTTP Mode

HTTP mode is intended for development, integration testing, and standalone dashboard access. It is not used by Claude Desktop.

```
archipelago-hotels-mcp http
archipelago-hotels-mcp http -addr :8080
archipelago-hotels-mcp http -addr :9011 -verbose
```

Default address: `:9011`

`-verbose` enables Gin's request logger (output to stderr) and sets Gin to debug mode.

### Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `POST /mcp` | MCP Streamable HTTP | MCP protocol endpoint (JSON-RPC over HTTP) |
| `GET /mcp` | MCP Streamable HTTP | SSE stream for server-sent events |
| `GET /dashboard` | HTML | Standalone dashboard UI (same embedded HTML as the MCP App resource) |
| `GET /api/hotels` | JSON | Hotel list; accepts `?city=` and `?brand=` query params |
| `GET /api/brands` | JSON | Sorted list of distinct brand names |
| `GET /api/regions` | JSON | Distinct region/city names |
| `GET /health` | JSON | `{"status":"ok","version":"...","db":true}` |

The `/api/*` endpoints back the standalone `/dashboard` page. They are not part of the MCP protocol surface.

CORS headers (`Access-Control-Allow-Origin: *`) are set on all routes to support the standalone dashboard being opened from a local file or different origin during development.

---

## Reference Architecture

```
cmd/archipelago-hotels-mcp/
  main.go               Entry point; parses args, wires pool + rateSvc, dispatches to stdio or http

internal/
  server/
    server.go           MCP server construction (tools + resource registration), RunStdio, RunHTTP

  tools/
    search.go           search_hotels tool
    dashboard.go        find_hotels tool
    detail.go           get_hotel_detail tool (visibility: app-only)
    recommend.go        recommend_hotel tool

  resources/
    dashboard.go        MCP resource registration (ui://hotel-dashboard)
    mcp-app.html        Embedded UI bundle (generated by make build-ui)

  repository/
    hotel.go            SearchHotels, GetHotelByID, GetThumbnails, resizeImageURL
    repository.go       Pool, Config, ConfigFromEnv, brand DB management

  rate/
    service.go          Rate aggregation, BatchMinRates
    simplebooking.go    SimpleBooking XML rate source

ui/
  src/mcp-app.ts        TypeScript source for MCP App UI
  dist/index.html       Vite build output (gitignored; copied to internal/resources/)

bin/
  archipelago-hotels-mcp  Compiled binary (gitignored)
```

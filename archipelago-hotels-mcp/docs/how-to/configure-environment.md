# How to configure the environment

The server reads all configuration from environment variables at startup. There are no configuration files — set variables in the shell, in Claude Desktop's `env` block, or in a systemd unit.

## Required environment variables

| Variable | Default | Description |
|---|---|---|
| `MYSQL_HOST` | `127.0.0.1` | MySQL server hostname or IP |
| `MYSQL_PORT` | `3306` | MySQL server port |
| `MYSQL_USER` | `root` | MySQL username |
| `MYSQL_PASS` | _(empty)_ | MySQL password |
| `MYSQL_DB` | `db_archipelagowebsite` | Central database name |
| `DEBUG` | _(unset)_ | Set to `1` to enable debug-level logging to stderr |
| `url_image_resizer` | _(unset)_ | Base URL of a custom image CDN/resizer (see below) |
| `SIMPLEBOOKING_ENDPOINT` | `https://xml.simplebooking.it/xmlservice.asmx/HotelAvailRQ` | Override the SimpleBooking XML API endpoint |

If the central database is unreachable at startup, the server continues in **degraded mode** — all hotel queries will fail but the process stays up and logs a warning.

## Claude Desktop stdio mode

Add the server to `~/Library/Application Support/Claude/claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "archipelago-hotels": {
      "command": "/path/to/bin/archipelago-hotels-mcp",
      "args": ["stdio"],
      "env": {
        "MYSQL_HOST": "db.internal.archipelagohotels.com",
        "MYSQL_PORT": "3306",
        "MYSQL_USER": "mcp_reader",
        "MYSQL_PASS": "s3cr3t",
        "MYSQL_DB": "db_archipelagowebsite"
      }
    }
  }
}
```

Restart Claude Desktop after editing this file.

## HTTP mode

Run directly on the default port (9011):

```sh
archipelago-hotels-mcp http
```

Custom port:

```sh
archipelago-hotels-mcp http -addr :8080
```

The HTTP server exposes:

- `POST /mcp` — MCP Streamable HTTP endpoint
- `GET  /mcp` — MCP SSE endpoint
- `GET  /dashboard` — standalone hotel dashboard
- `GET  /api/hotels` — JSON hotel list (supports `?city=` and `?brand=`)
- `GET  /health` — health check (see the debug guide)

## Debug logging

Set `DEBUG=1` to enable `slog.LevelDebug` output on stderr. This shows per-request database queries, brand DB connection events, SimpleBooking call results, and circuit breaker state changes.

```sh
DEBUG=1 archipelago-hotels-mcp stdio
```

In Claude Desktop, add `"DEBUG": "1"` to the `env` block.

## Custom image CDN

Set `url_image_resizer` to a base URL. When set, the server rewrites hotel thumbnail URLs to route through your CDN. The value must be a valid URL with scheme.

```sh
url_image_resizer=https://img.archipelagohotels.com archipelago-hotels-mcp stdio
```

Leave unset to use original image URLs from the database.

## Override the SimpleBooking endpoint

For local testing or staging, point `SIMPLEBOOKING_ENDPOINT` at a mock server:

```sh
SIMPLEBOOKING_ENDPOINT=http://localhost:9999/xmlservice archipelago-hotels-mcp http
```

The circuit breaker still applies: five consecutive failures open the breaker for 120 seconds.

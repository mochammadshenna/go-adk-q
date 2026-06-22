# Environment Variables Reference

All configuration is supplied via environment variables. There are no config files — the binary reads the environment at startup.

## Variable Reference

| Variable | Default | Required | Description |
|---|---|---|---|
| `MYSQL_HOST` | `127.0.0.1` | No | MySQL server hostname or IP address |
| `MYSQL_PORT` | `3306` | No | MySQL server TCP port |
| `MYSQL_USER` | `root` | No | MySQL username for all database connections (central + per-brand) |
| `MYSQL_PASS` | _(empty)_ | **Yes** (production) | MySQL password |
| `MYSQL_DB` | `db_archipelagowebsite` | No | Central database name. Override only if the central DB was renamed. |
| `DEBUG` | `0` | No | Set to `1` to enable debug-level structured logging to stderr. All other values keep log level at INFO. |
| `url_image_resizer` | `https://images.archipelagohotels.com/` | No | Base URL for the Archipelago image resizer service. Used to rewrite brand CDN thumbnail URLs into resized proxied URLs. Must end with `/`. |
| `VERSION` | _(empty)_ | No | Build-time version string. Injected via `-ldflags "-X main.Version=..."` during CI builds. Not read from the environment at runtime — set at link time. |

### Notes

**Single credential set**: `MYSQL_USER` and `MYSQL_PASS` are used for all database connections — both the central `db_archipelagowebsite` database and every per-brand database (e.g. `db_astonwebsite`, `db_favewebsite`). The MySQL user must have `SELECT` privileges on all brand databases.

**Degraded mode**: If the central DB is unreachable at startup the server logs a warning and continues. MCP tools that require the database return an appropriate error to the caller. This allows the binary to start even when the DB is temporarily unavailable.

**SimpleBooking credentials**: There are no `SIMPLEBOOKING_*` environment variables. SimpleBooking credentials (`simplebooking_id`, `simplebooking_user`, `simplebooking_pass`, `xml_user`, `xml_pass`, `hotel_channel`, `sentec_booking_id`) are stored per-hotel in each brand's `tb_hotels` table and fetched at runtime via `GetCredentials()`. The XML provider credentials embedded in the binary are a static provider-level key pair supplied by SimpleBooking, separate from per-hotel credentials.

**Sentec API**: No environment variables are required for the Sentec REST API. When implemented it will use per-hotel credentials from the `credential_booking_engines` table in the brand DB. No hotels currently use Sentec.

---

## Example `.env` File

```dotenv
# MySQL — central and all per-brand databases share one credential set
MYSQL_HOST=db.internal.archipelagohotels.com
MYSQL_PORT=3306
MYSQL_USER=mcp_readonly
MYSQL_PASS=s3cr3t

# Central DB name (default is correct for production)
# MYSQL_DB=db_archipelagowebsite

# Image resizer (leave unset to use the production Archipelago CDN)
# url_image_resizer=https://images.archipelagohotels.com/

# Debug logging (set to 1 during local development)
# DEBUG=1
```

Load it before running:

```bash
set -a && source .env && set +a
./bin/archipelago-hotels-mcp stdio
```

---

## Claude Desktop `mcpServers` Config Block

Stdio transport (recommended for Claude Desktop and Pi Agent):

```json
{
  "mcpServers": {
    "archipelago-hotels": {
      "command": "/path/to/archipelago-hotels-mcp",
      "args": ["stdio"],
      "env": {
        "MYSQL_HOST": "db.internal.archipelagohotels.com",
        "MYSQL_PORT": "3306",
        "MYSQL_USER": "mcp_readonly",
        "MYSQL_PASS": "s3cr3t"
      }
    }
  }
}
```

HTTP transport (useful when the binary runs as a persistent service):

```json
{
  "mcpServers": {
    "archipelago-hotels": {
      "url": "http://localhost:9011/mcp"
    }
  }
}
```

For the HTTP transport start the server separately:

```bash
./bin/archipelago-hotels-mcp http -addr :9011
```

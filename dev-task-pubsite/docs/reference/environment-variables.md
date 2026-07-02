# Environment Variables Reference

dev-task-pubsite is configured entirely through environment variables. There are no config files.

## YouTrack provider

| Variable | Required | Default | Description |
|---|---|---|---|
| `YOUTRACK_URL` | Yes (for yt_* tools) | — | Base URL of your YouTrack instance, e.g. `https://youtrack.yourcompany.com`. Trailing slash is stripped automatically. |
| `YOUTRACK_TOKEN` | Yes (for yt_* tools) | — | Permanent token or bearer token. Permanent tokens start with `perm:`. |

If either `YOUTRACK_URL` or `YOUTRACK_TOKEN` is missing, the YouTrack client is not created and all `yt_*` tools are absent from the MCP server. The server starts without error.

## GitHub provider

| Variable | Required | Default | Description |
|---|---|---|---|
| `GITHUB_TOKEN` | Yes (for gh_* tools) | — | Personal access token, fine-grained token, or installation access token. |
| `GITHUB_OWNER` | No | `""` | Default GitHub owner (org or user) used when a tool call omits the `owner` argument. |

If `GITHUB_TOKEN` is missing, all `gh_*` tools are absent. The server starts without error.

## HTTP server

| Variable | Required | Default | Description |
|---|---|---|---|
| `PORT` | No | `9012` | TCP port for HTTP mode. Used only when the binary is started with the `http` argument. |
| `DEBUG` | No | `""` | Set to `1` or `true` to enable Gin access logging to stderr. In release mode (default) access logs are suppressed. |

## Behaviour when variables are missing

The startup logic in `cmd/dev-task-pubsite/main.go`:

1. Attempts to create the YouTrack client. On failure, logs a warning and continues with `ytClient = nil`.
2. Attempts to create the GitHub client. On failure, logs a warning and continues with `ghClient = nil`.
3. Passes both (possibly nil) clients to `server.New()`.
4. `server.New()` registers tools only for non-nil clients.

This means you can run with zero environment variables and the server still starts — it will have no tools registered but will serve the `ui://task-dashboard` resource and respond to health checks.

## Secrets management

Do not hardcode tokens in source code or commit `.env` files. Recommended approaches:

- **Local development**: `.env` file sourced into the shell, excluded from git via `.gitignore`
- **Docker**: environment variables in `docker-compose.yml` or injected at runtime
- **CI/CD**: GitHub Actions secrets, GitLab CI/CD variables, etc.
- **Production**: vault solutions (HashiCorp Vault, AWS Secrets Manager, etc.) with dynamic token injection

## Example .env file

```bash
# YouTrack
YOUTRACK_URL=https://youtrack.yourcompany.com
YOUTRACK_TOKEN=perm:abc123yourtokenhere

# GitHub
GITHUB_TOKEN=ghp_yourpersonalaccesstoken
GITHUB_OWNER=sentinel-tech

# Server
PORT=9012
DEBUG=0
```

Source with: `source .env` or `export $(cat .env | xargs)`

## Claude Desktop configuration

When using Claude Desktop, set environment variables in the MCP server config entry rather than relying on the shell environment, since Claude Desktop does not inherit the user's shell profile:

```json
{
  "mcpServers": {
    "dev-task-pubsite": {
      "command": "/path/to/binary",
      "args": ["stdio"],
      "env": {
        "YOUTRACK_URL": "...",
        "YOUTRACK_TOKEN": "...",
        "GITHUB_TOKEN": "...",
        "GITHUB_OWNER": "..."
      }
    }
  }
}
```

## Related

- [Configure YouTrack →](../how-to/configure-youtrack.md)
- [Configure GitHub →](../how-to/configure-github.md)
- [Connect to Claude Desktop →](../how-to/connect-claude-desktop.md)

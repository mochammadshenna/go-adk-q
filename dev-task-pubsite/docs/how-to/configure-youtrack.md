# How to Configure YouTrack

This guide shows you how to connect dev-task-pubsite to your YouTrack instance so that the `yt_*` tools work.

## What you need

- A YouTrack Server or YouTrack Cloud instance
- Permission to create permanent tokens in YouTrack
- The base URL of your instance (e.g. `https://youtrack.yourcompany.com`)

---

## Step 1 — Create a permanent token

1. Log in to YouTrack
2. Go to **your profile → Authentication → New token**
3. Give it a name (e.g. `dev-task-pubsite`)
4. Scope: at minimum **YouTrack** with read access; add write access if you want `create_task` to work
5. Copy the token — it starts with `perm:`

> **Security**: treat the token like a password. Never commit it to version control. Use environment variables or a secrets manager.

---

## Step 2 — Set environment variables

```bash
export YOUTRACK_URL=https://youtrack.yourcompany.com
export YOUTRACK_TOKEN=perm:abc123yourtoken
```

For permanent configuration, add these to your shell profile or use a `.env` file (ensure `.env` is in `.gitignore`).

---

## Step 3 — Verify the connection

Start the server in HTTP mode:

```bash
make dev-http
```

Check `http://localhost:9012/health`:

```json
{"status":"ok","youtrack":true,"github":false}
```

`youtrack: true` means the client initialised. Note that this only confirms the client was created from non-empty env vars — it does not make a live API call. The first tool call will reveal any auth errors.

---

## Step 4 — Test a YouTrack tool

Using MCP Inspector or Claude Desktop, call `list_my_tasks`:

```json
{
  "project_id": "PROJ",
  "state": "In Progress"
}
```

Expected response structure:

```json
{
  "tasks": [...],
  "count": 5,
  "providerStatus": {
    "source": "youtrack",
    "degraded": false,
    "staleAge": ""
  }
}
```

If you see `"degraded": true`, the server is returning cached data because a recent request to YouTrack failed. Check the server logs for the underlying error.

---

## Troubleshooting

| Symptom | Likely cause | Fix |
|---|---|---|
| `yt_* tools absent` | Missing env vars | Set `YOUTRACK_URL` and `YOUTRACK_TOKEN` |
| `401 Unauthorized` | Bad token | Regenerate token in YouTrack profile |
| `connection refused` | Wrong URL | Verify `YOUTRACK_URL` is reachable from the server |
| `degraded: true` | YouTrack unreachable | Circuit breaker opened; wait 120s and retry |
| `404 on project` | Wrong project ID | Use the project **shortName** (e.g. `PROJ`), not the full name |

---

## Using YouTrack Cloud

YouTrack Cloud URLs typically look like `https://yourorg.youtrack.cloud`. The same token format and steps apply.

---

## Related

- [All YouTrack tools →](../reference/tools.md#youtrack-tools)
- [Environment variables reference →](../reference/environment-variables.md)
- [Resilience model →](../explanation/resilience-model.md)

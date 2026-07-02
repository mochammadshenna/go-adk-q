# How to Configure GitHub

This guide shows you how to connect dev-task-pubsite to GitHub so that the `gh_*` tools work.

## What you need

- A GitHub account (personal or organization)
- Ability to create a personal access token or GitHub App installation token

---

## Option A — Personal Access Token (classic)

### Step 1 — Create the token

1. Go to [github.com/settings/tokens](https://github.com/settings/tokens)
2. Click **Generate new token (classic)**
3. Name: `dev-task-pubsite`
4. Expiration: choose based on your security policy
5. Scopes needed:

| Scope | Required for |
|---|---|
| `repo` | All `gh_*` tools on private repos |
| `public_repo` | All `gh_*` tools on public repos only |
| `read:org` | `repo_stats` on org repos |

6. Copy the token — it starts with `ghp_`

### Step 2 — Set environment variables

```bash
export GITHUB_TOKEN=ghp_yourtoken
export GITHUB_OWNER=your-org-or-username   # optional default owner
```

---

## Option B — Fine-grained Personal Access Token

1. Go to [github.com/settings/tokens?type=beta](https://github.com/settings/tokens?type=beta)
2. Click **Generate new token**
3. Resource owner: your org or personal account
4. Repository access: **All repositories** or select specific ones
5. Permissions:
   - **Contents**: Read-only (for `commit_diff`)
   - **Issues**: Read and write (for `list_issues`, `create_issue`)
   - **Pull requests**: Read-only (for `list_pull_requests`, `get_pull_request`)
   - **Metadata**: Read-only (required)

Fine-grained tokens are more secure but cannot access organization-level stats on all orgs.

---

## Option C — GitHub App Installation Token

If you're running dev-task-pubsite in a CI/CD or production environment, prefer a GitHub App:

1. Create a GitHub App in your org settings
2. Grant the same permissions as Option B
3. Install the App on the relevant repositories
4. Generate an installation access token and set it as `GITHUB_TOKEN`

Installation tokens expire after 1 hour — rotate them via your deployment infrastructure.

---

## Step 3 — Verify the connection

```bash
make dev-http
```

Check `http://localhost:9012/health`:

```json
{"status":"ok","youtrack":false,"github":true}
```

---

## Step 4 — Test a GitHub tool

Call `repo_stats` with:

```json
{
  "owner": "octocat",
  "repo": "Hello-World"
}
```

Expected:

```json
{
  "stats": {
    "owner": "octocat",
    "repo": "Hello-World",
    "stars": 1924,
    "forks": 2700,
    "openIssues": 1104,
    "openPRs": 3,
    "defaultBranch": "master",
    "language": "C"
  },
  "providerStatus": {"source": "github", "degraded": false}
}
```

---

## Setting a default owner

The `GITHUB_OWNER` env var sets the default owner used when a tool call omits the `owner` argument. This saves you repeating `"owner": "my-org"` on every request.

```bash
export GITHUB_OWNER=sentinel-tech
```

With this set, `list_pull_requests` accepts just `{"repo": "sentec-pms"}`.

---

## Troubleshooting

| Symptom | Likely cause | Fix |
|---|---|---|
| `gh_* tools absent` | Missing `GITHUB_TOKEN` | Set `GITHUB_TOKEN` env var |
| `401 Bad credentials` | Expired or wrong token | Regenerate token |
| `403 Resource not accessible` | Missing scope | Add `repo` scope |
| `404 Not Found` | Wrong owner/repo | Check spelling; private repos need `repo` scope |
| `degraded: true` | GitHub API unreachable | Wait 120s for circuit breaker to reset |

---

## Rate limits

GitHub's REST API allows 5000 requests/hour for authenticated users. dev-task-pubsite caches responses for 5 minutes, which substantially reduces API calls during normal use. If you hit rate limits, the circuit breaker will open and stale cache will be served with `degraded: true`.

---

## Related

- [All GitHub tools →](../reference/tools.md#github-tools)
- [Environment variables reference →](../reference/environment-variables.md)
- [Provider fallback explanation →](../explanation/provider-fallback.md)

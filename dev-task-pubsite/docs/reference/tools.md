# MCP Tools Reference

dev-task-pubsite exposes 10 MCP tools across two providers.

Every tool response includes a `providerStatus` object:

```json
{
  "providerStatus": {
    "source": "youtrack" | "github",
    "degraded": false,
    "staleAge": ""
  }
}
```

When `degraded` is `true`, the data was served from stale cache because the provider was unreachable. `staleAge` shows how old the cached data is (e.g. `"4m32s"`).

---

## YouTrack Tools

### `list_my_tasks`

List tasks from a YouTrack project.

**Input**

| Field | Type | Required | Description |
|---|---|---|---|
| `project_id` | string | yes | Project shortname (e.g. `PROJ`) or ID |
| `sprint` | string | no | Sprint name filter |
| `state` | string | no | State filter (e.g. `In Progress`, `Open`) |
| `assignee` | string | no | Filter by assignee login |
| `limit` | integer | no | Max results (default: 50, max: 200) |

**Output**

```json
{
  "tasks": [
    {
      "id": "PROJ-123",
      "summary": "Fix login bug",
      "state": "In Progress",
      "priority": "Major",
      "type": "Bug",
      "assignee": {"login": "jsmith", "fullName": "Jane Smith"},
      "sprint": "Sprint 14",
      "created": "2026-06-01T10:00:00Z",
      "updated": "2026-06-20T14:30:00Z"
    }
  ],
  "count": 1,
  "providerStatus": {"source": "youtrack", "degraded": false, "staleAge": ""}
}
```

---

### `search_my_tasks`

Search YouTrack using the [YouTrack query language](https://www.jetbrains.com/help/youtrack/server/search-and-filters.html).

**Input**

| Field | Type | Required | Description |
|---|---|---|---|
| `query` | string | yes | YouTrack query string |
| `limit` | integer | no | Max results (default: 50) |

**Example queries**

- `project: PROJ State: {In Progress} Priority: Major`
- `#unresolved assigned to: me`
- `Sprint: {Sprint 14} type: Bug`

**Output**: same shape as `list_my_tasks`.

---

### `get_my_sprint`

Get a sprint summary with state breakdown and task list.

**Input**

| Field | Type | Required | Description |
|---|---|---|---|
| `project_id` | string | yes | Project shortname or ID |
| `sprint_name` | string | no | Sprint name. Omit for the active sprint. |

**Output**

```json
{
  "sprint": {
    "name": "Sprint 14",
    "state": "active",
    "stateBreakdown": {"Open": 5, "In Progress": 8, "Done": 12},
    "tasks": [...]
  },
  "providerStatus": {"source": "youtrack", "degraded": false, "staleAge": ""}
}
```

---

### `create_task`

Create a new task in YouTrack.

**Input**

| Field | Type | Required | Description |
|---|---|---|---|
| `project_id` | string | yes | Project shortname or ID |
| `summary` | string | yes | Task title |
| `description` | string | no | Task body (markdown supported) |
| `type` | string | no | Issue type (e.g. `Bug`, `Feature`, `Task`) |
| `priority` | string | no | Priority name (e.g. `Major`, `Minor`) |

**Output**

```json
{
  "task": {
    "id": "PROJ-456",
    "summary": "New task",
    "state": "Open",
    ...
  },
  "providerStatus": {"source": "youtrack", "degraded": false, "staleAge": ""}
}
```

> **Note**: `create_task` requires your YouTrack token to have **write** scope. Cache is not used for create operations.

---

## GitHub Tools

### `list_pull_requests`

List pull requests for a GitHub repository.

**Input**

| Field | Type | Required | Description |
|---|---|---|---|
| `owner` | string | yes* | GitHub org or user. Omit to use `GITHUB_OWNER` env default. |
| `repo` | string | yes | Repository name |
| `state` | string | no | `open` (default), `closed`, or `all` |

**Output**

```json
{
  "pullRequests": [
    {
      "number": 42,
      "title": "feat: add release tool",
      "state": "open",
      "author": {"login": "jsmith", "fullName": ""},
      "headBranch": "feat/release-tool",
      "baseBranch": "main",
      "isDraft": false,
      "createdAt": "2026-06-15T09:00:00Z",
      "updatedAt": "2026-06-25T11:00:00Z"
    }
  ],
  "count": 1,
  "providerStatus": {"source": "github", "degraded": false, "staleAge": ""}
}
```

---

### `get_pull_request`

Get a single PR with CI status and review state.

**Input**

| Field | Type | Required | Description |
|---|---|---|---|
| `owner` | string | yes* | GitHub org or user |
| `repo` | string | yes | Repository name |
| `number` | integer | yes | Pull request number |

**Output** includes all `list_pull_requests` fields plus:

```json
{
  "pullRequest": {
    ...
    "headSHA": "abc123def456",
    "ciStatus": "success",
    "reviewState": "approved",
    "diffURL": "https://github.com/org/repo/pull/42/files"
  },
  "providerStatus": {"source": "github", "degraded": false, "staleAge": ""}
}
```

`ciStatus` values: `success`, `pending`, `failure`, `error`, `""`

`reviewState` values: `approved`, `changes_requested`, `pending`, `""`

---

### `list_issues`

List GitHub issues. Pull requests are excluded from this list.

**Input**

| Field | Type | Required | Description |
|---|---|---|---|
| `owner` | string | yes* | GitHub org or user |
| `repo` | string | yes | Repository name |
| `state` | string | no | `open` (default), `closed`, or `all` |
| `labels` | string | no | Comma-separated label filter |

**Output**

```json
{
  "issues": [
    {
      "number": 99,
      "title": "Bug: crash on login",
      "state": "open",
      "author": {"login": "auser"},
      "labels": ["bug", "high-priority"],
      "body": "Steps to reproduce...",
      "createdAt": "2026-06-10T08:00:00Z",
      "updatedAt": "2026-06-24T16:00:00Z"
    }
  ],
  "count": 1,
  "providerStatus": {"source": "github", "degraded": false, "staleAge": ""}
}
```

---

### `create_issue`

Create a GitHub issue.

**Input**

| Field | Type | Required | Description |
|---|---|---|---|
| `owner` | string | yes* | GitHub org or user |
| `repo` | string | yes | Repository name |
| `title` | string | yes | Issue title |
| `body` | string | no | Issue body (markdown) |
| `labels` | []string | no | Labels to apply |
| `assignees` | []string | no | GitHub logins to assign |

---

### `repo_stats`

Get repository statistics.

**Input**

| Field | Type | Required | Description |
|---|---|---|---|
| `owner` | string | yes* | GitHub org or user |
| `repo` | string | yes | Repository name |

**Output**

```json
{
  "stats": {
    "owner": "sentinel-tech",
    "repo": "sentec-pms",
    "stars": 42,
    "forks": 7,
    "openIssues": 15,
    "openPRs": 3,
    "defaultBranch": "main",
    "language": "Go",
    "description": "Property Management System"
  },
  "providerStatus": {"source": "github", "degraded": false, "staleAge": ""}
}
```

---

### `commit_diff`

Get a commit with its full diff. Use this to link code changes to YouTrack task IDs found in commit messages.

**Input**

| Field | Type | Required | Description |
|---|---|---|---|
| `owner` | string | yes* | GitHub org or user |
| `repo` | string | yes | Repository name |
| `sha` | string | yes | Commit SHA (full or abbreviated) |

**Output**

```json
{
  "commit": {
    "sha": "abc123def456",
    "message": "fix: PROJ-123 resolve null pointer on login",
    "author": {"login": "jsmith", "fullName": "Jane Smith"},
    "committedAt": "2026-06-20T14:30:00Z",
    "additions": 12,
    "deletions": 3,
    "changedFiles": [
      {
        "filename": "internal/auth/login.go",
        "status": "modified",
        "additions": 12,
        "deletions": 3,
        "patch": "@@ -45,6 +45,18 @@\n ..."
      }
    ]
  },
  "providerStatus": {"source": "github", "degraded": false, "staleAge": ""}
}
```

> **Tip**: Combine with `list_my_tasks` — search for `PROJ-123` in the commit message to surface the associated YouTrack task.

---

## MCP Resource

### `ui://task-dashboard`

MIME type: `text/html;profile=mcp-app`

An interactive developer dashboard rendered inline by Claude Desktop's MCP Apps extension. Shows sprint board, PR timeline, and commit activity. Build the full UI with `make build-ui`.

---

*\* = Required unless `GITHUB_OWNER` env var is set*

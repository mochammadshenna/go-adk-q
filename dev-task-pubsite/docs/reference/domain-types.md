# Domain Types Reference

All types are defined in `internal/domain/task.go`.

## ProviderStatus

Embedded in every tool response. Tells the caller whether data is live or stale.

```go
type ProviderStatus struct {
    Source    string `json:"source"`    // "youtrack" or "github"
    Degraded  bool   `json:"degraded"`  // true = data served from stale cache
    StaleAge  string `json:"staleAge"`  // e.g. "4m32s", empty when not degraded
}
```

When `Degraded` is `true`, the provider's circuit breaker was open or the live request failed. The server returned the most recent cached value. `StaleAge` shows how long ago that cache entry was populated.

---

## Task

Represents a YouTrack issue.

```go
type Task struct {
    ID          string    `json:"id"`           // e.g. "PROJ-123"
    Summary     string    `json:"summary"`
    Description string    `json:"description"`
    State       string    `json:"state"`        // e.g. "In Progress"
    Priority    string    `json:"priority"`     // e.g. "Major"
    Type        string    `json:"type"`         // e.g. "Bug", "Feature"
    Assignee    *Person   `json:"assignee"`
    Sprint      string    `json:"sprint"`       // sprint name from custom fields
    ProjectID   string    `json:"projectId"`
    ProjectName string    `json:"projectName"`
    Created     time.Time `json:"created"`
    Updated     time.Time `json:"updated"`
}
```

`Sprint` is extracted from the YouTrack custom field named `"sprint"`. It is an empty string if not set.

---

## Person

Represents a user referenced from a task or PR.

```go
type Person struct {
    Login    string `json:"login"`
    FullName string `json:"fullName"`
}
```

For YouTrack users, `Login` is the YouTrack login (not email). For GitHub users, `Login` is the GitHub username and `FullName` is typically empty (GitHub API does not return full names on issue/PR endpoints without additional calls).

---

## PullRequest

Represents a GitHub pull request.

```go
type PullRequest struct {
    Number      int       `json:"number"`
    Title       string    `json:"title"`
    State       string    `json:"state"`        // "open", "closed"
    Author      Person    `json:"author"`
    HeadBranch  string    `json:"headBranch"`
    BaseBranch  string    `json:"baseBranch"`
    HeadSHA     string    `json:"headSHA"`
    IsDraft     bool      `json:"isDraft"`
    CIStatus    string    `json:"ciStatus"`     // "success","pending","failure","error",""
    ReviewState string    `json:"reviewState"`  // "approved","changes_requested","pending",""
    DiffURL     string    `json:"diffURL"`
    CreatedAt   time.Time `json:"createdAt"`
    UpdatedAt   time.Time `json:"updatedAt"`
}
```

`CIStatus` comes from GitHub's combined commit status API (`GetCombinedStatus`). It reflects the aggregate of all CI checks on the PR's head commit.

`ReviewState` is derived from the most recent review per reviewer. If any reviewer has requested changes, the state is `"changes_requested"`. If all reviewers approved and none requested changes, the state is `"approved"`.

---

## Issue

Represents a GitHub issue. Pull requests are excluded — the GitHub provider filters them out.

```go
type Issue struct {
    Number    int       `json:"number"`
    Title     string    `json:"title"`
    State     string    `json:"state"`      // "open", "closed"
    Author    Person    `json:"author"`
    Labels    []string  `json:"labels"`
    Body      string    `json:"body"`
    CreatedAt time.Time `json:"createdAt"`
    UpdatedAt time.Time `json:"updatedAt"`
}
```

---

## Sprint

Represents a YouTrack sprint summary.

```go
type Sprint struct {
    Name           string         `json:"name"`
    State          string         `json:"state"`          // "active", "archived", etc.
    StateBreakdown map[string]int `json:"stateBreakdown"` // e.g. {"Open":5,"Done":12}
    Tasks          []Task         `json:"tasks"`
}
```

`StateBreakdown` counts how many tasks are in each state within the sprint. The keys are YouTrack state names.

---

## RepoStats

Represents GitHub repository statistics.

```go
type RepoStats struct {
    Owner         string `json:"owner"`
    Repo          string `json:"repo"`
    Description   string `json:"description"`
    Stars         int    `json:"stars"`
    Forks         int    `json:"forks"`
    OpenIssues    int    `json:"openIssues"`  // excludes PRs
    OpenPRs       int    `json:"openPRs"`
    DefaultBranch string `json:"defaultBranch"`
    Language      string `json:"language"`
}
```

`OpenIssues` is accurate: the GitHub API's `OpenIssuesCount` includes PRs, so the provider subtracts `OpenPRs` to give a true issue count.

---

## CommitDiff

Represents a GitHub commit with its diff.

```go
type CommitDiff struct {
    SHA          string        `json:"sha"`
    Message      string        `json:"message"`
    Author       Person        `json:"author"`
    CommittedAt  time.Time     `json:"committedAt"`
    Additions    int           `json:"additions"`
    Deletions    int           `json:"deletions"`
    ChangedFiles []ChangedFile `json:"changedFiles"`
}
```

---

## ChangedFile

One file within a `CommitDiff`.

```go
type ChangedFile struct {
    Filename  string `json:"filename"`
    Status    string `json:"status"`    // "added","modified","removed","renamed"
    Additions int    `json:"additions"`
    Deletions int    `json:"deletions"`
    Patch     string `json:"patch"`     // unified diff patch
}
```

`Patch` is the raw unified diff text. For binary files or very large diffs, GitHub may omit it (empty string).

---

## Time format

All `time.Time` fields are serialised as RFC 3339 strings in JSON output, e.g.:

```
"2026-06-20T14:30:00Z"
```

YouTrack returns timestamps as Unix milliseconds internally; the `yt` provider converts these to `time.Time` before returning domain types.

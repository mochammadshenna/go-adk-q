# How to Add a New MCP Tool

This guide walks you through adding a new tool to dev-task-pubsite. Adding a tool requires touching three files: the provider client, an optional domain type, and a tools registration file.

## Overview of the pattern

Every tool follows this structure:

```
provider client method → domain type → tool handler → Register function → server wiring
```

The MCP SDK tool handler signature is:

```go
func(ctx context.Context, req *mcp.CallToolRequest, input InputT) (*mcp.CallToolResult, OutputT, error)
```

The SDK auto-serializes `OutputT` to JSON. Always embed `domain.ProviderStatus` in `OutputT`.

---

## Example: adding `gh_list_releases`

We'll add a tool that lists GitHub releases for a repository.

### Step 1 — Add a domain type (if needed)

Edit `internal/domain/task.go` and add:

```go
// Release represents a GitHub release.
type Release struct {
    TagName     string    `json:"tagName"`
    Name        string    `json:"name"`
    Draft       bool      `json:"draft"`
    Prerelease  bool      `json:"prerelease"`
    PublishedAt time.Time `json:"publishedAt"`
    URL         string    `json:"url"`
}
```

### Step 2 — Add a method to the provider client

Edit `internal/providers/gh/client.go` and add:

```go
// ListReleases returns GitHub releases for a repository.
func (c *Client) ListReleases(ctx context.Context, owner, repo string) ([]domain.Release, domain.ProviderStatus, error) {
    cacheKey := fmt.Sprintf("releases:%s/%s", owner, repo)
    status := domain.ProviderStatus{Source: "github"}

    if got := c.cache.Get(cacheKey); got.Found {
        releases := got.Value.([]domain.Release)
        if got.Stale {
            status.Degraded = true
            status.StaleAge = got.Age.Round(time.Second).String()
        }
        return releases, status, nil
    }

    if !c.cb.allow() {
        status.Degraded = true
        return nil, status, fmt.Errorf("github circuit breaker open")
    }

    ghReleases, _, err := c.client.Repositories.ListReleases(ctx, owner, repo, &github.ListOptions{PerPage: 30})
    if err != nil {
        c.cb.failure()
        return nil, status, fmt.Errorf("github list releases: %w", err)
    }
    c.cb.success()

    releases := make([]domain.Release, 0, len(ghReleases))
    for _, r := range ghReleases {
        releases = append(releases, domain.Release{
            TagName:    strVal(r.TagName),
            Name:       strVal(r.Name),
            Draft:      boolVal(r.Draft),
            Prerelease: boolVal(r.Prerelease),
            PublishedAt: timePtrVal(r.PublishedAt),
            URL:        strVal(r.HTMLURL),
        })
    }

    c.cache.Set(cacheKey, releases)
    return releases, status, nil
}
```

### Step 3 — Create the tool handler file

Create `internal/tools/releases.go`:

```go
package tools

import (
    "context"
    "fmt"
    "log/slog"

    mcp "github.com/modelcontextprotocol/go-sdk/mcp"
    "github.com/msw/dev-task-pubsite/internal/domain"
    "github.com/msw/dev-task-pubsite/internal/providers/gh"
    "github.com/msw/dev-task-pubsite/internal/resources"
)

type listReleasesArgs struct {
    Owner string `json:"owner"`
    Repo  string `json:"repo"`
}

type releasesResult struct {
    Releases []domain.Release      `json:"releases"`
    Count    int                   `json:"count"`
    Status   domain.ProviderStatus `json:"providerStatus"`
}

// RegisterListReleases registers gh_list_releases.
func RegisterListReleases(s *mcp.Server, ghClient *gh.Client) {
    mcp.AddTool(s, &mcp.Tool{
        Name:        "gh_list_releases",
        Description: "List GitHub releases for a repository, including drafts and prereleases.",
        Meta: mcp.Meta{
            "ui": map[string]any{"resourceUri": resources.ResourceURI},
        },
        InputSchema: map[string]any{
            "type": "object",
            "properties": map[string]any{
                "owner": map[string]any{"type": "string", "description": "GitHub owner"},
                "repo":  map[string]any{"type": "string", "description": "Repository name"},
            },
            "required": []string{"owner", "repo"},
        },
    }, listReleasesHandler(ghClient))
}

func listReleasesHandler(ghClient *gh.Client) func(context.Context, *mcp.CallToolRequest, listReleasesArgs) (*mcp.CallToolResult, releasesResult, error) {
    return func(ctx context.Context, _ *mcp.CallToolRequest, args listReleasesArgs) (res *mcp.CallToolResult, out releasesResult, err error) {
        defer func() {
            if r := recover(); r != nil {
                slog.Error("gh_list_releases panic", "recover", r)
                err = fmt.Errorf("internal error: %v", r)
            }
        }()
        releases, status, apiErr := ghClient.ListReleases(ctx, args.Owner, args.Repo)
        if apiErr != nil {
            return nil, releasesResult{Status: status}, apiErr
        }
        return nil, releasesResult{Releases: releases, Count: len(releases), Status: status}, nil
    }
}
```

### Step 4 — Register the tool in server.go

Edit `internal/server/server.go`. Inside `New()`, after the existing GitHub tool registrations:

```go
if ghClient != nil {
    // ... existing tools ...
    tools.RegisterListReleases(s, ghClient)  // add this line
}
```

### Step 5 — Build and verify

```bash
go build ./...
go vet ./...
make dev-http
```

Check the tool appears in MCP Inspector under **Tools**.

---

## Checklist

- [ ] Domain type added (if new fields needed)
- [ ] Provider client method handles cache hit, stale cache, and circuit breaker
- [ ] Tool handler has `defer recover()` panic safety
- [ ] Tool handler returns 3 values: `(*mcp.CallToolResult, OutputT, error)`
- [ ] `OutputT` embeds `domain.ProviderStatus`
- [ ] `mcp.AddTool` called with `Meta["ui"]["resourceUri"]` set
- [ ] `RegisterXxx` called from `server.New()`
- [ ] `go build ./...` and `go vet ./...` both pass

---

## Related

- [Tools reference →](../reference/tools.md)
- [Domain types reference →](../reference/domain-types.md)
- [Architecture explanation →](../explanation/architecture.md)

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

// ---- GitHub: repo_stats -------------------------------------------------

type repoStatsArgs struct {
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
}

type repoStatsResult struct {
	Stats  *domain.RepoStats     `json:"stats"`
	Status domain.ProviderStatus `json:"providerStatus"`
}

// RegisterRepoStats registers repo_stats.
func RegisterRepoStats(s *mcp.Server, ghClient *gh.Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "repo_stats",
		Description: "Get GitHub repository statistics: stars, forks, open issues, open PRs, default branch, and primary language.",
		Meta: mcp.Meta{
			"ui": map[string]any{"resourceUri": resources.ResourceURI},
		},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"owner": map[string]any{"type": "string", "description": "GitHub owner (user or org)"},
				"repo":  map[string]any{"type": "string", "description": "Repository name"},
			},
			"required": []string{"owner", "repo"},
		},
	}, repoStatsHandler(ghClient))
}

func repoStatsHandler(ghClient *gh.Client) func(context.Context, *mcp.CallToolRequest, repoStatsArgs) (*mcp.CallToolResult, repoStatsResult, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, args repoStatsArgs) (res *mcp.CallToolResult, out repoStatsResult, err error) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("repo_stats panic", "recover", r)
				err = fmt.Errorf("internal error: %v", r)
			}
		}()
		stats, status, apiErr := ghClient.GetRepoStats(ctx, args.Owner, args.Repo)
		if apiErr != nil {
			return nil, repoStatsResult{Status: status}, apiErr
		}
		return nil, repoStatsResult{Stats: stats, Status: status}, nil
	}
}

// ---- GitHub: commit_diff ------------------------------------------------

type commitDiffArgs struct {
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
	SHA   string `json:"sha"`
}

type commitDiffResult struct {
	Diff   *domain.CommitDiff    `json:"commit"`
	Status domain.ProviderStatus `json:"providerStatus"`
}

// RegisterCommitDiff registers commit_diff.
func RegisterCommitDiff(s *mcp.Server, ghClient *gh.Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "commit_diff",
		Description: "Get a GitHub commit with its full diff: changed files, additions/deletions, and patch content. Use to link code changes to YouTrack tasks.",
		Meta: mcp.Meta{
			"ui": map[string]any{"resourceUri": resources.ResourceURI},
		},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"owner": map[string]any{"type": "string", "description": "GitHub owner"},
				"repo":  map[string]any{"type": "string", "description": "Repository name"},
				"sha":   map[string]any{"type": "string", "description": "Commit SHA (full or abbreviated)"},
			},
			"required": []string{"owner", "repo", "sha"},
		},
	}, commitDiffHandler(ghClient))
}

func commitDiffHandler(ghClient *gh.Client) func(context.Context, *mcp.CallToolRequest, commitDiffArgs) (*mcp.CallToolResult, commitDiffResult, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, args commitDiffArgs) (res *mcp.CallToolResult, out commitDiffResult, err error) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("commit_diff panic", "recover", r)
				err = fmt.Errorf("internal error: %v", r)
			}
		}()
		diff, status, apiErr := ghClient.GetCommitDiff(ctx, args.Owner, args.Repo, args.SHA)
		if apiErr != nil {
			return nil, commitDiffResult{Status: status}, apiErr
		}
		return nil, commitDiffResult{Diff: diff, Status: status}, nil
	}
}

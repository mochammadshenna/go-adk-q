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

// ---- GitHub: list_pull_requests ---------------------------------------------------

type listPRsArgs struct {
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
	State string `json:"state"`
}

type prListResult struct {
	PRs    []domain.PullRequest  `json:"pullRequests"`
	Count  int                   `json:"count"`
	Status domain.ProviderStatus `json:"providerStatus"`
}

// RegisterListPRs registers list_pull_requests.
func RegisterListPRs(s *mcp.Server, ghClient *gh.Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_pull_requests",
		Description: "List pull requests for a GitHub repository. Returns PR number, title, state, author, branch names, and draft status.",
		Meta: mcp.Meta{
			"ui": map[string]any{"resourceUri": resources.ResourceURI},
		},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"owner": map[string]any{"type": "string", "description": "GitHub owner (user or org)"},
				"repo":  map[string]any{"type": "string", "description": "Repository name"},
				"state": map[string]any{"type": "string", "description": "open (default), closed, or all"},
			},
			"required": []string{"owner", "repo"},
		},
	}, listPRsHandler(ghClient))
}

func listPRsHandler(ghClient *gh.Client) func(context.Context, *mcp.CallToolRequest, listPRsArgs) (*mcp.CallToolResult, prListResult, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, args listPRsArgs) (res *mcp.CallToolResult, out prListResult, err error) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("list_pull_requests panic", "recover", r)
				err = fmt.Errorf("internal error: %v", r)
			}
		}()
		prs, status, apiErr := ghClient.ListPRs(ctx, args.Owner, args.Repo, gh.PRListOpts{State: args.State})
		if apiErr != nil {
			return nil, prListResult{Status: status}, apiErr
		}
		return nil, prListResult{PRs: prs, Count: len(prs), Status: status}, nil
	}
}

// ---- GitHub: get_pull_request -----------------------------------------------------

type getPRArgs struct {
	Owner  string `json:"owner"`
	Repo   string `json:"repo"`
	Number int    `json:"number"`
}

type prResult struct {
	PR     *domain.PullRequest   `json:"pullRequest"`
	Status domain.ProviderStatus `json:"providerStatus"`
}

// RegisterGetPR registers get_pull_request.
func RegisterGetPR(s *mcp.Server, ghClient *gh.Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_pull_request",
		Description: "Get a single GitHub pull request with CI status, review state, and diff URL. Includes head branch SHA and reviewer decisions.",
		Meta: mcp.Meta{
			"ui": map[string]any{"resourceUri": resources.ResourceURI},
		},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"owner":  map[string]any{"type": "string", "description": "GitHub owner"},
				"repo":   map[string]any{"type": "string", "description": "Repository name"},
				"number": map[string]any{"type": "integer", "description": "Pull request number"},
			},
			"required": []string{"owner", "repo", "number"},
		},
	}, getPRHandler(ghClient))
}

func getPRHandler(ghClient *gh.Client) func(context.Context, *mcp.CallToolRequest, getPRArgs) (*mcp.CallToolResult, prResult, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, args getPRArgs) (res *mcp.CallToolResult, out prResult, err error) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("get_pull_request panic", "recover", r)
				err = fmt.Errorf("internal error: %v", r)
			}
		}()
		if args.Number <= 0 {
			return nil, prResult{}, fmt.Errorf("number must be a positive integer")
		}
		pr, status, apiErr := ghClient.GetPR(ctx, args.Owner, args.Repo, args.Number)
		if apiErr != nil {
			return nil, prResult{Status: status}, apiErr
		}
		return nil, prResult{PR: pr, Status: status}, nil
	}
}

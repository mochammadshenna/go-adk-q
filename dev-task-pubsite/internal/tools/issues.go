package tools

import (
	"context"
	"fmt"
	"log/slog"

	mcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/msw/dev-task-pubsite/internal/domain"
	"github.com/msw/dev-task-pubsite/internal/providers/gh"
	"github.com/msw/dev-task-pubsite/internal/providers/yt"
	"github.com/msw/dev-task-pubsite/internal/resources"
)

// ---- YouTrack: list_my_tasks -----------------------------------------------

type listTasksArgs struct {
	Project string `json:"project"`
	Sprint  string `json:"sprint"`
	State   string `json:"state"`
	Limit   int    `json:"limit"`
}

type taskListResult struct {
	Tasks  []domain.Task         `json:"tasks"`
	Count  int                   `json:"count"`
	Status domain.ProviderStatus `json:"providerStatus"`
}

// RegisterListTasks registers list_my_tasks.
func RegisterListTasks(s *mcp.Server, ytClient *yt.Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_my_tasks",
		Description: "List YouTrack tasks/issues. Filter by project, sprint, or state. Returns tasks with resilience: serves stale cache when YouTrack is unavailable.",
		Meta: mcp.Meta{
			"ui": map[string]any{"resourceUri": resources.ResourceURI},
		},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project": map[string]any{"type": "string", "description": "YouTrack project shortname (e.g. PROJ)"},
				"sprint":  map[string]any{"type": "string", "description": "Sprint name. Use 'Active sprint' for the current sprint."},
				"state":   map[string]any{"type": "string", "description": "Issue state filter (e.g. 'In Progress', 'Open')"},
				"limit":   map[string]any{"type": "integer", "description": "Max results (1-200, default 50)"},
			},
		},
	}, listTasksHandler(ytClient))
}

func listTasksHandler(ytClient *yt.Client) func(context.Context, *mcp.CallToolRequest, listTasksArgs) (*mcp.CallToolResult, taskListResult, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, args listTasksArgs) (res *mcp.CallToolResult, out taskListResult, err error) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("list_my_tasks panic", "recover", r)
				err = fmt.Errorf("internal error: %v", r)
			}
		}()
		tasks, status, apiErr := ytClient.ListTasks(ctx, yt.TaskListOpts{
			Project: args.Project,
			Sprint:  args.Sprint,
			State:   args.State,
			Limit:   args.Limit,
		})
		if apiErr != nil {
			return nil, taskListResult{Status: status}, apiErr
		}
		return nil, taskListResult{Tasks: tasks, Count: len(tasks), Status: status}, nil
	}
}

// ---- YouTrack: search_my_tasks ---------------------------------------------

type searchTasksArgs struct {
	Query string `json:"query"`
}

type taskSearchResult struct {
	Tasks  []domain.Task         `json:"tasks"`
	Count  int                   `json:"count"`
	Query  string                `json:"query"`
	Status domain.ProviderStatus `json:"providerStatus"`
}

// RegisterSearchTasks registers search_my_tasks.
func RegisterSearchTasks(s *mcp.Server, ytClient *yt.Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "search_my_tasks",
		Description: "Search YouTrack tasks using YouTrack query language. Supports filters like 'Assignee: me', 'Priority: Critical', '#tag'. Returns matching tasks.",
		Meta: mcp.Meta{
			"ui": map[string]any{"resourceUri": resources.ResourceURI},
		},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "YouTrack query (e.g. 'project: PROJ Assignee: john Priority: Critical')"},
			},
			"required": []string{"query"},
		},
	}, searchTasksHandler(ytClient))
}

func searchTasksHandler(ytClient *yt.Client) func(context.Context, *mcp.CallToolRequest, searchTasksArgs) (*mcp.CallToolResult, taskSearchResult, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, args searchTasksArgs) (res *mcp.CallToolResult, out taskSearchResult, err error) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("search_my_tasks panic", "recover", r)
				err = fmt.Errorf("internal error: %v", r)
			}
		}()
		if args.Query == "" {
			return nil, taskSearchResult{}, fmt.Errorf("query is required")
		}
		tasks, status, apiErr := ytClient.SearchTasks(ctx, args.Query)
		if apiErr != nil {
			return nil, taskSearchResult{Query: args.Query, Status: status}, apiErr
		}
		return nil, taskSearchResult{Tasks: tasks, Count: len(tasks), Query: args.Query, Status: status}, nil
	}
}

// ---- YouTrack: create_task ----------------------------------------------

type createTaskArgs struct {
	ProjectID   string `json:"project_id"`
	Summary     string `json:"summary"`
	Description string `json:"description"`
	Priority    string `json:"priority"`
	Type        string `json:"type"`
}

type taskResult struct {
	Task   *domain.Task          `json:"task"`
	Status domain.ProviderStatus `json:"providerStatus"`
}

// RegisterCreateTask registers create_task.
func RegisterCreateTask(s *mcp.Server, ytClient *yt.Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "create_task",
		Description: "Create a new task/issue in YouTrack. Requires project_id and summary. Returns the created task with its assigned ID.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id":  map[string]any{"type": "string", "description": "YouTrack project ID or shortname"},
				"summary":     map[string]any{"type": "string", "description": "Task summary/title"},
				"description": map[string]any{"type": "string", "description": "Task description (Markdown supported)"},
				"priority":    map[string]any{"type": "string", "description": "Priority: Normal, Minor, Major, Critical, Show-stopper"},
				"type":        map[string]any{"type": "string", "description": "Issue type: Task, Bug, Feature, Improvement"},
			},
			"required": []string{"project_id", "summary"},
		},
	}, createTaskHandler(ytClient))
}

func createTaskHandler(ytClient *yt.Client) func(context.Context, *mcp.CallToolRequest, createTaskArgs) (*mcp.CallToolResult, taskResult, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, args createTaskArgs) (res *mcp.CallToolResult, out taskResult, err error) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("create_task panic", "recover", r)
				err = fmt.Errorf("internal error: %v", r)
			}
		}()
		task, status, apiErr := ytClient.CreateTask(ctx, yt.CreateTaskReq{
			ProjectID:   args.ProjectID,
			Summary:     args.Summary,
			Description: args.Description,
			Priority:    args.Priority,
			Type:        args.Type,
		})
		if apiErr != nil {
			return nil, taskResult{Status: status}, apiErr
		}
		return nil, taskResult{Task: task, Status: status}, nil
	}
}

// ---- GitHub: list_issues ------------------------------------------------

type listIssuesArgs struct {
	Owner  string   `json:"owner"`
	Repo   string   `json:"repo"`
	State  string   `json:"state"`
	Labels []string `json:"labels"`
}

type issueListResult struct {
	Issues []domain.Issue        `json:"issues"`
	Count  int                   `json:"count"`
	Status domain.ProviderStatus `json:"providerStatus"`
}

// RegisterListIssues registers list_issues.
func RegisterListIssues(s *mcp.Server, ghClient *gh.Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_issues",
		Description: "List GitHub issues for a repository. Filters by state and labels. Skips pull requests (use list_pull_requests for PRs).",
		Meta: mcp.Meta{
			"ui": map[string]any{"resourceUri": resources.ResourceURI},
		},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"owner":  map[string]any{"type": "string", "description": "GitHub owner (user or org)"},
				"repo":   map[string]any{"type": "string", "description": "Repository name"},
				"state":  map[string]any{"type": "string", "description": "open (default), closed, or all"},
				"labels": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Filter by labels"},
			},
			"required": []string{"owner", "repo"},
		},
	}, listIssuesHandler(ghClient))
}

func listIssuesHandler(ghClient *gh.Client) func(context.Context, *mcp.CallToolRequest, listIssuesArgs) (*mcp.CallToolResult, issueListResult, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, args listIssuesArgs) (res *mcp.CallToolResult, out issueListResult, err error) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("list_issues panic", "recover", r)
				err = fmt.Errorf("internal error: %v", r)
			}
		}()
		issues, status, apiErr := ghClient.ListIssues(ctx, args.Owner, args.Repo, gh.IssueListOpts{
			State:  args.State,
			Labels: args.Labels,
		})
		if apiErr != nil {
			return nil, issueListResult{Status: status}, apiErr
		}
		return nil, issueListResult{Issues: issues, Count: len(issues), Status: status}, nil
	}
}

// ---- GitHub: create_issue -----------------------------------------------

type createIssueArgs struct {
	Owner    string   `json:"owner"`
	Repo     string   `json:"repo"`
	Title    string   `json:"title"`
	Body     string   `json:"body"`
	Labels   []string `json:"labels"`
	Assignee string   `json:"assignee"`
}

type issueResult struct {
	Issue  *domain.Issue         `json:"issue"`
	Status domain.ProviderStatus `json:"providerStatus"`
}

// RegisterCreateIssue registers create_issue.
func RegisterCreateIssue(s *mcp.Server, ghClient *gh.Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "create_issue",
		Description: "Create a new GitHub issue. Returns the created issue with its number and URL.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"owner":    map[string]any{"type": "string", "description": "GitHub repository owner"},
				"repo":     map[string]any{"type": "string", "description": "Repository name"},
				"title":    map[string]any{"type": "string", "description": "Issue title"},
				"body":     map[string]any{"type": "string", "description": "Issue body (Markdown)"},
				"labels":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Labels to apply"},
				"assignee": map[string]any{"type": "string", "description": "GitHub login of assignee"},
			},
			"required": []string{"owner", "repo", "title"},
		},
	}, createIssueHandler(ghClient))
}

func createIssueHandler(ghClient *gh.Client) func(context.Context, *mcp.CallToolRequest, createIssueArgs) (*mcp.CallToolResult, issueResult, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, args createIssueArgs) (res *mcp.CallToolResult, out issueResult, err error) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("create_issue panic", "recover", r)
				err = fmt.Errorf("internal error: %v", r)
			}
		}()
		issue, status, apiErr := ghClient.CreateIssue(ctx, args.Owner, args.Repo, gh.CreateIssueReq{
			Title:    args.Title,
			Body:     args.Body,
			Labels:   args.Labels,
			Assignee: args.Assignee,
		})
		if apiErr != nil {
			return nil, issueResult{Status: status}, apiErr
		}
		return nil, issueResult{Issue: issue, Status: status}, nil
	}
}

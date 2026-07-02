package tools

import (
	"context"
	"fmt"
	"log/slog"

	mcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/msw/dev-task-pubsite/internal/domain"
	"github.com/msw/dev-task-pubsite/internal/providers/yt"
	"github.com/msw/dev-task-pubsite/internal/resources"
)

// ---- YouTrack: get_my_sprint -----------------------------------------------

type getSprintArgs struct {
	ProjectID  string `json:"project_id"`
	SprintName string `json:"sprint_name"`
}

type sprintResult struct {
	Sprint *domain.Sprint        `json:"sprint"`
	Status domain.ProviderStatus `json:"providerStatus"`
}

// RegisterGetSprint registers get_my_sprint.
func RegisterGetSprint(s *mcp.Server, ytClient *yt.Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_my_sprint",
		Description: "Get a YouTrack sprint summary: issue count, state breakdown (open/in-progress/done), and task list. Defaults to the active sprint when sprint_name is omitted.",
		Meta: mcp.Meta{
			"ui": map[string]any{"resourceUri": resources.ResourceURI},
		},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id":  map[string]any{"type": "string", "description": "YouTrack project shortname (e.g. PROJ) or ID"},
				"sprint_name": map[string]any{"type": "string", "description": "Sprint name. Omit for the active sprint."},
			},
			"required": []string{"project_id"},
		},
	}, getSprintHandler(ytClient))
}

func getSprintHandler(ytClient *yt.Client) func(context.Context, *mcp.CallToolRequest, getSprintArgs) (*mcp.CallToolResult, sprintResult, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, args getSprintArgs) (res *mcp.CallToolResult, out sprintResult, err error) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("get_my_sprint panic", "recover", r)
				err = fmt.Errorf("internal error: %v", r)
			}
		}()
		sprint, status, apiErr := ytClient.GetSprintSummary(ctx, args.ProjectID, args.SprintName)
		if apiErr != nil {
			return nil, sprintResult{Status: status}, apiErr
		}
		return nil, sprintResult{Sprint: sprint, Status: status}, nil
	}
}

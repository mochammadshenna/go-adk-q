package resources

import (
	"context"
	"embed"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/msw/dino-mcp/internal/tools"
)

//go:embed dashboard_ui.html
var uiFS embed.FS

// RegisterDashboardResource registers the ui://dino-dashboard/mcp-app.html
// resource that serves the interactive dinosaur dashboard HTML.
func RegisterDashboardResource(s *mcp.Server, logger *slog.Logger) {
	s.AddResource(&mcp.Resource{
		URI:         tools.ResourceURI,
		Name:        "Dino Dashboard",
		Description: "Interactive dinosaur dashboard with filterable cards, fun facts, and visual styling.",
		MIMEType:    tools.ResourceMIMEType,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		htmlData, err := uiFS.ReadFile("dashboard_ui.html")
		if err != nil {
			logger.Error("failed to read embedded dashboard UI", "error", err)
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{
				{
					URI:      req.Params.URI,
					MIMEType: tools.ResourceMIMEType,
					Text:     string(htmlData),
				},
			},
		}, nil
	})
}

// DashboardHTML returns the embedded dashboard UI bytes for the standalone
// HTTP fallback endpoints (/dashboard, /dashboard/*any) in the server package.
func DashboardHTML() ([]byte, error) {
	return uiFS.ReadFile("dashboard_ui.html")
}

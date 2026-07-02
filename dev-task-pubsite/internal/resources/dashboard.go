package resources

import (
	"context"
	_ "embed"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ResourceURI is the MCP Apps resource URI for the task dashboard.
const ResourceURI = "ui://task-dashboard"

//go:embed mcp-app.html
var dashboardHTML string

// DashboardHTML returns the embedded dashboard HTML.
func DashboardHTML() string { return dashboardHTML }

// RegisterDashboardResource registers the task dashboard HTML resource with
// the MCP Apps MIME type so Claude Desktop renders it as an inline app.
func RegisterDashboardResource(s *mcp.Server) {
	s.AddResource(&mcp.Resource{
		URI:         ResourceURI,
		Name:        "Task Dashboard",
		Description: "Interactive developer dashboard: YouTrack sprint board, GitHub PR timeline, and commit activity.",
		MIMEType:    "text/html;profile=mcp-app",
		Meta: mcp.Meta{
			"ui": map[string]any{
				"csp": map[string]any{
					"resourceDomains": []string{},
				},
			},
		},
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{
				{
					URI:      ResourceURI,
					MIMEType: "text/html;profile=mcp-app",
					Text:     dashboardHTML,
				},
			},
		}, nil
	})
}

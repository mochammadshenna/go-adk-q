package resources

import (
	"context"
	_ "embed"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ResourceURI is the URI used for the interactive dashboard MCP App resource.
const ResourceURI = "ui://hotel-dashboard"

//go:embed mcp-app.html
var dashboardHTML string

// DashboardHTML returns the embedded dashboard HTML.
func DashboardHTML() string { return dashboardHTML }

// RegisterDashboardResource registers the hotel dashboard HTML resource with MCP Apps MIME type.
func RegisterDashboardResource(s *mcp.Server) {
	s.AddResource(&mcp.Resource{
		URI:         ResourceURI,
		Name:        "Archipelago Hotels Dashboard",
		Description: "Interactive hotel dashboard showing all Archipelago properties with filters by city, brand, and room rate cards.",
		MIMEType:    "text/html;profile=mcp-app",
		Meta: mcp.Meta{
			"ui": map[string]any{
				"resourceDomains": []string{"images.archipelagohotels.com"},
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

func init() {}

package tools

import (
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"runtime"

	mcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/msw/archipelago-hotels-mcp/internal/resources"
)

// RegisterOpenURL registers the open_booking_url app-only tool.
// The MCP server (outside the Claude Desktop sandbox) opens the URL in the
// system browser using os/exec, bypassing Electron iframe restrictions.
func RegisterOpenURL(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "open_booking_url",
		Description: "Open a hotel booking URL in the system browser.",
		Meta: mcp.Meta{
			"ui": map[string]any{
				"resourceUri": resources.ResourceURI,
				"visibility":  []string{"app"},
			},
		},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{"type": "string", "description": "The booking URL to open."},
			},
			"required": []string{"url"},
		},
	}, openURLHandler())
}

type openURLArgs struct {
	URL string `json:"url"`
}

func openURLHandler() func(context.Context, *mcp.CallToolRequest, openURLArgs) (*mcp.CallToolResult, map[string]any, error) {
	return func(_ context.Context, _ *mcp.CallToolRequest, args openURLArgs) (*mcp.CallToolResult, map[string]any, error) {
		u, err := url.Parse(args.URL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
			return nil, nil, fmt.Errorf("invalid URL")
		}

		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			cmd = exec.Command("open", args.URL)
		case "linux":
			cmd = exec.Command("xdg-open", args.URL)
		case "windows":
			cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", args.URL)
		default:
			return nil, nil, fmt.Errorf("unsupported OS: %s", runtime.GOOS)
		}

		if err := cmd.Start(); err != nil {
			return nil, nil, fmt.Errorf("open failed: %w", err)
		}
		return nil, map[string]any{"ok": true}, nil
	}
}

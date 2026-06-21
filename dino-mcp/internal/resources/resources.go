// Package resources defines all MCP resource registrations for the dino-mcp
// server. Each resource is an addressable URI that returns content — in this
// project, the sole resource is the MCP App dashboard HTML.
//
// The resources package:
//   - Embeds the Vite-built dashboard HTML via //go:embed
//   - Registers the resource handler on the MCP server
//   - Exports DashboardHTML() for the standalone HTTP fallback in server/
package resources

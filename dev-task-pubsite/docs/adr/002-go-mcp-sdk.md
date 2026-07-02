# ADR 002 — Use modelcontextprotocol/go-sdk as MCP Transport

**Status**: Accepted  
**Date**: 2026-06-26  
**Deciders**: Senior Principal Architecture

---

## Context

dev-task-pubsite is an MCP server written in Go. The Model Context Protocol specification defines the wire format and protocol semantics, but does not mandate a specific library. Options were:

1. **Implement MCP protocol from scratch** — parse JSON-RPC, handle protocol lifecycle manually
2. **Use `modelcontextprotocol/go-sdk`** — the official Go SDK maintained by Anthropic/MCP
3. **Wrap the YouTrack MCP server** — proxy requests through the JetBrains-provided YouTrack MCP server

---

## Decision

Use **`github.com/modelcontextprotocol/go-sdk`** at the latest stable version (`v1.6.1` at decision time).

---

## Rationale

### Official SDK follows spec changes automatically

The MCP specification is still evolving. Using the official SDK means protocol-level concerns (session management, capability negotiation, JSON-RPC framing, streamable HTTP) are handled by the maintainers of the spec. Breaking spec changes will be absorbed by the SDK before they reach application code.

### Generic `mcp.AddTool` enables type-safe handlers

The SDK provides a generic package-level function:

```go
mcp.AddTool(s, tool, func(ctx, req, input T) (*mcp.CallToolResult, OutputT, error))
```

This pattern:
- Eliminates manual JSON unmarshalling in every handler
- Provides compile-time type checking on handler signatures
- Auto-serialises `OutputT` to JSON without reflection boilerplate

Rolling this from scratch would require either unsafe type assertions or significant reflection code.

### MCP Apps (ext-apps) resource registration

The SDK's `s.AddResource()` method supports the `text/html;profile=mcp-app` MIME type used by the ext-apps protocol. Implementing this from scratch would require careful adherence to the resource content format and URI scheme.

### Gin wraps the SDK's `StreamableHTTPHandler`

The SDK provides `mcp.NewStreamableHTTPHandler` for HTTP transport. Wrapping this in Gin gives us CORS middleware and structured routing without re-implementing the MCP HTTP protocol layer. The two concerns are cleanly separated: Gin handles HTTP concerns, the SDK handles MCP concerns.

---

## Decision details

```go
// Tool registration (generic, type-safe)
mcp.AddTool(s, &mcp.Tool{...}, handlerFunc)

// Resource registration (server method)
s.AddResource(&mcp.Resource{
    URI:      "ui://task-dashboard",
    MIMEType: "text/html;profile=mcp-app",
}, handlerFunc)

// HTTP transport
handler := mcp.NewStreamableHTTPHandler(serverFunc, &mcp.StreamableHTTPOptions{})

// Stdio transport
s.Run(ctx, &mcp.StdioTransport{})
```

---

## Consequences

### Positive

- Protocol correctness guaranteed by maintainers of the spec
- Type-safe handlers reduce runtime errors
- MCP Apps protocol supported out of the box
- Stdio and HTTP transports available without additional code

### Negative

- Go SDK API is still maturing — minor breaking changes between minor versions are possible
- Generic functions (`mcp.AddTool`) require Go 1.18+ generics support
- SDK documentation is sparse; reading the source and existing MCP server examples is necessary

### Neutral

- The SDK version is pinned in `go.mod`; updates are a deliberate action (`go get`)
- If the SDK's approach proves limiting, individual handlers can be replaced with lower-level JSON-RPC handling while keeping the rest of the SDK

---

## Alternatives considered

### Implement MCP from scratch (rejected)

Would require implementing: JSON-RPC 2.0 framing, MCP capability negotiation, session management, streamable HTTP transport, stdio transport. Significant ongoing maintenance burden as the spec evolves. The risk of subtle protocol incompatibilities with MCP clients is high.

### Wrap the YouTrack MCP server (rejected)

JetBrains provides a YouTrack MCP server. Proxying through it would add a dependency on a third-party process, complicate deployment, limit control over error handling, and prevent integrating GitHub data into a unified server. The requirement was a direct REST API client, not a proxy.

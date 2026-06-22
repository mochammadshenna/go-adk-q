# ADR-0001: Go + modelcontextprotocol/go-sdk v1.6.1 for MCP Server Implementation

**Date:** 2025-06
**Status:** Accepted

---

## Context

Archipelago Hotels & Resorts required an MCP (Model Context Protocol) server to expose hotel search, recommendation, and detail data to AI assistants such as Claude Desktop. The server needed to:

- Integrate with a central MySQL catalog (`db_archipelagowebsite`) and eight per-brand databases.
- Support both stdio transport (Claude Desktop child-process model) and Streamable HTTP transport (browser agents, CI pipelines).
- Be deployable as a self-contained binary with no runtime dependency on the target host.
- Fit within the parent `go-adk-q` monorepo, which is written entirely in Go.

Three implementation languages were evaluated: Node.js/TypeScript, Python, and Go.

---

## Decision

The server is implemented in **Go 1.25** using the official **`github.com/modelcontextprotocol/go-sdk` v1.6.1** SDK.

Go was selected as the implementation language and the official go-sdk as the MCP library.

---

## Alternatives Considered

### Node.js / TypeScript (`@modelcontextprotocol/sdk`)

- **Reason considered:** The TypeScript SDK is the most mature MCP implementation; large community; broad example coverage.
- **Reason rejected:** Requires a Node.js runtime on every deployment target. Produces a directory tree of `node_modules` rather than a single binary. Mismatches the Go toolchain used across the parent monorepo and the Sentec Tech engineering organization.

### Python (`mcp` PyPI package)

- **Reason considered:** Python MCP SDK exists; familiar to data-science adjacent teams.
- **Reason rejected:** Org-wide preference for Go over Python is explicit (see `CLAUDE.md` organization instructions). Dynamic typing increases refactor risk at scale. Startup latency is higher and single-binary distribution requires extra tooling (PyInstaller, etc.).

### Raw JSON-RPC 2.0 (no SDK)

- **Reason considered:** Full control, no external dependency.
- **Reason rejected:** Re-implementing the MCP protocol from scratch is error-prone and forfeits future SDK improvements (e.g., new transport negotiation, structured content, MCP Apps extension).

---

## Consequences

### Positive

- **Single binary deployment.** `go build` produces one self-contained executable (~23 MB including the embedded UI HTML). No runtime installation required on the host.
- **Performance.** Go's compiled execution and lightweight goroutine model handle concurrent hotel-search requests and the bounded rate-fetch worker pool efficiently.
- **Type safety.** `mcp.AddTool[TArgs, TResult]()` generics enforce compile-time correctness on tool input/output schemas, catching mismatches before runtime.
- **Monorepo consistency.** The server uses the same Go version, module proxy, and CI pipeline as all other services in `go-adk-q`.
- **Official SDK.** Using `modelcontextprotocol/go-sdk` ensures compatibility with Claude Desktop's MCP negotiation, structured-content responses, and the MCP Apps (`ext-apps`) protocol extension used by the embedded UI.

### Negative

- **Larger codebase than a JS prototype.** Explicit type declarations, error handling, and interface definitions produce more lines of code than equivalent TypeScript or Python.
- **Smaller ecosystem.** The Go MCP SDK has fewer community examples than the TypeScript SDK; some features (e.g., MCP Apps) required reading the SDK source directly.
- **Go generics learning curve.** Team members unfamiliar with Go generics need onboarding before contributing tool handlers safely.

---

## Implementation Reference

```go
// internal/server/server.go — server creation
s := mcp.NewServer(
    &mcp.Implementation{Name: "archipelago-hotels-mcp", Version: Version},
    &mcp.ServerOptions{
        Instructions: `You have access to the full Archipelago Hotels & Resorts catalog...`,
        Logger:       logger,
    },
)

// Type-safe tool registration
mcp.AddTool(s, &mcp.Tool{
    Name:        "search_hotels",
    Description: "Search Archipelago hotels by city, brand, or query",
    InputSchema: mcp.MustSchema(SearchInput{}),
}, searchHandler(pool, rateSvc))

// stdio transport — Claude Desktop child-process model
s.Run(ctx, &mcp.StdioTransport{})

// Streamable HTTP transport — browser agents, CI
handler := mcp.NewStreamableHTTPHandler(func(...) *mcp.Server { return s }, options)
router.POST("/mcp", gin.WrapH(handler))
router.GET("/mcp",  gin.WrapH(handler))
```

Key files affected by this decision:

| File | Role |
|------|------|
| `cmd/archipelago-hotels-mcp/main.go` | Entrypoint; transport dispatch (stdio vs HTTP) |
| `internal/server/server.go` | MCP server wiring, tool + resource registration, HTTP routes |
| `internal/tools/search.go` | `search_hotels` handler (type-safe TArgs pattern) |
| `internal/tools/recommend.go` | `recommend_hotel` handler |
| `internal/tools/dashboard.go` | `find_hotels` handler |
| `internal/tools/detail.go` | `get_hotel_detail` handler (app-only visibility) |
| `internal/resources/dashboard.go` | MCP resource registration for embedded UI |

# MEMORY.md — dino-mcp Session Memory & Decision Log

> **Persistent context for AI agents.** Records key decisions, architecture choices, resolved issues, and development history.

---

## §1 Active Decisions

| ID | Decision | Date | Rationale |
|----|----------|------|-----------|
| D-001 | **Go over TypeScript for server** | 2026-06-19 | `go-adk-q` parent project uses Go; MCP Go SDK v1.6.1 available; avoids Node.js runtime dependency |
| D-002 | **Gin over standard http.ServeMux** | 2026-06-21 | Gin provides built-in CORS middleware, recovery, logging; cleaner route grouping; `gin.WrapH()` integrates MCP handler seamlessly |
| D-003 | **Vite + ext-apps SDK for UI** | 2026-06-21 | Official MCP Apps SDK provides `App` class with correct `ui/initialize` protocol; `vite-plugin-singlefile` bundles all assets into one embeddable HTML |
| D-004 | **`//go:embed` for UI delivery** | 2026-06-19 | Self-contained binary; no external file serving; production-grade deployment |
| D-005 | **Streamable HTTP only (no SSE)** | 2026-06-21 | SSE transport removed from MCP spec 2025-11-25; Go SDK confirms: "SSE transports have been removed from the spec" |
| D-006 | **DisableLocalhostProtection: true** | 2026-06-21 | Required for Cloudflare Tunnel; DNS rebinding protection blocks non-localhost Host headers |
| D-007 | **CORS with `*` origin** | 2026-06-21 | MCP Inspector runs at localhost:5173; tunnel at trycloudflare.com; wildcard is safe for dev server |
| D-008 | **Stdio for Claude Desktop, HTTP for Inspector** | 2026-06-21 | Claude Desktop optimizes for stdio; MCP Inspector requires HTTP; both transports coexist |
| D-009 | **Standalone `/dashboard` + `/api/dinosaurs`** | 2026-06-21 | Fallback for clients without MCP Apps support; enables browser-based testing |
| D-010 | **`_meta.ui.resourceUri` nested format** | 2026-06-21 | Flat `"ui/resourceUri"` deprecated by MCP Apps spec; must use `{ui: {resourceUri: "..."}}` |

---

## §2 Resolved Issues

| Issue | Status | Resolution |
|-------|--------|------------|
| SSE transport deprecated | ✅ Closed | Switched to Streamable HTTP only; removed `sse` subcommand |
| CORS OPTIONS returns 405 | ✅ Fixed | Added `corsMiddleware()` with `OPTIONS → 200` handler |
| Cloudflare Tunnel 403 Forbidden | ✅ Fixed | Set `DisableLocalhostProtection: true` in `StreamableHTTPOptions` |
| MCP-UI wrong protocol | ✅ Fixed | Replaced MCP-UI community protocol with `@modelcontextprotocol/ext-apps` SDK |
| `extract_sid` picks up CORS header | ✅ Fixed | Updated regex to match `^Mcp-Session-Id:` precisely |
| `run-sse` target orphaned | ✅ Fixed | Removed from Makefile `.PHONY` |
| Module path mismatch | ✅ Fixed | Set `module github.com/msw/dino-mcp` in go.mod |

---

## §3 Architecture Decisions

### Why Gin over `http.ServeMux`?

**Context**: Original impl used `http.ServeMux` with manual CORS and logging middleware.

**Decision**: Switch to Gin v1.12.0.

**Rationale**:
- Built-in `gin.Recovery()` catches panics
- `gin.Logger()` / custom logging middleware
- `gin.WrapH()` wraps any `http.Handler` (like `StreamableHTTPHandler`)
- Clean route grouping with `r.Any()` for catch-all patterns
- JSON responses for `/health` and `/api/dinosaurs` are one-liners

### Why ext-apps SDK over manual postMessage?

**Context**: Original HTML used MCP-UI community protocol (`iframe-ready` messages).

**Decision**: Use `@modelcontextprotocol/ext-apps` App class with Vite bundling.

**Rationale**:
- Official protocol with `ui/initialize` handshake
- Handles `ui/notifications/tool-input`, `ui/notifications/tool-result`
- Provides `onhostcontextchanged` for theme adaptation
- `callServerTool()` for View-triggered tool calls
- TypeScript SDK with type safety
- Vite + singlefile plugin produces embeddable HTML

### Why two transport modes?

**Context**: MCP supports stdio and Streamable HTTP.

**Decision**: Support both, each optimized for its use case.

**Rationale**:
- **stdio**: Claude Desktop requires it; lower latency; no port management
- **Streamable HTTP**: MCP Inspector, curl testing, Cloudflare Tunnel, remote access

---

## §4 Development Log

### 2026-06-19: Initial scaffold
- Created project skeleton
- Implemented dino_think, dino_ask, dino_dashboard tools
- Built embedded standalone HTML dashboard

### 2026-06-21: Major refactor
- Switched from SSE to Streamable HTTP
- Replaced `http.ServeMux` with Gin
- Added CORS middleware
- Fixed Cloudflare Tunnel compatibility
- Replaced MCP-UI protocol with official ext-apps SDK
- Created Vite build pipeline for UI
- Wrote 7 integration tests
- Set default port to :9010

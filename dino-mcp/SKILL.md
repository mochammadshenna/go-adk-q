# SKILL.md — dino-mcp Development Skills

> **Agent skills for building, testing, and debugging dino-mcp.** Each skill is a focused capability an AI agent can invoke when working on this project.

---

## Skill: Build & Run

```yaml
name: build-and-run
description: Build and run dino-mcp in any transport mode
trigger: make build, make dev-http, make run-stdio, make run-http
```

```bash
# Quick build (Go only, uses existing UI)
make build-fast

# Full build (Vite UI + Go)
make build

# Run in dev mode (verbose)
make dev-http          # → localhost:9010

# Run for Claude Desktop
make run-stdio
```

---

## Skill: Test

```yaml
name: test
description: Run integration tests and verify protocol compliance
trigger: make test, test_mcp.sh
```

```bash
# Full integration suite
make test

# Verify MIME type
# Expected: text/html;profile=mcp-app for ui:// resources
```

**What tests verify**:
1. Initialize handshake returns session ID
2. `notifications/initialized` accepted
3. `tools/list` includes `dino_dashboard` with `_meta.ui.resourceUri`
4. `tools/call dino_think` returns structured content
5. `tools/call dino_dashboard` returns filtered dinosaurs
6. `resources/read` returns HTML with `text/html;profile=mcp-app`
7. `tools/call dino_ask` returns Q&A

---

## Skill: Debug

```yaml
name: debug
description: Debug server issues, MCP protocol problems, and UI rendering
trigger: server error, 400/403/5xx, UI not rendering
```

### MCP Protocol Debugging

```bash
# Start verbose server
make dev-http

# Test initialize
curl -s -X POST http://localhost:9010/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}'
```

### Common Errors

| Error | Cause | Fix |
|-------|-------|-----|
| `400 Bad Request` | Missing/wrong Accept header | Send `Accept: application/json, text/event-stream` |
| `403 Forbidden: invalid Host header` | DNS rebinding protection | Set `DisableLocalhostProtection: true` |
| `405 Method Not Allowed` | No CORS handler for OPTIONS | Add `corsMiddleware()` before route |
| SSE not working | SSE transport removed from spec | Use Streamable HTTP (POST /mcp) |
| UI not showing in Claude Desktop | Wrong postMessage protocol | Use `App` from `@modelcontextprotocol/ext-apps` |

---

## Skill: Tunnel

```yaml
name: tunnel
description: Expose local server via Cloudflare Tunnel for remote testing
trigger: cloudflared, trycloudflare, tunnel
```

```bash
# Start server + tunnel in one command
make run-tunnel

# Or manually:
./bin/dino-mcp http -addr :9010 &
cloudflared tunnel --url http://localhost:9010

# Test the tunnel
curl -X POST https://<tunnel-id>.trycloudflare.com/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize",...}'
```

---

## Skill: UI Development

```yaml
name: ui-development
description: Develop and build the MCP App View using ext-apps SDK
trigger: ui/, dashboard_ui.html, mcp-app.ts
```

```bash
# Build UI (requires npm)
make build-ui

# The built HTML goes to ui/dist/mcp-app.html
# Then gets copied to internal/resources/dashboard_ui.html
```

**Key files**:
- `ui/src/mcp-app.ts` — App class, postMessage protocol, card rendering
- `ui/vite.config.ts` — `vite-plugin-singlefile` for inline bundles
- `internal/resources/dashboard_ui.html` — Embedded output (also served standalone)

---

## Skill: Add New Tool

```yaml
name: add-tool
description: Add a new MCP tool to the server
trigger: new tool, new feature, add capability
```

**Steps**:
1. Define args struct and result struct in `server.go` (tool types section)
2. Create `register*Tool(s *mcp.Server)` function (new file or in `tools.go`)
3. Call `mcp.AddTool(s, &mcp.Tool{...}, handler)`
4. Call the register function in `server.New()`
5. Add tool name to the `Instructions` string
6. Add to logger.Info() tools list
7. Update `test_mcp.sh` with a new test section
8. Run `make test`

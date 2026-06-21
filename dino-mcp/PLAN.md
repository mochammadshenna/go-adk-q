# PLAN.md — dino-mcp Development Roadmap

> **Strategic plan for dino-mcp development.** Current status, milestones, and future directions.

---

## Current Status: MVP ✅

The Minimum Viable Product is complete and verified:

- ✅ Go server with 3 tools
- ✅ MCP Apps protocol (ext-apps SDK)
- ✅ Streamable HTTP transport
- ✅ stdio transport for Claude Desktop
- ✅ Standalone dashboard UI
- ✅ API endpoint for browser fallback
- ✅ CORS + Tunnel support
- ✅ 7 integration tests passing
- ✅ Comprehensive documentation

---

## Phase 2: Production Hardening

### Milestone 2.1 — Observability
- [ ] Structured logging with request IDs
- [ ] Prometheus metrics endpoint
- [ ] OpenTelemetry tracing
- [ ] Server-sent events for dashboard live updates

### Milestone 2.2 — Security
- [ ] Rate limiting middleware
- [ ] Configurable CORS origins (not `*`)
- [ ] Session timeout cleanup
- [ ] CSP headers on standalone dashboard

### Milestone 2.3 — Testing
- [ ] Go unit tests (not just bash integration)
- [ ] GitHub Actions CI pipeline
- [ ] Load testing with k6
- [ ] MCP Inspector regression suite

---

## Phase 3: Enhanced UI

### Milestone 3.1 — Dashboard Features
- [ ] Real-time dinosaur data from external API
- [ ] Export to image/PDF
- [ ] Compare mode (side-by-side dinosaurs)
- [ ] Timeline visualization

### Milestone 3.2 — Additional Views
- [ ] Dino quiz app (MCP App)
- [ ] Fossil map viewer
- [ ] Evolutionary tree visualizer

### Milestone 3.3 — Customization
- [ ] Theme toggle (dark/light)
- [ ] User preferences persistence
- [ ] Locale/i18n support

---

## Phase 4: Ecosystem

### Milestone 4.1 — MCP Integration
- [ ] Integration with `go-adk-q` parent project
- [ ] ADK agent tools via MCP client
- [ ] Multi-server dashboard aggregator

### Milestone 4.2 — Distribution
- [ ] Homebrew formula
- [ ] Docker image
- [ ] GitHub release with pre-built binaries
- [ ] npm package for the UI

---

## Phase 5: Advanced Features

### Milestone 5.1 — Multi-Model
- [ ] Support streaming tool inputs
- [ ] Progressive rendering with partial data
- [ ] Cached responses for repeated queries

### Milestone 5.2 — Collaboration
- [ ] Multi-user dashboard sessions
- [ ] Shared filter state across clients
- [ ] Real-time sync via WebSocket

---

## Known Technical Debt

| Item | Priority | Notes |
|------|----------|-------|
| `dino_ask` uses hardcoded answers | High | Should integrate with knowledge graph or LLM |
| Test script is bash, not Go | Medium | Fragile; should migrate to Go test framework |
| No graceful shutdown for tunnel | Low | `make run-tunnel` needs ctrl+c to stop |
| MIME type not validated in test | Medium | Test checks string match, not actual MIME parsing |
| No CI/CD pipeline | Medium | Manual testing only |

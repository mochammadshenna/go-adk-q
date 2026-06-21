# Contributing to dino-mcp

First off, thank you for considering contributing to dino-mcp! 🦕

## Code of Conduct

This project and everyone participating in it is governed by the [Code of Conduct](CODE_CONDUCT.md). By participating, you are expected to uphold this code.

## How Can I Contribute?

### Reporting Bugs

Before creating a bug report, check the existing issues. When creating a bug report, include:

- **Summary**: Clear, concise description
- **Steps to reproduce**: Exact steps, commands used
- **Expected behavior**: What should happen
- **Actual behavior**: What actually happens
- **Environment**: Go version, OS, MCP client version
- **Logs**: Server verbose output, client logs

### Suggesting Enhancements

Enhancement suggestions are tracked as GitHub issues. When creating one:

- **Use a clear title** describing the suggestion
- **Provide a step-by-step description** of the suggested enhancement
- **Describe the current behavior** and why it's problematic
- **Explain why this enhancement would be useful** to most users

### Pull Requests

1. **Fork the repo** and create your branch from `main`
2. **Run tests** before committing: `make test`
3. **Run lint**: `make lint`
4. **Update documentation** if you change behavior
5. **Update test script** if you add/modify tools
6. **Issue the pull request**

### Development Workflow

```bash
# 1. Set up
git clone <repo-url>
cd dino-mcp

# 2. Build & test
make build-fast
make test

# 3. Create feature branch
git checkout -b feature/my-feature

# 4. Make changes
# 5. Build & test
make build-fast && make test

# 6. Commit (imperative present tense)
git commit -m "Add Triassic period filter to dashboard"

# 7. Push and PR
git push origin feature/my-feature
```

## Style Guides

### Go Code

- Follow standard `go fmt` formatting (`make lint` enforces this)
- Use `PascalCase` for exported types, `camelCase` for unexported
- Error handling: return errors, don't panic (except in constructors)
- Comments: `// PackageName` doc comment on every package
- Tool handlers: typed args structs with `json` + `jsonschema` tags

### TypeScript/UI Code

- Follow TypeScript strict mode
- Use `@modelcontextprotocol/ext-apps` App class
- Prefer `fetch()` for standalone API calls
- PostMessage: use the official protocol (`ui/initialize`, not `iframe-ready`)

### Documentation

- Follow Diátaxis framework (Tutorials, How-to, Reference, Explanation)
- Use Mermaid.js for diagrams
- Update `llms.txt` and `llms-full.txt` for any file changes
- Update `AGENTS.md` for any architecture changes

## Project Structure Rules

- **Composition root** → `internal/server/server.go` (imports tools + resources)
- **Tool handlers** → `internal/tools/` (think.go, ask.go, dashboard.go)
- **Resource handlers** → `internal/resources/` (dashboard.go, dashboard_ui.html)
- **All UI source** → `ui/src/`
- **CLI entry point** → `cmd/dino-mcp/main.go`
- **Documentation** → `docs/` (Diátaxis structure)
- **Architecture decisions** → `docs/adr/ADR-NNNN-title.md`
- **Tests** → `test_mcp.sh` (bash integration)

## Commit Convention

Use imperative present tense:

- `Add dinosaur quiz MCP App`
- `Fix CORS preflight for OPTIONS requests`
- `Refactor CORS middleware to use Gin`
- `Update docs with ADR-0012`
- `Remove deprecated SSE transport`

## Questions?

Open a [GitHub Discussion](https://github.com/msw/dino-mcp/discussions) or reach out to the maintainers.

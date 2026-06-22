# How to update the hotel dashboard UI

The interactive dashboard is a single-file web app built with Vite and embedded directly into the Go binary using `//go:embed`. Updating it requires three steps: edit the source, rebuild the UI, then recompile Go.

## File layout

```
ui/
  src/mcp-app.ts          <- edit this (TypeScript source)
  index.html              <- Vite entry point
  dist/index.html         <- build output (generated, do not edit)

internal/resources/
  mcp-app.html            <- embedded into binary (copied from ui/dist/index.html)
  dashboard.go            <- Go file that embeds mcp-app.html with //go:embed
```

`ui/dist/` is the Vite build output. `internal/resources/mcp-app.html` is the file that actually gets embedded into the binary. The `make build-ui` target builds Vite and then copies the result.

## Step 1: Edit the source

Open `ui/src/mcp-app.ts` and make your changes. The dashboard is a self-contained TypeScript app — all CSS and assets must be inlined because the MCP Apps content security policy blocks external resources.

## Step 2: Rebuild the UI

```sh
make build-ui
```

This runs:

1. `npm install` in `ui/`
2. `npm run build` (Vite bundles and inlines everything into `ui/dist/index.html`)
3. `cp ui/dist/index.html internal/resources/mcp-app.html`

After this step, `internal/resources/mcp-app.html` contains the updated, self-contained HTML.

## Step 3: Recompile the Go binary

```sh
make build-go
```

This recompiles `cmd/archipelago-hotels-mcp/main.go` and all packages. Because `internal/resources/mcp-app.html` changed, Go's embed directive picks up the new content and bakes it into the binary.

To do both steps in one command:

```sh
make build
```

## Step 4: Restart Claude Desktop

Claude Desktop must be fully restarted to pick up the new binary. The MCP server process is launched once at startup — a running Claude Desktop continues using the old binary until restarted.

Quit from the menu bar (not just closing the window), then reopen.

## Verify the change

In Claude Desktop, ask it to open the hotel dashboard. The UI panel should reflect your changes. If it does not, confirm the binary path in `claude_desktop_config.json` points to `bin/archipelago-hotels-mcp` and not a separately installed copy.

In HTTP mode you can verify without Claude Desktop:

```sh
make dev-http
# then open http://localhost:9011/dashboard in a browser
```

## Quick iteration during development

If you only need to test the UI in a browser without the full MCP tooling, use the HTTP standalone dashboard:

```sh
make dev-http
```

This starts the server with `-verbose` on port 9011. The `/dashboard` route serves `internal/resources/mcp-app.html` directly as static HTML, so you can iterate on `make build-ui && make dev-http` without restarting Claude Desktop each time.

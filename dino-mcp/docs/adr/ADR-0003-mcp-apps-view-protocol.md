# ADR-0003: MCP Apps View Protocol

**File:** `ui/src/mcp-app.ts`
**SDK:** `@modelcontextprotocol/ext-apps` ^1.7.0
**Server SDK equivalent:** Go MCP SDK (`mcp.AddTool` + `s.AddResource`)
**MIME type:** `text/html;profile=mcp-app`

---

## Standard ext-apps SDK Usage

The dashboard UI uses the official `@modelcontextprotocol/ext-apps` SDK. The standard lifecycle follows this pattern:

### 1. Create App Instance

```typescript
const app = new App({ name: "Dino Dashboard", version: "0.1.0" });
```

### 2. Register Handlers (BEFORE connect)

Handlers **must** be registered before calling `app.connect()` to avoid missing events:

```typescript
// Tool input received from host (arguments + structured data)
app.ontoolinput = (params) => { /* render input data */ };

// Tool result received from host (final structured data)
app.ontoolresult = (result) => { /* render result data */ };

// Tool call was cancelled by host
app.ontoolcancelled = (params) => { /* handle cancellation */ };

// Host context changed (theme, fonts, safe areas, display modes)
app.onhostcontextchanged = (ctx) => {
  if (ctx.theme) applyDocumentTheme(ctx.theme);
  if (ctx.styles?.variables) applyHostStyleVariables(ctx.styles.variables);
  if (ctx.styles?.css?.fonts) applyHostFonts(ctx.styles.css.fonts);
  if (ctx.safeAreaInsets) { /* apply padding */ }
};

// App is being torn down — return state to host
app.onteardown = async () => { /* cleanup */ return {}; };

// Error handler
app.onerror = console.error;
```

### 3. Connect to Host

```typescript
app.connect().then(() => {
  const ctx = app.getHostContext();
  if (ctx) { /* apply initial host context */ }
});
```

`app.connect()` auto-detects whether the app is running inside an iframe and uses `PostMessageTransport` automatically. Explicit transport construction (`new PostMessageTransport()`) is supported but not required.

### 4. Interact with Host

```typescript
// Call a server tool from the View
const result = await app.callServerTool({ name: "dino_think", arguments: {} });

// Send a message to the conversation
await app.sendMessage({ role: "user", content: [{ type: "text", text: "..." }] });

// Send a log entry to the host
await app.sendLog({ level: "info", data: "User clicked filter" });

// Request host to open a URL
await app.openLink({ url: "https://example.com" });
```

---

## Server-Side Registration (Go MCP SDK)

On the Go server side, the MCP Apps pattern uses two MCP primitives:

### Tool Registration with `_meta.ui.resourceUri`

`internal/tools/dashboard.go` line 64:

```go
mcp.AddTool(s, &mcp.Tool{
    Name: "dino_dashboard",
    // ...
    Meta: mcp.Meta{
        "ui": map[string]any{
            "resourceUri": "ui://dino-dashboard/mcp-app.html",
        },
    },
}, handler)
```

This is the Go equivalent of the TypeScript SDK's `registerAppTool()`. The `_meta.ui.resourceUri` field tells the host which resource contains the interactive HTML UI.

### Resource Registration with MCP Apps MIME Type

`internal/tools/dashboard.go` line 35:

```go
s.AddResource(&mcp.Resource{
    URI:      "ui://dino-dashboard/mcp-app.html",
    Name:     "Dino Dashboard",
    MIMEType: "text/html;profile=mcp-app",
}, handler)
```

This is the Go equivalent of the TypeScript SDK's `registerAppResource()`. The MIME type `text/html;profile=mcp-app` identifies this as an MCP App resource.

---

## PostMessage Transport

`PostMessageTransport` wraps `window.parent.postMessage()` and `window.addEventListener('message', ...)` to create a bidirectional JSON-RPC channel between the iframe and the host.

**View → Host messages:**
| Method | Purpose |
|---------|---------|
| `ui/initialize` | Register capabilities, request host context |
| `ui/notifications/initialized` | Confirm ready for tool data |
| `tools/call` | View invokes a server tool |
| `sendMessage` | View sends a message to the conversation |
| `sendLog` | View sends a log entry |
| `openLink` | View requests opening a URL |
| `ui/request-display-mode` | Request fullscreen/inline mode |

**Host → View messages:**
| Method | Purpose |
|---------|---------|
| `ui/initialize` (result) | Host context: theme, capabilities, fonts, safe areas |
| `ui/notifications/tool-input` | Tool arguments (partial or complete) |
| `ui/notifications/tool-result` | Final structured data |
| `ui/notifications/tool-cancelled` | Tool call was cancelled |
| `ui/notifications/host-context-changed` | Theme/font/layout changes |

---

## Resource URI Scheme

MCP App resources use the `ui://` URI scheme:

| Component | Value | Purpose |
|-----------|-------|---------|
| Scheme | `ui://` | Identifies MCP App resources |
| Authority | `dino-dashboard` | Unique app identifier |
| Path | `/mcp-app.html` | Resource path within the app |
| Full URI | `ui://dino-dashboard/mcp-app.html` | Used in `_meta.ui.resourceUri` and `resources/read` |

---

## UI Build Pipeline

```
ui/src/mcp-app.ts (TypeScript + ext-apps SDK)
  → Vite + vite-plugin-singlefile
  → ui/dist/mcp-app.html (single 354KB HTML file)
  → Makefile copies to internal/resources/dashboard_ui.html
  → Go //go:embed at compile time
  → Served at runtime via resources/read or GET /dashboard
```

---

## Key Implementation Details

| Aspect | Implementation | File/Line |
|--------|---------------|-----------|
| App creation | `new App({ name: "Dino Dashboard", version: "0.1.0" })` | `mcp-app.ts` |
| Connect (auto-detect) | `app.connect()` no explicit transport | `mcp-app.ts` |
| Initial host context | `app.getHostContext()` after `connect()` | `mcp-app.ts` |
| `ontoolresult` handler | Receives `structuredContent`, renders dinosaur cards | `mcp-app.ts` |
| `ontoolinput` handler | Receives arguments/partial structured content | `mcp-app.ts` |
| `onhostcontextchanged` handler | Theme, fonts, safe areas, display modes | `mcp-app.ts` |
| `onteardown` handler | Resets state, returns `{}` | `mcp-app.ts` |
| `onerror` handler | Logs errors to host console | `mcp-app.ts` |
| `ontoolcancelled` handler | Logs cancellation reason | `mcp-app.ts` |
| Tool registration (Go) | `Meta: mcp.Meta{"ui": {"resourceUri": "ui://..."}}` | `dashboard.go:64` |
| Resource MIME type | `"text/html;profile=mcp-app"` | `dashboard.go:19` |
| Resource registration (Go) | `s.AddResource({MIMEType: "...profile=mcp-app"}, handler)` | `dashboard.go:35` |

## Official References

| Resource | URL |
|----------|-----|
| ext-apps SDK README | https://github.com/modelcontextprotocol/ext-apps |
| ext-apps Quickstart | https://apps.extensions.modelcontextprotocol.io/api/documents/Quickstart.html |
| ext-apps API Docs | https://apps.extensions.modelcontextprotocol.io/api/ |
| MCP Apps Overview | https://modelcontextprotocol.io/extensions/apps/overview |
| MCP Apps Spec | https://github.com/modelcontextprotocol/ext-apps/blob/main/specification/2026-01-26/apps.mdx |
| Vanilla JS Example | https://github.com/modelcontextprotocol/ext-apps/tree/main/examples/basic-server-vanillajs |
| MCP UI Gallery | https://mcpui.dev |

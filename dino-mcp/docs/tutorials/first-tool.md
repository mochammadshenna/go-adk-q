# Tutorial: Build Your First MCP Tool

> **Add a new typed MCP tool to dino-mcp.** You'll learn the `mcp.AddTool` generic pattern with typed args/result structs, JSON schema generation, and how to register tools with the server.

**Official references:**
- [MCP Spec — Tools](https://spec.modelcontextprotocol.io/specification/2025-03-26/server/tools/)
- [Go SDK — `mcp.AddTool`](https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp#AddTool)
- [Canonical example in this project](https://github.com/msw/dino-mcp/blob/main/internal/tools/think.go + ask.go)

---

## The Pattern

Every MCP tool in this project follows the same 3-step pattern:

```
Step 1: Define Args and Result structs (with json + jsonschema tags)
Step 2: Write a handler function (typed context, args → result)
Step 3: Register with the server via mcp.AddTool[T, A, R]()
```

The canonical implementations are in `internal/tools/think.go + ask.go`:
- `tools.RegisterThink()` — zero-arg tool, `any` args type
- `tools.RegisterAsk()` — single-arg tool, `DinoAskArgs` type

---

## Step 1: Add Your Tool Types

Edit `internal/tools/tools.go`. Add a new Args and Result struct after the existing type definitions:

```go
// DinoRoarArgs defines the input for the dino_roar tool.

// DinoRoarArgs defines the input for the dino_roar tool.
type DinoRoarArgs struct {
    Species string `json:"species" jsonschema:"The dinosaur species to roar for."`
    Volume  int    `json:"volume,omitempty" jsonschema:"Optional volume level (1-10, default 8)."`
}

// DinoRoarResult defines the output of the dino_roar tool.
type DinoRoarResult struct {
    Species string `json:"species" jsonschema:"The species that roared."`
    Roar    string `json:"roar" jsonschema:"The actual roar sound."`
    Volume  int    `json:"volume" jsonschema:"Volume level of the roar."`
}
```

**About the tags** (from `server.go` lines 158-172):
- **`json:"field_name"`** — serializes to JSON in the tool response
- **`jsonschema:"..."`** — generates the `description` in the tool's JSON Schema. The Go SDK reads these tags and builds the `InputSchema` from the Args struct.
- **`omitempty`** — makes the field optional in the schema

If your tool has **zero arguments** (like `dino_think`), use `any` as the args type:

```go
mcp.AddTool(s, &mcp.Tool{...}, func(_ context.Context, _ *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, DinoThinkResult, error) { ... })
```

---

## Step 2: Write the Handler

Create a new file `internal/tools/roar.go`:

```go
package tools

import (
    "context"
    "fmt"
    "strings"

    "github.com/modelcontextprotocol/go-sdk/mcp"
)

// RegisterRoar adds a tool that makes dinosaurs roar.
func RegisterRoar(s *mcp.Server) {
    mcp.AddTool(s, &mcp.Tool{
        Name:        "dino_roar",
        Description: "Makes a dinosaur roar! Choose a species and hear its mighty sound.",
        InputSchema: map[string]any{
            "type": "object",
            "properties": map[string]any{
                "species": map[string]any{
                    "type":        "string",
                    "description": "The dinosaur species name.",
                },
                "volume": map[string]any{
                    "type":        "integer",
                    "description": "Optional volume level 1-10.",
                    "default":     8,
                },
            },
            "required": []string{"species"},
        },
    }, func(_ context.Context, _ *mcp.CallToolRequest, args DinoRoarArgs) (*mcp.CallToolResult, DinoRoarResult, error) {
        // Validate
        if args.Species == "" {
            return nil, DinoRoarResult{}, fmt.Errorf("species must not be empty")
        }
        if args.Volume <= 0 {
            args.Volume = 8
        }
        if args.Volume > 10 {
            args.Volume = 10
        }

        // Curated roars
        roars := map[string]string{
            "tyrannosaurus rex": "ROOOOAR! The ground trembles!",
            "t-rex":             "ROOOOAR! The ground trembles!",
            "velociraptor":      "screeeech! *sounds of rapid movement*",
            "triceratops":       "rumble... grumble...",
            "stegosaurus":       "*a low, deep rumble*",
            "brachiosaurus":     "WOOOOOP! (a surprisingly gentle sound for such a giant)",
        }

        species := strings.ToLower(args.Species)
        roar, ok := roars[species]
        if !ok {
            roar = fmt.Sprintf("*a mysterious %s noise*", args.Species)
        }

        // Return both text content and structured content
        return &mcp.CallToolResult{
            Content: []mcp.Content{
                &mcp.TextContent{
                    Text: fmt.Sprintf("🦕 **%s** roars: %s (volume: %d/10)", args.Species, roar, args.Volume),
                },
            },
        }, DinoRoarResult{
            Species: args.Species,
            Roar:    roar,
            Volume:  args.Volume,
        }, nil
    })
}
```

**Key details about the handler signature** (from `mcp.AddTool` — see `internal/tools/think.go`):

```go
mcp.AddTool[ArgsType, ResultType](
    s *mcp.Server,                     // server instance
    tool *mcp.Tool,                    // tool metadata + input schema
    handler func(
        ctx context.Context,           // request context (with session state)
        req *mcp.CallToolRequest,      // full request (contains metadata, meta)
        args ArgsType,                 // deserialized args struct
    ) (*mcp.CallToolResult, ResultType, error),
)
```

The handler **must** return three values:
1. `*mcp.CallToolResult` — optional, contains `Content []Content` (array of `TextContent`, `ImageContent`, `StructuredContent`, etc.). If nil, it's auto-constructed from the result type.
2. `ResultType` — becomes `structuredContent` in the response for typed client access
3. `error` — if non-nil, the tool is considered failed and the error message is returned to the client

---

## Step 3: Register in the Server

Edit `internal/server/server.go`, find the `New()` function, and add the registration call:

```go
func New(logger *slog.Logger) *mcp.Server {
    // ...existing setup...
    
    tools.RegisterThink(s)
    tools.RegisterAsk(s)
    tools.RegisterDashboardTool(s)
    resources.RegisterDashboardResource(s, logger)
    tools.RegisterRoar(s)  // ← add this line
    
    logger.Info("dino-mcp server initialized",
        "tools", []string{"dino_think", "dino_ask", "dino_dashboard", "dino_roar"},  // ← update this
        "resources", []string{"ui://dino-dashboard/mcp-app.html"},
    )
    
    return s
}
```

---

## Step 4: Build and Test

```bash
# Build
make build-fast

# Run the integration tests (which test all registered tools)
make test

# Or test manually with curl
```

### Manual curl test

```bash
# 1. Initialize and grab session ID
SID=$(curl -s -X POST http://localhost:9010/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-05","capabilities":{},"clientInfo":{"name":"curl","version":"1.0"}}}' \
  | grep -o '"Mcp-Session-Id":"[^"]*"' | cut -d'"' -f4)
echo "Session: $SID"

# 2. Notify initialized
curl -s -X POST http://localhost:9010/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -H "Mcp-Session-Id: $SID" \
  -d '{"jsonrpc":"2.0","id":2,"method":"notifications/initialized"}'

# 3. Call dino_roar
curl -s -X POST http://localhost:9010/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -H "Mcp-Session-Id: $SID" \
  -d '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"dino_roar","arguments":{"species":"T-Rex","volume":10}}}' \
  | python3 -m json.tool
```

---

## Integration Test

Add a test to `test_mcp.sh` following the existing pattern (see test 4 for `dino_think`):

```bash
# Test: dino_roar tool
echo "  [$(step)] Testing dino_roar..."
ROAR=$(curl -s -X POST "$MCP_URL" \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -H "Mcp-Session-Id: $SID" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"dino_roar","arguments":{"species":"T-Rex"}}}' \
  | grep -o '"roar":"[^"]*"' | head -1)
if [ -z "$ROAR" ]; then
  fail $step "dino_roar: no roar in response"
fi
pass $step "dino_roar"
```

---

## Recap

| Step | File | What you did |
|------|------|-------------|
| 1 | `internal/tools/tools.go` | Added `DinoRoarArgs` and `DinoRoarResult` types |
| 2 | `internal/tools/roar.go` | Implemented `RegisterRoar()` with `mcp.AddTool` |
| 3 | `internal/server/server.go` (in `New()`) | Called `tools.RegisterRoar(s)` |
| 4 | Terminal | `make build-fast && make test` |
| (optional) | `test_mcp.sh` | Added integration test |

**Next:** [Add a dinosaur species](../how-to/add-dinosaur.md) or [understand the MCP protocol in depth](../explanation/architecture.md)

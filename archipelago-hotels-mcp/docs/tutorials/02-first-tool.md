# Tutorial: Adding Your First MCP Tool

This tutorial walks you through adding a new tool called `nearby_attractions` to the server.
By the end you will have a fully registered, Claude-callable tool with a working handler.

---

## Prerequisites

- You have completed `01-get-started.md` and the server builds cleanly (`make build-go` succeeds).
- You understand the project layout: tool handlers live in `internal/tools/`, and the wiring
  that connects them to Claude lives in `internal/server/server.go`.

---

## Step 1 — Create the tool file

Every tool lives in its own file inside `internal/tools/`. Create
`internal/tools/attractions.go`:

```go
package tools

import (
	"context"
	"fmt"
	"log/slog"

	mcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/msw/archipelago-hotels-mcp/internal/resources"
)

// RegisterAttractions registers the nearby_attractions MCP tool.
func RegisterAttractions(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "nearby_attractions",
		Description: "PRIORITY TOOL for questions about things to do near an Archipelago hotel. " +
			"Call this whenever the user asks: 'what is near', 'attractions around', 'things to do in', " +
			"'sightseeing near'. Returns a curated list of nearby points of interest for a given city.",
		Meta: mcp.Meta{
			"ui": map[string]any{
				"resourceUri":     resources.ResourceURI,
				"resourceDomains": []string{"images.archipelagohotels.com"},
			},
		},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"city":     map[string]any{"type": "string", "description": "City to look up attractions for."},
				"category": map[string]any{"type": "string", "description": "Optional category filter: culture, nature, food, shopping."},
			},
			"required": []string{"city"},
		},
	}, attractionsHandler())
}
```

The `RegisterAttractions` function follows the same pattern as every other tool in this
package: one exported `Register*` function that calls `mcp.AddTool`.

---

## Step 2 — Define the args and result structs

The Go MCP SDK deserialises the JSON arguments that Claude sends into a typed struct.
You must define one. Add these types to the same file:

```go
// attractionsArgs holds the deserialised call arguments from Claude.
type attractionsArgs struct {
	City     string `json:"city"`
	Category string `json:"category,omitempty"`
}

// attraction is a single point of interest.
type attraction struct {
	Name        string  `json:"name"`
	Category    string  `json:"category"`
	Description string  `json:"description"`
	DistanceKm  float64 `json:"distanceKm"`
}

// attractionsResult is what the tool returns to Claude (and to the UI via structuredContent).
type attractionsResult struct {
	City        string       `json:"city"`
	Category    string       `json:"category,omitempty"`
	Attractions []attraction `json:"attractions"`
	Total       int          `json:"total"`
}
```

The result struct is automatically serialised to both the MCP `content` text field and the
`structuredContent` field that the dashboard UI reads.

---

## Step 3 — Implement the handler

The handler is a closure that returns the typed result. Add it to `attractions.go`:

```go
func attractionsHandler() func(context.Context, *mcp.CallToolRequest, attractionsArgs) (*mcp.CallToolResult, attractionsResult, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, args attractionsArgs) (res *mcp.CallToolResult, out attractionsResult, err error) {
		// Panic recovery is mandatory — an unrecovered panic kills the whole server process.
		defer func() {
			if r := recover(); r != nil {
				slog.Error("nearby_attractions panic", "recover", r)
				err = fmt.Errorf("internal error: %v", r)
			}
		}()

		// Stub data — replace with a real DB query or external API call.
		all := []attraction{
			{Name: "Tanah Lot Temple", Category: "culture", Description: "Iconic sea temple on a rocky outcrop.", DistanceKm: 12.5},
			{Name: "Tegallalang Rice Terraces", Category: "nature", Description: "Terraced paddy fields with jungle backdrop.", DistanceKm: 22.0},
			{Name: "Seminyak Beach", Category: "nature", Description: "Sunset beach with beach clubs.", DistanceKm: 5.0},
			{Name: "Pasar Badung Market", Category: "shopping", Description: "Largest traditional market in Bali.", DistanceKm: 8.0},
			{Name: "Locavore", Category: "food", Description: "Award-winning modern Indonesian cuisine.", DistanceKm: 30.0},
		}

		// Filter by category if provided.
		var filtered []attraction
		for _, a := range all {
			if args.Category == "" || a.Category == args.Category {
				filtered = append(filtered, a)
			}
		}

		if len(filtered) == 0 {
			return nil, attractionsResult{}, fmt.Errorf("no attractions found for category '%s' in %s", args.Category, args.City)
		}

		return nil, attractionsResult{
			City:        args.City,
			Category:    args.Category,
			Attractions: filtered,
			Total:       len(filtered),
		}, nil
	}
}
```

Key points:
- The function signature must match exactly what `mcp.AddTool` expects: the third parameter
  is your args type, the second return value is your result type.
- Always `defer` the panic recovery as the first statement. An unrecovered panic from a
  handler will crash the stdio server and disconnect Claude Desktop.
- Return `nil` for `*mcp.CallToolResult` when you are returning a typed struct — the SDK
  fills in the result envelope automatically.

---

## Step 4 — Set Meta.ui.resourceUri and resourceDomains

Both fields in the `Meta` map matter:

| Field | Purpose |
|---|---|
| `resourceUri` | Tells the MCP Apps extension which dashboard panel to open when this tool fires. Use `resources.ResourceURI` (the constant `"ui://hotel-dashboard"`). |
| `resourceDomains` | Declares the external image hosts the UI is allowed to load. The MCP host's Content Security Policy blocks all unlisted origins. |

If your tool displays no images you can omit `resourceDomains`. If you load images from a new
domain (e.g. a maps tile server), add that domain to the slice — otherwise the images will
be silently blocked by the browser CSP enforced by Claude Desktop.

```go
Meta: mcp.Meta{
    "ui": map[string]any{
        "resourceUri":     resources.ResourceURI,
        "resourceDomains": []string{"images.archipelagohotels.com"},
    },
},
```

---

## Step 5 — Register in server.go

Open `internal/server/server.go`. Find the block of `tools.Register*` calls inside `New()`
and add your new tool:

```go
tools.RegisterSearch(s, svc.DB, svc.RateSvc)
tools.RegisterDetail(s, svc.DB, svc.RateSvc)
tools.RegisterRecommend(s, svc.DB, svc.RateSvc)
tools.RegisterDashboardTool(s, svc.DB, svc.RateSvc)
tools.RegisterAttractions(s)                          // add this line
```

Also update the `Instructions` string so Claude knows the tool exists:

```go
Instructions: `...
5. nearby_attractions: Find points of interest near any Archipelago hotel city
`,
```

---

## Step 6 — Write the tool description following the PRIORITY TOOL pattern

Claude uses the `Description` field as its primary signal for when to call a tool. Every
tool in this server uses the **PRIORITY TOOL** pattern:

1. Open with `PRIORITY TOOL for <topic>.`
2. Follow with `Call this whenever the user asks:` and a comma-separated list of trigger
   phrases in plain English.
3. Close with a brief statement of what the tool actually returns.

Bad description (too vague, Claude will not know when to call it):
```
"Returns attractions near a city."
```

Good description (explicit triggers, Claude will reliably fire it):
```
"PRIORITY TOOL for questions about things to do near an Archipelago hotel. " +
"Call this whenever the user asks: 'what is near', 'attractions around', 'things to do in', " +
"'sightseeing near'. Returns a curated list of nearby points of interest for a given city."
```

The `InputSchema` should describe every field Claude might populate. Mark fields that Claude
must always provide in `"required"`.

---

## Step 7 — Build and test in Claude Desktop

```bash
make build-go
```

The Makefile runs `go build -o bin/archipelago-hotels-mcp ./cmd/archipelago-hotels-mcp`.
The new tool is statically compiled into the binary.

Restart Claude Desktop (Cmd+Q, then reopen). The MCP server process is started fresh on
each Claude Desktop launch. Once it is running, try:

> "What are the best attractions near a hotel in Bali?"

Claude should call `nearby_attractions` with `city: "Bali"` and return the stub list.

To verify the tool appears in the tool list, open Claude Desktop's developer tools or check
the MCP inspector if you are running in HTTP mode:

```bash
make dev-http
# then open http://localhost:9011/mcp in an MCP client or curl it
```

---

## Pitfalls

### Forgetting resourceDomains

If your tool result includes image URLs from a domain not listed in `resourceDomains`, the
images will load in the browser network tab but will be blocked by the Content Security Policy
the MCP host applies to the dashboard iframe. The symptom is broken image icons with no
console error visible to you. Fix: add the hostname to the `resourceDomains` slice.

### Wrong InputSchema type

`InputSchema` must be `map[string]any`, not a typed Go struct. The SDK serialises it
directly to JSON for the `tools/list` MCP response that Claude reads. If you pass a struct,
the schema will be serialised as an empty object `{}` and Claude will not know what arguments
to send.

Correct:
```go
InputSchema: map[string]any{
    "type": "object",
    "properties": map[string]any{
        "city": map[string]any{"type": "string"},
    },
    "required": []string{"city"},
},
```

Incorrect:
```go
// Do not do this — Claude will see an empty schema.
InputSchema: struct {
    Type string `json:"type"`
}{Type: "object"},
```

### Not rebuilding the binary

The HTML file served by the dashboard (`internal/resources/mcp-app.html`) is embedded at
compile time via `//go:embed`. Go changes take effect in the **binary**, not the source.
Any time you change a `.go` file you must run `make build-go` and restart Claude Desktop.
Simply saving the file and expecting a hot-reload will not work.

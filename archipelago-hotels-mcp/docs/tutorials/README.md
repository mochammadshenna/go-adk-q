# Tutorials

> **Learning-oriented.** These guides take you from zero to working code.
> Follow them in order the first time through.

---

## Tutorial 1 — Build and run your first query {#tutorial-1}

**Goal:** compile the binary, point it at a MySQL instance, and call `search_hotels`
from the Claude Desktop chat window.

### Prerequisites

- Go 1.25+ (`go version`)
- Node 20+ and `npm` (for the UI build step)
- A running MySQL 8.x instance with the Archipelago schema loaded
- Claude Desktop (any recent version)

### Steps

**1. Clone the repository**

```bash
git clone https://github.com/archipelago-hotels/archipelago-hotels-mcp.git
cd archipelago-hotels-mcp
```

**2. Build the UI assets**

The TypeScript ext-app is compiled by Vite and embedded into the Go binary via
`//go:embed`. The `make build` target runs both steps in order:

```bash
make build
# Produces: bin/archipelago-hotels-mcp
```

**3. Verify the binary starts**

```bash
MYSQL_HOST=127.0.0.1 MYSQL_USER=root MYSQL_PASS=secret \
  ./bin/archipelago-hotels-mcp --help
```

You should see the usage banner. If MySQL is unreachable the process exits
immediately — the server fails fast on startup rather than serving broken tools.

**4. Register with Claude Desktop**

Open `~/Library/Application Support/Claude/claude_desktop_config.json` and add:

```jsonc
{
  "mcpServers": {
    "archipelago-hotels": {
      "command": "/absolute/path/to/bin/archipelago-hotels-mcp",
      "env": {
        "MYSQL_HOST": "127.0.0.1",
        "MYSQL_USER": "root",
        "MYSQL_PASS": "secret",
        "MYSQL_DB":   "db_archipelagowebsite"
      }
    }
  }
}
```

Restart Claude Desktop.

**5. Ask Claude**

In the chat input type:

> _"Find me hotels in Bali under USD 150 per night."_

Claude will call `search_hotels` and return a formatted list with live rates.

**What you learned:** the binary is self-contained; all it needs is network access to
MySQL and (optionally) the SimpleBooking API.

---

## Tutorial 2 — Add a new MCP tool {#tutorial-2}

**Goal:** add a `list_brands` tool that returns all hotel brands.

### Steps

**1. Define the tool in `internal/tools/`**

Create `internal/tools/brands.go`:

```go
package tools

import (
    "context"
    "encoding/json"

    "github.com/modelcontextprotocol/go-sdk/mcp"
    "github.com/msw/archipelago-hotels-mcp/internal/repository"
)

type ListBrandsParams struct{}

func ListBrandsHandler(repo *repository.Pool) mcp.ToolHandlerFunc {
    return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        brands, err := repo.ListBrands(ctx)
        if err != nil {
            return mcp.NewToolResultError(err.Error()), nil
        }
        b, _ := json.MarshalIndent(brands, "", "  ")
        return mcp.NewToolResultText(string(b)), nil
    }
}
```

**2. Register in `internal/server/server.go`**

Find the block where `search_hotels` is registered and add:

```go
s.AddTool(mcp.NewTool("list_brands",
    mcp.WithDescription("Return all Archipelago hotel brands"),
), tools.ListBrandsHandler(pool))
```

**3. Implement the repository method**

Add `ListBrands` to `internal/repository/hotel.go`:

```go
func (p *Pool) ListBrands(ctx context.Context) ([]BrandRow, error) {
    rows, err := p.Central.QueryContext(ctx,
        `SELECT brand_id, brand_name, db_prefix_name FROM tb_brands ORDER BY brand_name`)
    // ... scan rows into []BrandRow
}
```

**4. Rebuild and test**

```bash
make build-go
# Restart Claude Desktop, then ask: "List all Archipelago brands"
```

**What you learned:** tools are thin handlers wired in `server.go`; business logic
lives in `repository/`.

---

## Tutorial 3 — Extend the ext-app UI {#tutorial-3}

**Goal:** add a "Brands" filter chip to the hotel dashboard TypeScript UI.

### Prerequisites

- Node 20+, `npm install` already run inside `ui/`

### Steps

**1. Start the hot-reload dev server**

```bash
make dev-http        # starts Go HTTP server on :9011
cd ui && npm run dev # starts Vite dev server, proxies /api to :9011
```

Open `http://localhost:5173` in a browser.

**2. Add the filter to `ui/src/mcp-app.ts`**

Find the `renderFilters()` function and add a brand chip group alongside the
existing city chips. The pattern is identical — call `/api/brands` to populate,
then append `&brand=<slug>` to the hotel list URL on click.

**3. Rebuild and embed**

```bash
cd ui && npm run build   # outputs to ui/dist/
make build-go            # embeds ui/dist via //go:embed
```

**4. Verify in Claude Desktop**

Restart Claude Desktop. Open the ext-app panel — the Brands filter row should
appear above the hotel grid.

**What you learned:** the UI is a single compiled TypeScript file embedded in the
binary; changes require a full rebuild before they appear in Claude Desktop.

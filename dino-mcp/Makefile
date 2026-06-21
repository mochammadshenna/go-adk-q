# dino-mcp Makefile — Build, install, and run the MCP server
#
# Targets:
#   build          Build the Go binary and the Vite UI
#   build-ui       Build the Vite MCP App UI (requires npm)
#   build-go       Build only the Go binary
#   install        Install the binary to GOPATH/bin
#   run-stdio      Run on stdio transport
#   run-http       Run on Streamable HTTP at :9010
#   dev-http       Dev mode: run Go server with debug logging
#   lint           Run go vet
#   clean          Remove build artifacts
#   help           Print this help

GO ?= go
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
BINARY ?= dino-mcp
OUTDIR ?= bin
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

# Embed the version at build time
LDFLAGS = -ldflags="-X github.com/msw/dino-mcp/internal/server.Version=$(VERSION)"

.PHONY: all build build-ui build-go install run-stdio run-http run-http-json dev-http run-tunnel test test-inspector lint clean help

all: build

# --- UI Build ---

UI_DIR = ui
UI_DIST = $(UI_DIR)/dist/mcp-app.html
EMBED_TARGET = internal/resources/dashboard_ui.html
STATIC_FALLBACK = internal/resources/dashboard_ui.html

build-ui:
	@echo "=== Building MCP App UI ==="
	cd $(UI_DIR) && npm install --silent && npm run build
	@echo "UI built: $(UI_DIST)"
	@echo "Copying UI to embed target..."
	cp $(UI_DIST) $(EMBED_TARGET)
	@echo "UI embedded at: $(EMBED_TARGET)"

# --- Go Build ---

build-go:
	@echo "=== Building dino-mcp binary ==="
	$(GO) build $(LDFLAGS) -o $(OUTDIR)/$(BINARY) ./cmd/dino-mcp
	@echo "Binary: $(OUTDIR)/$(BINARY)"

# Full build: UI + Go
build: build-ui build-go
	@echo "=== Build complete ==="
	@ls -lh $(OUTDIR)/$(BINARY)

# Fast build: Go only (uses embedded standalone UI)
build-fast: build-go
	@echo "=== Fast build complete (using standalone UI) ==="

install:
	@echo "=== Installing dino-mcp ==="
	$(GO) install $(LDFLAGS) ./cmd/dino-mcp
	@echo "Installed to $$(go env GOPATH)/bin/dino-mcp"

# --- Run Targets ---

run-stdio: build-fast
	$(OUTDIR)/$(BINARY) stdio

run-http: build-fast
	$(OUTDIR)/$(BINARY) http -addr :9010

dev-http: build-fast
	$(OUTDIR)/$(BINARY) http -addr :9010 -verbose

# Cloudflare Tunnel — exposes :9010 publicly via trycloudflare.com
run-tunnel: build-fast
	@echo "=== Starting dino-mcp + Cloudflare Tunnel ==="
	@echo ""
	$(OUTDIR)/$(BINARY) http -addr :9010 &
	sleep 2
	@echo "Tunnel URL will appear below (ctrl+c to stop both):"
	cloudflared tunnel --url http://localhost:9010

# --- Run: JSON mode ---

run-http-json: build-fast
	$(OUTDIR)/$(BINARY) http -addr :9010 -verbose

# --- Testing ---

test: build-fast
	@echo "=== Running integration test suite ==="
	bash test_mcp.sh

test-inspector: build-fast
	@echo "=== Starting dino-mcp (Streamable HTTP) + MCP Inspector ==="
	@echo "Open http://localhost:5173 in your browser"
	@echo ""
	$(OUTDIR)/$(BINARY) http -addr :9010 &
	sleep 2
	npx @modelcontextprotocol/inspector --transport http --server-url http://localhost:9010/mcp --port 5173

# --- Verification ---

lint:
	$(GO) vet ./...
	$(GO) fmt ./...

# --- Clean ---

clean:
	rm -rf $(OUTDIR) $(UI_DIR)/dist node_modules
	$(GO) clean ./...

# --- Help ---

help:
	@echo 'dino-mcp Makefile'
	@echo ''
	@echo 'Targets:'
	@grep -E '^[a-zA-Z_-]+:' $(MAKEFILE_LIST) | sort | \
	  while IFS=: read -r target rest; do \
	    desc=$$(grep -A1 "^$$target:" $(MAKEFILE_LIST) | tail -1 | sed 's/^# //'); \
	    printf "  %-15s %s\n" "$$target" "$$desc"; \
	  done

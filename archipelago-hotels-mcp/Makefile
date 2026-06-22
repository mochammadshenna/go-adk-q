# Archipelago Hotels MCP Makefile
#
# Targets:
#   build          Full build: UI + Go binary
#   build-ui       Vite MCP App UI (requires npm)
#   build-go       Go binary only
#   install        Install binary to GOPATH/bin
#   run-stdio      Run on stdio transport
#   dev-http       HTTP mode with debug logging on :9011
#   lint           go vet
#   clean          Remove build artifacts
#   help           Print this help

GO ?= go
BINARY ?= archipelago-hotels-mcp
OUTDIR ?= bin

.PHONY: all build build-ui build-go install run-stdio dev-http lint clean help

all: build

# --- UI Build ---

UI_DIR = ui
UI_DIST = $(UI_DIR)/dist/index.html
EMBED_TARGET = internal/resources/mcp-app.html

build-ui:
	@echo "=== Building MCP App UI ==="
	cd $(UI_DIR) && npm install --silent && npm run build
	@echo "UI built: $(UI_DIST)"
	cp $(UI_DIST) $(EMBED_TARGET)
	@echo "UI embedded at: $(EMBED_TARGET)"

# --- Go Build ---

build-go:
	@echo "=== Building archipelago-hotels-mcp binary ==="
	$(GO) build -o $(OUTDIR)/$(BINARY) ./cmd/archipelago-hotels-mcp
	@echo "Binary: $(OUTDIR)/$(BINARY)"

build: build-ui build-go
	@echo "=== Build complete ==="
	@ls -lh $(OUTDIR)/$(BINARY)

build-fast: build-go
	@echo "=== Fast build complete (using standalone UI) ==="

install:
	$(GO) install ./cmd/archipelago-hotels-mcp

# --- Run ---

run-stdio: build-fast
	$(OUTDIR)/$(BINARY) stdio

dev-http: build-fast
	$(OUTDIR)/$(BINARY) http -addr :9011 -verbose

# --- Verify ---

lint:
	$(GO) vet ./...

# --- Clean ---

clean:
	rm -rf $(OUTDIR) $(UI_DIR)/dist $(UI_DIR)/node_modules
	$(GO) clean ./...

help:
	@echo 'Archipelago Hotels MCP Makefile'
	@echo ''
	@grep -E '^[a-zA-Z_-]+:' $(MAKEFILE_LIST) | sort | \
	  while IFS=: read -r target rest; do \
	    desc=$$(grep -A1 "^$$target:" $(MAKEFILE_LIST) | tail -1 | sed 's/^# //'); \
	    printf "  %-15s %s\n" "$$target" "$$desc"; \
	  done

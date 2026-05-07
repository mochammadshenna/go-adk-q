# go-adk-q — Google ADK Go reference implementation
#
# Usage:
#   make                           print this help
#   make build                     compile (go build ./...)
#   make run                       Bubbletea/Lipgloss TUI chat  (go run ./cmd/tui chat)
#   make console                   ADK built-in text console     (go run . console)
#   make web                       browser dev-UI + API   (web webui api, http://localhost:8080)
#   make api                       REST API server only   (web api,       http://localhost:8080)
#   make test                      run all tests
#   make vet                       run go vet
#   make lint                      run golangci-lint (install: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
#   make tidy                      go mod tidy
#   make clean                     remove build artefacts
#   make env                       show which env vars are set / unset
#
# ── Linux cross-compilation ───────────────────────────────────────────────────
#
#   make build-linux-amd64         CGO=0  linux/amd64  portable, no glibc dep
#   make build-tui-linux-amd64     CGO=0  linux/amd64  TUI, portable
#   make build-linux-amd64-cgo     CGO=1  linux/amd64  glibc 2.34 via zig (AL2023/x86_64)
#   make build-linux-arm64-cgo     CGO=1  linux/arm64  glibc 2.34 via zig (AL2023/Graviton)
#   make build-tui-linux-amd64-cgo CGO=1  linux/amd64  TUI, glibc 2.34 via zig
#   make cgo-info                  show CGO env, active CGO packages, available targets
#   make verify-elf                run 'file' on built linux binaries
#   make check-zig                 verify zig is on PATH (prereq for CGO targets)
#
# ── Environment variables ────────────────────────────────────────────────────
#
# Required:
#   GOOGLE_API_KEY            Gemini API key (https://aistudio.google.com/apikey)
#
# Groq (enables groq_agent):
#   GROQ_API_KEY              https://console.groq.com/keys
#   GROQ_MODEL                override default (llama-3.3-70b-versatile)
#
# NVIDIA NIM (enables nvidia_agent):
#   NVIDIA_API_KEY            https://build.nvidia.com
#   NVIDIA_MODEL              override default (minimaxai/minimax-m1)
#   NVIDIA_BASE_URL           on-premises NIM endpoint, e.g. http://nim:8000/v1
#
# OpenRouter (enables openrouter_agent):
#   OPENROUTER_API_KEY        https://openrouter.ai/keys
#   OPENROUTER_MODEL          override default (meta-llama/llama-3.3-70b-instruct)
#   OPENROUTER_SITE_URL       HTTP-Referer attribution header
#   OPENROUTER_APP_NAME       X-Title attribution header
#
# HuggingFace (enables huggingface_agent):
#   HF_TOKEN                  https://huggingface.co/settings/tokens
#   HF_MODEL                  override default (mistralai/Mistral-7B-Instruct-v0.3)
#   HF_ENDPOINT_URL           dedicated Inference Endpoint URL (disables serverless)
#
# You can put any of the above in a .env file and run `source .env` before make,
# or use `export VAR=value` directly in your shell.
# ─────────────────────────────────────────────────────────────────────────────

# Pull in a .env file if present (never committed; add to .gitignore).
-include .env
export

BINARY      := go-adk-q
TUI_BINARY  := tui
GO          := go
GOFLAGS     ?=
PKG         := ./...

# ── Output directory ──────────────────────────────────────────────────────────
BIN_DIR     := bin

# ── CGO cross-compilation via Zig ────────────────────────────────────────────
# Zig bundles every historical glibc ABI, enabling deterministic CGO
# cross-compilation to a specific glibc version without Docker or a sysroot.
# Specifying "x86_64-linux-gnu.2.34" emits only symbols present in glibc ≤ 2.34
# — the exact version shipped in Amazon Linux 2023.
#
# CGO packages active on linux/amd64 with CGO_ENABLED=1 (stdlib only):
#   net         — libc resolver (getaddrinfo; respects /etc/nsswitch.conf + VPC DNS)
#   os/user     — libc user lookup (getpwuid_r, getgrnam_r)
#   runtime/cgo — CGO bookkeeping
#
# No third-party package in this codebase has CGO files.
# CGO_ENABLED=0 path: -tags "netgo osusergo" makes stdlib pure-Go too.
ZIG_TARGET_AMD64 := x86_64-linux-gnu.2.34
ZIG_TARGET_ARM64 := aarch64-linux-gnu.2.34

# Detect whether golangci-lint is on PATH.
HAS_LINT := $(shell command -v golangci-lint 2>/dev/null)

.DEFAULT_GOAL := help

# ── help ─────────────────────────────────────────────────────────────────────
.PHONY: help
help:
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
	  | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-28s\033[0m %s\n", $$1, $$2}' \
	  | sort
	@echo ""
	@echo "Run 'make env' to see which API keys are configured."

# ── build ─────────────────────────────────────────────────────────────────────
.PHONY: build
build: ## Compile all packages (go build ./...)
	$(GO) build $(GOFLAGS) $(PKG)

# ── Linux cross-compilation ───────────────────────────────────────────────────

.PHONY: build-linux-amd64
build-linux-amd64: ## Build main binary for linux/amd64 — CGO=0, no glibc dep, portable
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
	  $(GO) build $(GOFLAGS) \
	  -ldflags="-s -w" \
	  -tags "netgo osusergo" \
	  -o $(BIN_DIR)/$(BINARY)-linux-amd64 .
	@echo "Built: $(BIN_DIR)/$(BINARY)-linux-amd64  [CGO=0, netgo, osusergo]"

.PHONY: build-tui-linux-amd64
build-tui-linux-amd64: ## Build TUI binary for linux/amd64 — CGO=0, no glibc dep, portable
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
	  $(GO) build $(GOFLAGS) \
	  -ldflags="-s -w" \
	  -tags "netgo osusergo" \
	  -o $(BIN_DIR)/$(TUI_BINARY)-linux-amd64 ./cmd/tui
	@echo "Built: $(BIN_DIR)/$(TUI_BINARY)-linux-amd64  [CGO=0, netgo, osusergo]"

.PHONY: build-linux-amd64-cgo
build-linux-amd64-cgo: check-zig ## Build main binary for linux/amd64 — CGO=1, glibc 2.34 via zig (AL2023/x86_64)
	@mkdir -p $(BIN_DIR)
	CC="zig cc -target $(ZIG_TARGET_AMD64)" \
	CXX="zig c++ -target $(ZIG_TARGET_AMD64)" \
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
	  $(GO) build $(GOFLAGS) \
	  -ldflags="-s -w" \
	  -o $(BIN_DIR)/$(BINARY)-linux-amd64-cgo .
	@echo "Built: $(BIN_DIR)/$(BINARY)-linux-amd64-cgo  [CGO=1, glibc $(ZIG_TARGET_AMD64)]"

.PHONY: build-linux-arm64-cgo
build-linux-arm64-cgo: check-zig ## Build main binary for linux/arm64 — CGO=1, glibc 2.34 via zig (AL2023/Graviton)
	@mkdir -p $(BIN_DIR)
	CC="zig cc -target $(ZIG_TARGET_ARM64)" \
	CXX="zig c++ -target $(ZIG_TARGET_ARM64)" \
	CGO_ENABLED=1 GOOS=linux GOARCH=arm64 \
	  $(GO) build $(GOFLAGS) \
	  -ldflags="-s -w" \
	  -o $(BIN_DIR)/$(BINARY)-linux-arm64-cgo .
	@echo "Built: $(BIN_DIR)/$(BINARY)-linux-arm64-cgo  [CGO=1, glibc $(ZIG_TARGET_ARM64)]"

.PHONY: build-tui-linux-amd64-cgo
build-tui-linux-amd64-cgo: check-zig ## Build TUI binary for linux/amd64 — CGO=1, glibc 2.34 via zig (AL2023)
	@mkdir -p $(BIN_DIR)
	CC="zig cc -target $(ZIG_TARGET_AMD64)" \
	CXX="zig c++ -target $(ZIG_TARGET_AMD64)" \
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
	  $(GO) build $(GOFLAGS) \
	  -ldflags="-s -w" \
	  -o $(BIN_DIR)/$(TUI_BINARY)-linux-amd64-cgo ./cmd/tui
	@echo "Built: $(BIN_DIR)/$(TUI_BINARY)-linux-amd64-cgo  [CGO=1, glibc $(ZIG_TARGET_AMD64)]"

.PHONY: check-zig
check-zig: ## Verify zig is on PATH (required for CGO cross-compilation targets)
	@command -v zig >/dev/null 2>&1 || { \
	  echo ""; \
	  echo "  ERROR: 'zig' not found on PATH."; \
	  echo ""; \
	  echo "  Zig is required for CGO cross-compilation targeting glibc 2.34."; \
	  echo "  Install:"; \
	  echo "    macOS:  brew install zig"; \
	  echo "    Debian: apt install zig"; \
	  echo "    Fedora: dnf install zig"; \
	  echo "    Manual: https://ziglang.org/download/"; \
	  echo ""; \
	  exit 1; \
	}
	@printf "  zig %s found\n" "$$(zig version 2>/dev/null)"

.PHONY: cgo-info
cgo-info: ## Show CGO env, active CGO packages per platform, and available build targets
	@echo "── Host environment ─────────────────────────────────────────────────"
	@printf "  %-16s %s\n" "OS/Arch:"  "$$(uname -s)/$$(uname -m)"
	@printf "  %-16s %s\n" "Go:"       "$$($(GO) version)"
	@printf "  %-16s %s\n" "CGO:"      "$$($(GO) env CGO_ENABLED)"
	@printf "  %-16s %s\n" "CC:"       "$$($(GO) env CC)"
	@printf "  %-16s %s\n" "CXX:"      "$$($(GO) env CXX)"
	@printf "  %-16s %s\n" "Zig:"      "$$(zig version 2>/dev/null || echo '(not installed)')"
	@echo ""
	@echo "── CGO-active packages — linux/amd64 CGO_ENABLED=1 ─────────────────"
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=1 $(GO) list -deps \
	  -f '{{if .CgoFiles}}  {{.ImportPath}}{{end}}' $(PKG) 2>/dev/null | sort -u || true
	@echo ""
	@echo "── CGO-active packages — $$($(GO) env GOOS)/$$($(GO) env GOARCH) (current) ──────"
	@$(GO) list -deps -f '{{if .CgoFiles}}  {{.ImportPath}}{{end}}' $(PKG) 2>/dev/null \
	  | sort -u | grep -v '^$$' || echo "  (none)"
	@echo ""
	@echo "── Available build targets ──────────────────────────────────────────"
	@echo "  make build                         dev       $$($(GO) env GOOS)/$$($(GO) env GOARCH)  CGO auto"
	@echo "  make build-linux-amd64             CGO=0     linux/amd64  portable, no glibc dep"
	@echo "  make build-tui-linux-amd64         CGO=0     linux/amd64  TUI, portable"
	@echo "  make build-linux-amd64-cgo         CGO=1     linux/amd64  glibc $(ZIG_TARGET_AMD64) via zig"
	@echo "  make build-linux-arm64-cgo         CGO=1     linux/arm64  glibc $(ZIG_TARGET_ARM64) via zig (Graviton)"
	@echo "  make build-tui-linux-amd64-cgo     CGO=1     linux/amd64  TUI, glibc $(ZIG_TARGET_AMD64) via zig"

.PHONY: verify-elf
verify-elf: ## Run 'file' on built linux binaries; print glibc-check instructions
	@echo "── ELF verification ─────────────────────────────────────────────────"
	@found=0; \
	for f in $(BIN_DIR)/*-linux-*; do \
	  if [ -f "$$f" ]; then \
	    found=1; \
	    echo ""; \
	    echo "  $$f"; \
	    file "$$f" | sed 's/^/    /'; \
	  fi; \
	done; \
	if [ $$found -eq 0 ]; then \
	  echo "  (no linux binaries in $(BIN_DIR)/ — run a build target first)"; \
	fi
	@echo ""
	@echo "  To verify glibc symbol ceiling on a Linux host:"
	@echo "    objdump -p <binary> | grep GLIBC    # expect: GLIBC_2.34 or earlier"
	@echo "    readelf -d <binary> | grep NEEDED    # expect: libc.so.6 for CGO builds"

# ── run modes ─────────────────────────────────────────────────────────────────
.PHONY: chat
chat: ## Start Bubbletea/Lipgloss TUI chat (full-terminal UI with spinner and colours)
	$(GO) run $(GOFLAGS) ./cmd/tui chat

.PHONY: console
console: ## Start ADK built-in text console (raw readline REPL, no Bubbletea)
	$(GO) run $(GOFLAGS) . console

.PHONY: web
web: ## Start browser dev-UI + REST API at http://localhost:8080 (web webui api)
	$(GO) run $(GOFLAGS) . web webui api

.PHONY: run
run: ## Start REST API server only at http://localhost:8080 (web api)
	$(GO) run $(GOFLAGS) . web api

# ── quality ───────────────────────────────────────────────────────────────────
.PHONY: test
test: ## Run all tests (go test -race ./...)
	$(GO) test $(GOFLAGS) -race -count=1 $(PKG)

.PHONY: test-failover
test-failover: ## Run failover unit tests (no API keys needed — uses in-process mocks)
	$(GO) test $(GOFLAGS) -v -count=1 ./model/failover/...

# test-failover-live: end-to-end proof that failover kicks in with real providers.
#
# How it works:
#   GOOGLE_MODEL=gemini-intentionally-broken  forces Gemini to return a 400 error
#   A real fallback key (GROQ_API_KEY etc.)   provides the working backup
#
# Expected output in the console session:
#   INFO  model chain  providers="failover(gemini-intentionally-broken → groq/llama-3.3-70b-versatile)"
#   WARN  failover: provider error, trying next  provider=gemini-...  error="unexpected model name format"
#   INFO  failover: recovered via fallback        provider=groq/...
#
# Prerequisites: set at least one of GROQ_API_KEY / NVIDIA_API_KEY /
#               OPENROUTER_API_KEY / HF_TOKEN in addition to GOOGLE_API_KEY.
.PHONY: test-failover-live
test-failover-live: ## Live failover test: bad Gemini model → real provider fallback (requires a fallback key)
	@if [ -z "$(GROQ_API_KEY)" ] && [ -z "$(NVIDIA_API_KEY)" ] && \
	    [ -z "$(OPENROUTER_API_KEY)" ] && [ -z "$(HF_TOKEN)" ]; then \
	  echo ""; \
	  echo "  ERROR: no fallback provider configured."; \
	  echo "  Set at least one of:"; \
	  echo "    GROQ_API_KEY, NVIDIA_API_KEY, OPENROUTER_API_KEY, HF_TOKEN"; \
	  echo "  Or use 'make test-failover-echo' for a zero-key local demo."; \
	  echo ""; \
	  exit 1; \
	fi
	GOOGLE_MODEL=gemini-intentionally-broken $(GO) run $(GOFLAGS) . console

# test-failover-echo: demonstrates failover without any third-party API keys.
#
# How it works:
#   GOOGLE_MODEL=gemini-intentionally-broken  forces Gemini to return a 400 error
#   ECHO_FALLBACK_ENABLED=1                   appends the echo stub as last fallback
#
# Expected output in the console session:
#   WARN  echo fallback enabled — for local testing only; not for production
#   INFO  model chain  providers="failover(gemini-intentionally-broken → echo)"
#   WARN  failover: provider error, trying next  provider=gemini-...  error="..."
#   INFO  failover: recovered via fallback        provider=echo
#
# Only GOOGLE_API_KEY is required (so Gemini can be constructed; it will then fail).
.PHONY: test-failover-echo
test-failover-echo: ## Zero-key failover demo: bad Gemini → echo stub (no fallback keys needed)
	GOOGLE_MODEL=gemini-intentionally-broken ECHO_FALLBACK_ENABLED=1 $(GO) run $(GOFLAGS) . console

.PHONY: vet
vet: ## Run go vet
	$(GO) vet $(PKG)

.PHONY: lint
lint: ## Run golangci-lint (must be installed separately)
ifdef HAS_LINT
	golangci-lint run $(PKG)
else
	@echo "golangci-lint not found. Install with:"
	@echo "  go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"
	@exit 1
endif

# ── module maintenance ────────────────────────────────────────────────────────
.PHONY: tidy
tidy: ## Run go mod tidy
	$(GO) mod tidy

# ── clean ─────────────────────────────────────────────────────────────────────
.PHONY: clean
clean: ## Remove compiled binaries (BINARY and bin/)
	rm -f $(BINARY)
	rm -rf $(BIN_DIR)

# ── env ───────────────────────────────────────────────────────────────────────
# Print a human-readable summary of which API keys / overrides are configured.
.PHONY: env
env: ## Show configured environment variables (keys masked)
	@echo "── Required ───────────────────────────────────────────"
	@$(call show_var,GOOGLE_API_KEY)
	@$(call show_var,GOOGLE_MODEL)
	@echo ""
	@echo "── Groq ───────────────────────────────────────────────"
	@$(call show_var,GROQ_API_KEY)
	@$(call show_var,GROQ_MODEL)
	@echo ""
	@echo "── NVIDIA NIM ─────────────────────────────────────────"
	@$(call show_var,NVIDIA_API_KEY)
	@$(call show_var,NVIDIA_MODEL)
	@$(call show_var,NVIDIA_BASE_URL)
	@echo ""
	@echo "── OpenRouter ─────────────────────────────────────────"
	@$(call show_var,OPENROUTER_API_KEY)
	@$(call show_var,OPENROUTER_MODEL)
	@$(call show_var,OPENROUTER_SITE_URL)
	@$(call show_var,OPENROUTER_APP_NAME)
	@echo ""
	@echo "── HuggingFace ────────────────────────────────────────"
	@$(call show_var,HF_TOKEN)
	@$(call show_var,HF_MODEL)
	@$(call show_var,HF_ENDPOINT_URL)
	@echo ""
	@echo "── Local testing ──────────────────────────────────────"
	@$(call show_var,ECHO_FALLBACK_ENABLED)

# show_var: print VAR = <masked> if set, or VAR = (not set) if empty.
# Masks all but the first 6 chars of a key to avoid leaking secrets in logs.
define show_var
	@if [ -n "$($(1))" ]; then \
	  val="$($(1))"; \
	  masked=$$(echo "$$val" | cut -c1-6)"..."; \
	  printf "  %-26s = %s\n" "$(1)" "$$masked"; \
	else \
	  printf "  %-26s = (not set)\n" "$(1)"; \
	fi
endef

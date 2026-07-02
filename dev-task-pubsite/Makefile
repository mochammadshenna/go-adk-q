.PHONY: all build build-go build-ui dev-http dev-stdio clean lint test

BINARY   := dev-task-pubsite
VERSION  := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS  := -s -w -X github.com/msw/dev-task-pubsite/internal/server.Version=$(VERSION)
BUILD_DIR := .build

all: build

build: build-ui build-go

build-go:
	go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY) ./cmd/dev-task-pubsite

build-ui:
	@if [ -d ui ]; then \
		cd ui && npm install --silent && npm run build; \
	else \
		echo "ui/ not found — skipping UI build (placeholder HTML will be used)"; \
	fi

dev-http:
	@echo "Starting HTTP mode on :$${PORT:-9012} ..."
	@env $$(grep -v '^#' .env 2>/dev/null | xargs) go run ./cmd/dev-task-pubsite http

dev-stdio:
	@echo "Starting stdio mode ..."
	go run ./cmd/dev-task-pubsite stdio

lint:
	go vet ./...
	@which staticcheck > /dev/null 2>&1 && staticcheck ./... || true

test:
	go test ./... -race -timeout 30s

clean:
	rm -rf $(BUILD_DIR)
	@if [ -d ui/dist ]; then rm -rf ui/dist; fi

package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/msw/archipelago-hotels-mcp/internal/rate"
	"github.com/msw/archipelago-hotels-mcp/internal/repository"
	"github.com/msw/archipelago-hotels-mcp/internal/server"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	lvl := slog.LevelInfo
	if os.Getenv("DEBUG") == "1" {
		lvl = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl})))

	dbCfg := repository.ConfigFromEnv()
	pool, err := repository.NewPool(ctx, dbCfg)
	if err != nil {
		slog.Error("database connection failed", "error", err)
		slog.Warn("starting in DEGRADED mode — database unavailable")
	}
	if pool != nil {
		defer pool.Close()
	}

	rateSvc := rate.New(pool)

	switch os.Args[1] {
	case "stdio":
		runStdio(ctx, pool, rateSvc)
	case "http":
		runHTTP(ctx, pool, rateSvc)
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	os.Stderr.WriteString(`Archipelago Hotels MCP Server — search and explore hotels across Archipelago brands.

Usage:
  archipelago-hotels-mcp stdio                Run on stdio transport (Claude Desktop, Pi Agent)
  archipelago-hotels-mcp http                 Run on Streamable HTTP at :9011
  archipelago-hotels-mcp http -addr :PORT     Custom port
  archipelago-hotels-mcp http -verbose        Debug logging

Examples:
  archipelago-hotels-mcp stdio
  archipelago-hotels-mcp http
  archipelago-hotels-mcp http -addr :8080 -verbose
`)
}

func runStdio(ctx context.Context, pool *repository.Pool, rateSvc *rate.Service) {
	svc := server.New(pool, rateSvc)
	if err := server.RunStdio(ctx, svc.MCP); err != nil {
		slog.Error("stdio error", "error", err)
		os.Exit(1)
	}
}

func runHTTP(ctx context.Context, pool *repository.Pool, rateSvc *rate.Service) {
	addr := ":9011"
	verbose := false
	for i := 2; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "-addr":
			if i+1 < len(os.Args) {
				addr = os.Args[i+1]
				i++
			}
		case "-verbose":
			verbose = true
		}
	}
	svc := server.New(pool, rateSvc)
	if err := server.RunHTTP(ctx, svc, addr, verbose); err != nil {
		slog.Error("http error", "error", err)
		os.Exit(1)
	}
}

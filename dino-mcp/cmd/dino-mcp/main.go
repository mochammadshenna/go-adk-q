// dino-mcp — An MCP server with MCP Apps interactive UI.
//
// Usage:
//
//	# stdio mode (for Claude Desktop, Copilot, etc.)
//	dino-mcp stdio
//
//	# Streamable HTTP mode
//	dino-mcp http -addr :9010
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/msw/dino-mcp/internal/server"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	subcommand := os.Args[1]
	args := os.Args[2:]

	// Shared flags
	fs := flag.NewFlagSet("dino-mcp", flag.ExitOnError)
	addr := fs.String("addr", ":9010", "HTTP listen address (for http mode)")
	verbose := fs.Bool("verbose", false, "Enable verbose debug logging")
	version := fs.Bool("version", false, "Print version and exit")

	switch subcommand {
	case "stdio":
		fs.Parse(args)
		if *version {
			fmt.Printf("dino-mcp %s\n", server.Version)
			return
		}
		runStdio(*verbose)

	case "http", "streamable-http":
		fs.Parse(args)
		if *version {
			fmt.Printf("dino-mcp %s\n", server.Version)
			return
		}
		runHTTP(*addr, *verbose)

	case "help", "--help", "-h":
		printUsage()

	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: %s\n\n", subcommand)
		printUsage()
		os.Exit(1)
	}
}

func runStdio(verbose bool) {
	logger := newLogger(verbose)
	s := server.New(logger)
	ctx, cancel := signalCtx()
	defer cancel()

	if err := server.RunStdio(ctx, s, logger); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}

func runHTTP(addr string, verbose bool) {
	logger := newLogger(verbose)
	s := server.New(logger)
	ctx, cancel := signalCtx()
	defer cancel()

	if err := server.RunStreamableHTTP(ctx, s, addr, logger); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}

func newLogger(verbose bool) *slog.Logger {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	}))
}

func signalCtx() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `dino-mcp — MCP Server with Interactive Dinosaur Dashboard

USAGE:
  dino-mcp <subcommand> [flags]

SUBCOMMANDS:
  stdio             Run on stdin/stdout (for Claude Desktop, etc.)
  http              Run as Streamable HTTP server
  help              Print this help

FLAGS:
  -addr :9010       HTTP listen address (http mode)
  -verbose          Enable debug logging
  -version          Print version and exit

EXAMPLES:
  dino-mcp stdio
  dino-mcp http -addr :9010 -verbose

ENVIRONMENT:
  No API keys required — all data is built-in.
`)
}

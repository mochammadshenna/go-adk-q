package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/msw/dev-task-pubsite/internal/providers/gh"
	"github.com/msw/dev-task-pubsite/internal/providers/yt"
	"github.com/msw/dev-task-pubsite/internal/server"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	mode := "stdio"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}

	ytClient, ytErr := buildYTClient()
	ghClient, ghErr := buildGHClient()

	// Warn but don't exit — degraded mode is valid.
	if ytErr != nil {
		log.Printf("WARN: YouTrack unavailable: %v (yt_* tools disabled)", ytErr)
	}
	if ghErr != nil {
		log.Printf("WARN: GitHub unavailable: %v (gh_* tools disabled)", ghErr)
	}
	if ytErr != nil && ghErr != nil {
		log.Println("WARN: Both providers unavailable — server starts in fully degraded mode")
	}

	svc := server.New(ytClient, ghClient)

	switch mode {
	case "http":
		addr := ":" + port()
		if err := server.RunHTTP(ctx, svc, addr, debug()); err != nil {
			log.Fatalf("HTTP server error: %v", err)
		}
	case "stdio":
		if err := server.RunStdio(ctx, svc.MCP); err != nil {
			log.Fatalf("Stdio server error: %v", err)
		}
	default:
		fmt.Fprintf(os.Stderr, "usage: dev-task-pubsite [stdio|http]\n")
		os.Exit(1)
	}
}

func buildYTClient() (*yt.Client, error) {
	u := os.Getenv("YOUTRACK_URL")
	t := os.Getenv("YOUTRACK_TOKEN")
	if u == "" || t == "" {
		return nil, fmt.Errorf("YOUTRACK_URL and YOUTRACK_TOKEN required")
	}
	return yt.New(u, t)
}

func buildGHClient() (*gh.Client, error) {
	t := os.Getenv("GITHUB_TOKEN")
	if t == "" {
		return nil, fmt.Errorf("GITHUB_TOKEN required")
	}
	return gh.New(t)
}

func port() string {
	if p := os.Getenv("PORT"); p != "" {
		return p
	}
	return "9012"
}

func debug() bool {
	return os.Getenv("DEBUG") == "1" || os.Getenv("DEBUG") == "true"
}

package server

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	mcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/msw/dev-task-pubsite/internal/providers/gh"
	"github.com/msw/dev-task-pubsite/internal/providers/yt"
	"github.com/msw/dev-task-pubsite/internal/resources"
	"github.com/msw/dev-task-pubsite/internal/tools"
)

// Version is set at build time via -ldflags.
var Version = "dev"

// Service holds the wired MCP server and provider clients.
type Service struct {
	MCP      *mcp.Server
	YTClient *yt.Client
	GHClient *gh.Client
}

// New creates a fully wired MCP server. ytClient and ghClient may be nil when
// the respective provider is unavailable at startup.
func New(ytClient *yt.Client, ghClient *gh.Client) *Service {
	s := mcp.NewServer(
		&mcp.Implementation{
			Name:    "dev-task-pubsite",
			Version: Version,
		},
		&mcp.ServerOptions{
			Instructions: `Bridge between YouTrack tasks and GitHub code activity.

## YouTrack Tools
- list_my_tasks   — filter by project/sprint/state
- search_my_tasks — YouTrack query language full-text search
- get_my_sprint   — sprint summary with state breakdown
- create_task     — create a new task

## GitHub Tools
- list_pull_requests — list PRs by state
- get_pull_request   — PR detail with CI status and reviews
- list_issues        — GitHub issues (PRs excluded)
- create_issue       — create an issue
- repo_stats         — stars, forks, open issues/PRs, language
- commit_diff        — commit diff linked to YouTrack tasks

## Resilience
Providers degrade independently: stale cache is returned with
providerStatus.degraded=true and providerStatus.staleAge set.`,
		},
	)

	if ytClient != nil {
		tools.RegisterListTasks(s, ytClient)
		tools.RegisterSearchTasks(s, ytClient)
		tools.RegisterCreateTask(s, ytClient)
		tools.RegisterGetSprint(s, ytClient)
	}
	if ghClient != nil {
		tools.RegisterListPRs(s, ghClient)
		tools.RegisterGetPR(s, ghClient)
		tools.RegisterListIssues(s, ghClient)
		tools.RegisterCreateIssue(s, ghClient)
		tools.RegisterRepoStats(s, ghClient)
		tools.RegisterCommitDiff(s, ghClient)
	}

	resources.RegisterDashboardResource(s)

	return &Service{MCP: s, YTClient: ytClient, GHClient: ghClient}
}

// RunStdio runs the MCP server over stdin/stdout.
func RunStdio(ctx context.Context, s *mcp.Server) error {
	log.SetPrefix("[task-pubsite:stdio] ")
	log.SetFlags(log.Ltime | log.Lmsgprefix)
	log.Println("Dev Task Pubsite MCP server running on stdio")
	return s.Run(ctx, &mcp.StdioTransport{})
}

// RunHTTP starts the MCP server on Streamable HTTP using Gin.
func RunHTTP(ctx context.Context, svc *Service, addr string, verbose bool) error {
	if !verbose {
		gin.SetMode(gin.ReleaseMode)
		gin.DefaultWriter = io.Discard
	}

	r := gin.New()
	r.Use(gin.Recovery())
	if verbose {
		r.Use(gin.LoggerWithWriter(os.Stderr))
	}

	// Restrict CORS to known local origins. Wildcard is unsafe on a server that
	// holds provider tokens — any page could exfiltrate data cross-origin.
	allowedOrigins := map[string]bool{
		"http://localhost:6274":        true, // MCP Inspector default port
		"http://127.0.0.1:6274":       true,
		"http://localhost" + addr:      true, // dashboard's own origin
		"http://127.0.0.1" + addr:     true,
	}
	r.Use(func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			if !allowedOrigins[origin] {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
		}
		c.Header("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Mcp-Session-Id, Authorization")
		c.Header("Access-Control-Expose-Headers", "Mcp-Session-Id")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	})

	// JSONResponse: true → POST /mcp returns application/json (not text/event-stream),
	// making the server compatible with MCP Inspector and other clients that don't
	// implement SSE parsing in POST response bodies.
	mcpHandler := mcp.NewStreamableHTTPHandler(
		func(r *http.Request) *mcp.Server { return svc.MCP },
		&mcp.StreamableHTTPOptions{JSONResponse: true},
	)
	r.POST("/mcp", func(c *gin.Context) { mcpHandler.ServeHTTP(c.Writer, c.Request) })
	r.GET("/mcp", func(c *gin.Context) { mcpHandler.ServeHTTP(c.Writer, c.Request) })
	r.DELETE("/mcp", func(c *gin.Context) { mcpHandler.ServeHTTP(c.Writer, c.Request) })

	// Legacy SSE transport (2024-11-05 spec): GET /sse opens the SSE stream and
	// returns an endpoint event; subsequent POSTs go to /sse?sessionid=xxx.
	sseHandler := mcp.NewSSEHandler(
		func(r *http.Request) *mcp.Server { return svc.MCP },
		&mcp.SSEOptions{},
	)
	r.GET("/sse", func(c *gin.Context) { sseHandler.ServeHTTP(c.Writer, c.Request) })
	r.POST("/sse", func(c *gin.Context) { sseHandler.ServeHTTP(c.Writer, c.Request) })

	r.GET("/dashboard", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(resources.DashboardHTML()))
	})

	r.GET("/health", func(c *gin.Context) {
		_, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		status := "ok"
		if svc.YTClient == nil && svc.GHClient == nil {
			status = "degraded"
		}
		c.JSON(http.StatusOK, gin.H{
			"status":   status,
			"version":  Version,
			"youtrack": svc.YTClient != nil,
			"github":   svc.GHClient != nil,
		})
	})

	log.SetPrefix("[task-pubsite:http] ")
	log.SetFlags(log.Ltime | log.Lmsgprefix)

	toolCount := 0
	if svc.YTClient != nil {
		toolCount += 4
	}
	if svc.GHClient != nil {
		toolCount += 6
	}
	log.Printf("Dev Task Pubsite MCP server listening on %s (%d tools registered)", addr, toolCount)
	if svc.YTClient == nil {
		log.Printf("  [!] YouTrack tools absent — set YOUTRACK_URL + YOUTRACK_TOKEN")
	}
	if svc.GHClient == nil {
		log.Printf("  [!] GitHub tools absent — set GITHUB_TOKEN")
	}
	log.Printf("  Dashboard: http://localhost%s/dashboard", addr)
	log.Printf("  MCP (streamable): http://localhost%s/mcp", addr)
	log.Printf("  MCP (SSE legacy): http://localhost%s/sse", addr)
	log.Printf("  Health:    http://localhost%s/health", addr)

	srv := &http.Server{Addr: addr, Handler: r}
	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		log.Println("Shutting down gracefully...")
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutCtx)
	}
}

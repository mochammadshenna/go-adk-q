// Package server wires the dino-mcp MCP server — it is the composition root.
//
// Responsibilities:
//   - Create the *mcp.Server instance with all tools and resources registered
//   - Provide RunStdio() and RunStreamableHTTP() transport entry points
//   - Mount Gin HTTP routes (CORS, middleware, standalone dashboard, API, health)
//
// Tool and resource implementations live in their own packages:
//   - internal/tools/    — dino_think, dino_ask, dino_dashboard tool handlers
//   - internal/resources/ — ui://dino-dashboard/mcp-app.html + embedded HTML
package server

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/msw/dino-mcp/internal/resources"
	"github.com/msw/dino-mcp/internal/tools"
)

// Version is the server version, set at build time.
var Version = "0.1.0"

// Name is the server name.
const Name = "dino-mcp"

// New creates a fully configured MCP server with all tools and resources
// registered. Acts as the composition root — imports and wires sub-packages.
func New(logger *slog.Logger) *mcp.Server {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}

	s := mcp.NewServer(
		&mcp.Implementation{
			Name:    Name,
			Version: Version,
		},
		&mcp.ServerOptions{
			Instructions: `# Dino MCP Server

Interactive dinosaur tools with MCP Apps UI capabilities.

## Tools

1. dino_think: Returns a random dinosaur fact — great for inspiration
2. dino_ask: Ask anything about dinosaurs — returns structured information
3. dino_dashboard: Opens an interactive dinosaur dashboard with visual cards

## Usage Tips

- Use dino_think to start conversations with fun facts
- Use dino_ask for specific queries about species, periods, or diets
- Use dino_dashboard when the user wants a visual, interactive experience
`,
			Logger: logger,
		},
	)

	// Wire tools (each tool is a separate file in internal/tools/)
	tools.RegisterThink(s)
	tools.RegisterAsk(s)
	tools.RegisterDashboardTool(s)

	// Wire resources (internal/resources/)
	resources.RegisterDashboardResource(s, logger)

	logger.Info("dino-mcp server initialized",
		"tools", []string{"dino_think", "dino_ask", "dino_dashboard"},
		"resources", []string{"ui://dino-dashboard/mcp-app.html"},
	)

	return s
}

// RunStdio runs the server over stdin/stdout.
func RunStdio(ctx context.Context, s *mcp.Server, logger *slog.Logger) error {
	logger.Info("starting dino-mcp on stdio transport")
	return s.Run(ctx, &mcp.StdioTransport{})
}

// RunStreamableHTTP runs the server over Streamable HTTP transport using Gin.
func RunStreamableHTTP(ctx context.Context, s *mcp.Server, addr string, logger *slog.Logger) error {
	gin.SetMode(gin.ReleaseMode)
	if logger.Enabled(ctx, slog.LevelDebug) {
		gin.SetMode(gin.DebugMode)
	}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(requestLogger(logger))
	r.Use(corsMiddleware())

	// Streamable HTTP endpoint (MCP protocol transport)
	mcpHandler := mcp.NewStreamableHTTPHandler(
		func(r *http.Request) *mcp.Server { return s },
		&mcp.StreamableHTTPOptions{
			Logger:                     logger,
			DisableLocalhostProtection: true,
		},
	)
	r.Any("/mcp", gin.WrapH(mcpHandler))
	r.Any("/mcp/*any", gin.WrapH(mcpHandler))

	// Standalone dashboard (browser fallback for non-MCP hosts)
	r.GET("/dashboard", dashboardHandler)
	r.GET("/dashboard/*any", dashboardHandler)

	// API endpoint for standalone dashboard (JSON fallback)
	r.GET("/api/dinosaurs", dinosaursHandler)

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "server": Name, "version": Version})
	})

	httpServer := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	logger.Info("starting dino-mcp on streamable HTTP", "addr", addr)

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		httpServer.Shutdown(shutdownCtx)
	}()

	return httpServer.ListenAndServe()
}

// -- standalone HTTP handlers --

// dashboardHandler serves the dashboard HTML for browser access.
func dashboardHandler(c *gin.Context) {
	htmlData, err := resources.DashboardHTML()
	if err != nil {
		c.String(http.StatusInternalServerError, "dashboard not found")
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", htmlData)
}

// dinosaursHandler serves filtered dinosaur data as JSON.
func dinosaursHandler(c *gin.Context) {
	filter := c.Query("filter")
	dinos := tools.FilteredDinosaurs(filter)
	c.JSON(http.StatusOK, gin.H{
		"dinosaurs": dinos,
		"filter":    filter,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"total":     len(dinos),
	})
}

// -- Gin middleware --

// requestLogger adapts slog logging to Gin's middleware format.
func requestLogger(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		c.Next()
		logger.Debug("gin request",
			"method", c.Request.Method,
			"path", path,
			"status", c.Writer.Status(),
			"latency", time.Since(start),
		)
	}
}

// corsMiddleware handles CORS for browser-based clients (Inspector, Claude web).
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Accept, Origin, MCP-Session-Id")
		c.Header("Access-Control-Expose-Headers", "MCP-Session-Id, Content-Type")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusOK)
			return
		}
		c.Next()
	}
}

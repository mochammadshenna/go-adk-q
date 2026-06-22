package server

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
	mcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/msw/archipelago-hotels-mcp/internal/rate"
	"github.com/msw/archipelago-hotels-mcp/internal/repository"
	"github.com/msw/archipelago-hotels-mcp/internal/resources"
	"github.com/msw/archipelago-hotels-mcp/internal/tools"
)

// Version set at build time via -ldflags.
var Version = "dev"

// Service holds shared dependencies for the MCP server.
type Service struct {
	DB      *repository.Pool
	RateSvc *rate.Service
	MCP     *mcp.Server
}

// New creates a fully wired MCP server with DB-backed tools.
func New(dbPool *repository.Pool, rateSvc *rate.Service) *Service {
	s := mcp.NewServer(
		&mcp.Implementation{
			Name:    "archipelago-hotels-mcp",
			Version: Version,
		},
		&mcp.ServerOptions{
			Instructions: `# Archipelago Hotels MCP Server

Search, recommend, and explore hotels across 13 Archipelago brands (Aston, The Alana, Harper, NEO, favehotels, Kamuela, Huxley, and more).

## Tools

1. search_hotels: Search hotels by city, country, brand, or free-text query
2. get_hotel_detail: Get full details including all room types and prices
3. recommend_hotel: Get personalized recommendations by vibe, budget, and purpose
4. hotel_dashboard: Opens the interactive dashboard with filters

## Usage Tips

- Start with a destination: "Find hotels in Bali" → search_hotels
- Or get recommendations: "I want luxury hotels in Jakarta for business" → recommend_hotel
- Use hotel_dashboard for the visual interactive experience
`,
		},
	)

	svc := &Service{DB: dbPool, RateSvc: rateSvc, MCP: s}

	// Discover brand CDN hostnames for the iframe CSP resourceDomains allowlist.
	if dbPool != nil {
		dbPool.SetThumbnailDomains(dbPool.ThumbnailDomains(context.Background()))
	}

	tools.RegisterSearch(s, svc.DB, svc.RateSvc)
	tools.RegisterDetail(s, svc.DB, svc.RateSvc)
	tools.RegisterRecommend(s, svc.DB, svc.RateSvc)
	tools.RegisterDashboardTool(s, svc.DB, svc.RateSvc)
	tools.RegisterOpenURL(s)

	resources.RegisterDashboardResource(s, dbPool.ImageDomains())

	return svc
}

// RunStdio runs the server over stdin/stdout.
func RunStdio(ctx context.Context, s *mcp.Server) error {
	log.SetPrefix("[hotels-mcp:stdio] ")
	log.SetFlags(log.Ltime | log.Lmsgprefix)
	log.Println("Archipelago Hotels MCP Server running on stdio")
	return s.Run(ctx, &mcp.StdioTransport{})
}

// RunHTTP starts the server on Streamable HTTP using Gin.
func RunHTTP(ctx context.Context, svc *Service, addr string, verbose bool) error {
	if !verbose {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Recovery())

	if verbose {
		r.Use(gin.LoggerWithWriter(os.Stderr))
	}

	// CORS
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Mcp-Session-Id, Authorization")
		c.Header("Access-Control-Expose-Headers", "Mcp-Session-Id")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	})

	// MCP Streamable HTTP handler
	mcpHandler := mcp.NewStreamableHTTPHandler(
		func(r *http.Request) *mcp.Server { return svc.MCP },
		&mcp.StreamableHTTPOptions{},
	)

	r.POST("/mcp", func(c *gin.Context) { mcpHandler.ServeHTTP(c.Writer, c.Request) })
	r.GET("/mcp", func(c *gin.Context) { mcpHandler.ServeHTTP(c.Writer, c.Request) })

	// Standalone dashboard
	r.GET("/dashboard", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(resources.DashboardHTML()))
	})

	// API endpoints for standalone dashboard
	r.GET("/api/hotels", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		city := c.Query("city")
		brand := c.Query("brand")
		if len(city) > 100 {
			city = city[:100]
		}
		if len(brand) > 100 {
			brand = brand[:100]
		}

		hotels, _, err := svc.DB.SearchHotels(ctx, repository.SearchParams{
			City:  city,
			Brand: brand,
			Limit: 100,
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, hotels)
	})
	r.GET("/api/brands", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		brands, _, err := svc.DB.SearchHotels(ctx, repository.SearchParams{Limit: 100})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		// Extract unique brands from results.
		brandSet := make(map[string]bool)
		for _, h := range brands {
			brandSet[h.BrandName] = true
		}
		brandNames := make([]string, 0, len(brandSet))
		for b := range brandSet {
			brandNames = append(brandNames, b)
		}
		sort.Strings(brandNames)
		result := make([]map[string]string, 0, len(brandNames))
		for _, b := range brandNames {
			result = append(result, map[string]string{"brand": b})
		}
		c.JSON(http.StatusOK, result)
	})
	r.GET("/api/regions", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		regions, err := svc.DB.ListRegions(ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, regions)
	})
	r.GET("/health", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		healthErr := svc.DB.Health(ctx)
		status := "ok"
		if healthErr != nil {
			status = "degraded"
		}
		c.JSON(http.StatusOK, gin.H{
			"status":  status,
			"version": Version,
			"db":      healthErr == nil,
		})
	})

	info := fmt.Sprintf("Archipelago Hotels MCP Server listening on %s", addr)
	logPrefix := "[hotels-mcp:http] "
	log.SetPrefix(logPrefix)
	log.SetFlags(log.Ltime | log.Lmsgprefix)
	log.Println(info)
	log.Printf("  Dashboard: http://localhost%s/dashboard", addr)
	log.Printf("  API:       http://localhost%s/api/hotels", addr)
	log.Printf("  MCP:       http://localhost%s/mcp", addr)
	log.Printf("  Health:    http://localhost%s/health", addr)

	if !verbose {
		gin.DefaultWriter = io.Discard
	}

	return r.Run(addr)
}

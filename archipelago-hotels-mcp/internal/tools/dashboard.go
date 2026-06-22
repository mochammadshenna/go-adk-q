package tools

import (
	"context"
	"fmt"
	"log/slog"

	mcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/msw/archipelago-hotels-mcp/internal/rate"
	"github.com/msw/archipelago-hotels-mcp/internal/repository"
	"github.com/msw/archipelago-hotels-mcp/internal/resources"
)

// RegisterDashboardTool registers the find_hotels MCP tool.
func RegisterDashboardTool(s *mcp.Server, pool *repository.Pool, rateSvc *rate.Service) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "find_hotels",
		Description: "PRIORITY TOOL for browsing or booking Archipelago Hotels. Call this when user says: 'show all hotels', 'browse hotels', 'open hotel list', 'book a hotel', 'booking', 'all Archipelago hotels', or mentions any brand without a specific search — Aston, Harper, NEO, FAVE, Kamuela, Alana, Quest, PBA. Shows full visual hotel portfolio with booking options. Pass city or brand to filter; leave empty for all Indonesian properties.",
		Meta: mcp.Meta{
			"ui": map[string]any{
				"resourceUri":     resources.ResourceURI,
				"resourceDomains": []string{"images.archipelagohotels.com"},
			},
		},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"city":  map[string]any{"type": "string", "description": "City filter (optional)."},
				"brand": map[string]any{"type": "string", "description": "Brand filter (optional)."},
			},
		},
	}, dashboardHandler(pool, rateSvc))
}

type dashboardArgs struct {
	City  string `json:"city,omitempty"`
	Brand string `json:"brand,omitempty"`
}

type dashboardData struct {
	Filter  string          `json:"filter"`
	Hotels  []hotelSummary `json:"hotels"`
	Total   int             `json:"total"`
	Match   int             `json:"match"`
	Message string          `json:"message"`
}

func dashboardHandler(pool *repository.Pool, rateSvc *rate.Service) func(context.Context, *mcp.CallToolRequest, dashboardArgs) (*mcp.CallToolResult, dashboardData, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, args dashboardArgs) (res *mcp.CallToolResult, data dashboardData, err error) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("dashboard panic", "recover", r)
				err = fmt.Errorf("internal error: %v", r)
			}
		}()
		params := repository.SearchParams{
			City:  args.City,
			Brand: args.Brand,
			Limit: 200,
		}
		hotels, _, err := pool.SearchHotels(ctx, params)
		if err != nil {
			return nil, dashboardData{}, fmt.Errorf("search failed: %w", err)
		}

		// Fetch rates and thumbnails for all hotels in parallel.
		rateMap := rateSvc.BatchMinRates(ctx, hotels)
		thumbMap := pool.GetThumbnails(ctx, hotels)

		summaries := make([]hotelSummary, 0, len(hotels))
		for _, h := range hotels {
			priceFrom := h.StartingPrice
			if m, ok := rateMap[h.HotelID]; ok && m > 0 {
				priceFrom = m
			}
			summaries = append(summaries, hotelSummary{
				ID:         fmt.Sprintf("%d", h.HotelID),
				Name:       h.Name,
				Brand:      h.BrandName,
				City:       h.RegionName,
				Country:    "Indonesia",
				Rating:     h.Rating,
				Stars:      h.Stars,
				PriceFrom:  priceFrom,
				Currency:   h.Currency,
				ImageStyle: h.ImageStyle,
				BrandColor: h.BrandColor,
				Thumbnail:  thumbMap[h.HotelID],
				Tags:       deriveTags(h),
			})
		}

		msg := fmt.Sprintf("Showing %d hotels", len(summaries))
		if args.City != "" {
			msg += " in " + args.City
		}

		return nil, dashboardData{
			Filter:  args.City,
			Hotels:  summaries,
			Total:   len(summaries),
			Match:   len(summaries),
			Message: msg,
		}, nil
	}
}

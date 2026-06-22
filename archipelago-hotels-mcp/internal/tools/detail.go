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

// RegisterDetail registers the get_hotel_detail MCP tool as an app-only helper.
// Visibility "app" hides it from Claude's tool list; the dashboard UI calls it directly.
func RegisterDetail(s *mcp.Server, pool *repository.Pool, rateSvc *rate.Service) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_hotel_detail",
		Description: "Get detailed information about a specific hotel, including all room types, amenities, pricing, and location.",
		Meta: mcp.Meta{
			"ui": map[string]any{
				"resourceUri": resources.ResourceURI,
				"visibility":  []string{"app"},
				"csp": map[string]any{
					"resourceDomains": pool.ImageDomains(),
				},
			},
		},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"hotelId":  map[string]any{"type": "string", "description": "The hotel ID (e.g. aston-jakarta, harper-bali)."},
				"checkIn":  map[string]any{"type": "string", "description": "Check-in date YYYY-MM-DD. Defaults to today."},
				"checkOut": map[string]any{"type": "string", "description": "Check-out date YYYY-MM-DD. Defaults to tomorrow."},
			},
			"required": []string{"hotelId"},
		},
	}, detailHandler(pool, rateSvc))
}

type detailArgs struct {
	HotelID  string `json:"hotelId"`
	CheckIn  string `json:"checkIn,omitempty"`
	CheckOut string `json:"checkOut,omitempty"`
}

func detailHandler(pool *repository.Pool, rateSvc *rate.Service) func(context.Context, *mcp.CallToolRequest, detailArgs) (*mcp.CallToolResult, map[string]any, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, args detailArgs) (res *mcp.CallToolResult, out map[string]any, err error) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("detail panic", "recover", r)
				err = fmt.Errorf("internal error: %v", r)
			}
		}()
		var hotelID int
		if _, err := fmt.Sscanf(args.HotelID, "%d", &hotelID); err != nil {
			return nil, nil, fmt.Errorf("invalid hotel ID '%s': %w", args.HotelID, err)
		}

		h, err := pool.GetHotelByID(ctx, hotelID)
		if err != nil {
			return nil, nil, fmt.Errorf("database error: %w", err)
		}
		if h == nil {
			return nil, nil, fmt.Errorf("hotel not found: %s", args.HotelID)
		}

		thumbMap := pool.GetThumbnails(ctx, []repository.HotelRow{*h})
		bookingURL := ""
		if h.DBPrefix != "" && h.APIHotelID.Valid {
			bookingURL = pool.GetBookingURL(ctx, h.DBPrefix, int(h.APIHotelID.Int64))
		}

		detail := map[string]any{
			"id":         fmt.Sprintf("%d", h.HotelID),
			"name":       h.Name,
			"brand":      h.BrandName,
			"city":       h.RegionName,
			"country":    "Indonesia",
			"address":    h.Address,
			"rating":     h.Rating,
			"stars":      h.Stars,
			"latitude":   h.Latitude,
			"longitude":  h.Longitude,
			"currency":   h.Currency,
			"imageStyle": h.ImageStyle,
			"brandColor": h.BrandColor,
			"thumbnail":  thumbMap[h.HotelID],
			"bookingUrl": bookingURL,
		}

		if h.APIHotelID.Valid && h.DBPrefix != "" {
			rates, rErr := rateSvc.GetRates(ctx, h.DBPrefix, int(h.APIHotelID.Int64), args.CheckIn, args.CheckOut)
			if rErr == nil && len(rates) > 0 {
				roomTypes := make([]map[string]any, 0, len(rates))
				for _, r := range rates {
					roomTypes = append(roomTypes, map[string]any{
						"name":          r.Name,
						"pricePerNight": r.RatePerNight,
						"baseRate":      r.BaseRate,
						"currency":      h.Currency,
						"maxGuests":     2,
						"rateSource":    r.Source,
						"roomImage":     r.RoomImage,
					})
				}
				detail["roomTypes"] = roomTypes
				if info := rate.MinRateWithBase(rates); info.Rate > 0 {
					detail["startingPrice"] = info.Rate
					if info.BaseRate > info.Rate {
						detail["startingBasePrice"] = info.BaseRate
					}
				}
			}
		}

		if _, ok := detail["roomTypes"]; !ok && h.StartingPrice > 0 {
			detail["startingPrice"] = h.StartingPrice
		}

		return nil, detail, nil
	}
}

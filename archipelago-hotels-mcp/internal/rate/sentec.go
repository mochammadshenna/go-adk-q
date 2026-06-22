package rate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"
)

const sentecAPIEndpoint = "https://api.booking.sentec.io/sm/api/availability/search"

// sentecCreds returns the global Sentec API credentials.
// Override via SENTEC_USER / SENTEC_PASS environment variables for credential rotation.
func sentecCreds() (username, password string) {
	u := os.Getenv("SENTEC_USER")
	if u == "" {
		u = "website@archipelagointernational.com"
	}
	p := os.Getenv("SENTEC_PASS")
	if p == "" {
		p = "NllhBd0GHC7V2w"
	}
	return u, p
}

// SentechDayRate is the per-night rate breakdown from the Sentec REST API.
// base_rate = rack rate before discount; final_rate = displayed price after discount.
type SentechDayRate struct {
	BaseRate  float64 `json:"base_rate"`
	Discount  float64 `json:"discount"`
	FinalRate float64 `json:"final_rate"`
	TaxPrice  float64 `json:"tax_price"`
}

// sentecRequest is the JSON body for POST .../availability/search.
// Matches the Sentec booking engine API contract.
type sentecRequest struct {
	PropertyID  string             `json:"property_id"`
	Stay        sentecStay         `json:"stay"`
	Occupancies []sentecOccupancy  `json:"occupancies"`
	PromoCode   string             `json:"promo_code"`
}

type sentecStay struct {
	CheckIn  string `json:"check_in"`
	CheckOut string `json:"check_out"`
}

type sentecOccupancy struct {
	RoomIndex int           `json:"room_index"`
	Guests    []sentecGuest `json:"guests"`
}

type sentecGuest struct {
	Type string `json:"type"` // "AD" = adult, "CH" = child
}

type sentecResponse struct {
	Data struct {
		Rooms []sentecRoom `json:"rooms"`
	} `json:"data"`
}

type sentecRoom struct {
	RoomID    string           `json:"room_id"` // string, e.g. "1372"
	RoomName  string           `json:"room_name"`
	RatePlans []sentecRatePlan `json:"rate_plans"`
}

type sentecRatePlan struct {
	Rates map[string]SentechDayRate `json:"rates"` // keyed by date "YYYY-MM-DD"
}

// sentecRate holds a single room's best available rate from the Sentec API.
type sentecRate struct {
	RoomID    string // matches tb_hrooms.sentec_id (as string)
	RoomName  string
	BaseRate  float64 // rack rate before discount
	FinalRate float64 // display price shown on booking engine
}

// callSentecAPI POSTs to the Sentec availability search endpoint and returns
// the lowest FinalRate per room across all rate plans for the checkIn date.
func callSentecAPI(ctx context.Context, propID, checkIn, checkOut, user, pass string) ([]sentecRate, error) {
	body, err := json.Marshal(sentecRequest{
		PropertyID: propID,
		Stay:       sentecStay{CheckIn: checkIn, CheckOut: checkOut},
		Occupancies: []sentecOccupancy{{
			RoomIndex: 1,
			Guests:    []sentecGuest{{Type: "AD"}, {Type: "AD"}},
		}},
		PromoCode: "",
	})
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sentecAPIEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.SetBasicAuth(user, pass)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	slog.Debug("rate: sentec response", "status", resp.StatusCode, "body", string(respBody))

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d: %.200s", resp.StatusCode, respBody)
	}

	var result sentecResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	rates := make([]sentecRate, 0, len(result.Data.Rooms))
	for _, room := range result.Data.Rooms {
		best := pickBestSentecRate(room, checkIn)
		if best != nil {
			rates = append(rates, *best)
		}
	}
	return rates, nil
}

// pickBestSentecRate returns the lowest FinalRate across all rate plans for the checkIn date.
func pickBestSentecRate(room sentecRoom, checkIn string) *sentecRate {
	var best *sentecRate
	for _, rp := range room.RatePlans {
		dr, ok := rp.Rates[checkIn]
		if !ok || dr.FinalRate <= 0 {
			continue
		}
		if best == nil || dr.FinalRate < best.FinalRate {
			r := sentecRate{
				RoomID:    room.RoomID,
				RoomName:  room.RoomName,
				BaseRate:  dr.BaseRate,
				FinalRate: dr.FinalRate,
			}
			best = &r
		}
	}
	return best
}

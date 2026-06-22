// Package rate — room rate orchestration with multi-source fallback.
//
// Fallback chain: SimpleBooking live API → stored tb_hrooms.room_rate
//   → hotel_starting_price from central DB.
//
// Senetc REST API is reserved for future use (zero hotels currently use it).
package rate

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/msw/archipelago-hotels-mcp/internal/repository"
)

// RoomRate represents a single room's pricing from any source.
type RoomRate struct {
	Name         string  `json:"name"`
	RatePerNight float64 `json:"ratePerNight"`
	BaseRate     float64 `json:"baseRate,omitempty"` // before discount (SB only)
	Source       string  `json:"source"`             // "simplebooking", "stored", "starting_price"
}

// Service fetches room rates with automatic fallback and caching.
// Goroutine-safe: all shared state is locked internally.
type Service struct {
	pool    *repository.Pool
	sb      *SBClient
	cache   *rateCache
	timeNow func() time.Time
}

// New creates a rate service backed by the given DB pool.
func New(pool *repository.Pool) *Service {
	return &Service{
		pool:    pool,
		sb:      NewSBClient(),
		cache:   newRateCache(5 * time.Minute), // TTL: 5 minutes
		timeNow: time.Now,
	}
}

// GetRates returns all room rates for a hotel, trying live sources first.
// Results are cached for the configured TTL.
//
// apiHotelID is the per-brand DB's hotel_id.
// checkIn/checkOut format: "YYYY-MM-DD" (defaults to today/tomorrow if empty).
func (s *Service) GetRates(ctx context.Context, dbPrefix string, apiHotelID int, checkIn, checkOut string) ([]RoomRate, error) {
	if s.pool == nil {
		return nil, nil
	}

	key := cacheKey(dbPrefix, apiHotelID)

	// Check cache first.
	if cached, ok := s.cache.Get(key); ok {
		return cached, nil
	}

	if checkIn == "" || checkOut == "" {
		now := s.timeNow()
		checkIn = now.Format("2006-01-02")
		checkOut = now.Add(24 * time.Hour).Format("2006-01-02")
	}

	// 1. Try SimpleBooking live API.
	rates := s.trySB(ctx, dbPrefix, apiHotelID, checkIn, checkOut)
	if rates != nil {
		s.cache.Set(key, rates)
		return rates, nil
	}

	// 2. Fallback: stored tb_hrooms.room_rate.
	rates = s.tryStored(ctx, dbPrefix, apiHotelID)
	if rates != nil {
		s.cache.Set(key, rates)
		return rates, nil
	}

	// 3. No rates found. Cache empty to avoid repeated lookups.
	s.cache.Set(key, nil)
	return nil, nil
}

// trySB attempts to fetch live rates from SimpleBooking.
// Returns nil if credentials are missing or the API fails.
func (s *Service) trySB(ctx context.Context, dbPrefix string, apiHotelID int, checkIn, checkOut string) []RoomRate {
	creds, err := s.pool.GetCredentials(ctx, dbPrefix, apiHotelID)
	if err != nil {
		slog.Warn("rate: credentials fetch failed", "prefix", dbPrefix, "hotel_id", apiHotelID, "error", err)
		return nil
	}
	if creds == nil || !s.sb.Enabled() || !credsHaveSB(creds) {
		return nil
	}

	sbRates, sbErr := s.sb.GetRates(ctx, SBRequest{
		StartDate: checkIn,
		EndDate:   checkOut,
		Username:  creds.SimpleBookingUser,
		Password:  creds.SimpleBookingPass,
		SBID:      creds.SimpleBookingID,
	})
	if sbErr != nil {
		slog.Warn("rate: simplebooking failed, falling back", "prefix", dbPrefix, "hotel_id", apiHotelID, "error", sbErr)
		return nil
	}
	if len(sbRates) == 0 {
		return nil
	}

	slog.Info("rate: simplebooking live", "prefix", dbPrefix, "hotel_id", apiHotelID, "rooms", len(sbRates))
	result := make([]RoomRate, len(sbRates))
	for i, r := range sbRates {
		result[i] = RoomRate{
			Name:         r.RoomName,
			RatePerNight: r.TotalAfterTax,
			BaseRate:     r.AmountAfterTax,
			Source:       "simplebooking",
		}
	}
	return result
}

// tryStored fetches rates from tb_hrooms.room_rate as fallback.
func (s *Service) tryStored(ctx context.Context, dbPrefix string, apiHotelID int) []RoomRate {
	rooms, err := s.pool.GetRooms(ctx, dbPrefix, apiHotelID)
	if err != nil {
		slog.Warn("rate: stored rooms fetch failed", "prefix", dbPrefix, "hotel_id", apiHotelID, "error", err)
		return nil
	}
	if len(rooms) == 0 {
		return nil
	}

	result := make([]RoomRate, 0, len(rooms))
	for _, r := range rooms {
		if r.Rate > 0 {
			result = append(result, RoomRate{
				Name:         r.Name,
				RatePerNight: r.Rate,
				Source:       "stored",
			})
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// MinRate returns the lowest rate from a list of room rates.
// Returns 0 if the list is empty.
func MinRate(rates []RoomRate) float64 {
	var min float64
	for _, r := range rates {
		if min == 0 || r.RatePerNight < min {
			min = r.RatePerNight
		}
	}
	return min
}

// BatchMinRates fetches the minimum rate for each hotel in parallel.
// Uses a bounded goroutine pool (maxWorkers) to avoid resource exhaustion.
// Returns a map hotelID → minimum price (0 = not found).
// Context cancellation stops all pending goroutines.
func (s *Service) BatchMinRates(ctx context.Context, hotels []repository.HotelRow) map[int]float64 {
	if len(hotels) == 0 {
		return nil
	}

	// Build list of hotels that need rate lookup from brand DB.
	type rateReq struct {
		hotelID   int
		dbPrefix  string
		apiID     int
		startFrom float64 // StartingPrice fallback
	}
	var reqs []rateReq
	for _, h := range hotels {
		if h.APIHotelID.Valid && h.DBPrefix != "" {
			reqs = append(reqs, rateReq{
				hotelID:   h.HotelID,
				dbPrefix:  h.DBPrefix,
				apiID:     int(h.APIHotelID.Int64),
				startFrom: h.StartingPrice,
			})
		}
	}
	if len(reqs) == 0 {
		return nil
	}

	// maxWorkers: 5 concurrent fetches — enough for ~50 hotels in ~10 batches.
	const maxWorkers = 5

	type result struct {
		hotelID int
		minRate float64
	}

	sem := make(chan struct{}, maxWorkers)
	results := make(chan result, len(reqs))
	var wg sync.WaitGroup

	for _, r := range reqs {
		wg.Add(1)
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			wg.Done()
			continue
		}

		go func(r rateReq) {
			defer wg.Done()
			defer func() { <-sem }()

			rates, err := s.GetRates(ctx, r.dbPrefix, r.apiID, "", "")
			if err != nil || len(rates) == 0 {
				// Fallback: use StartingPrice from central DB.
				if r.startFrom > 0 {
					results <- result{r.hotelID, r.startFrom}
				}
				return
			}
			if m := MinRate(rates); m > 0 {
				results <- result{r.hotelID, m}
			} else if r.startFrom > 0 {
				results <- result{r.hotelID, r.startFrom}
			}
		}(r)
	}

	// Close results channel when all goroutines finish.
	go func() {
		wg.Wait()
		close(results)
	}()

	m := make(map[int]float64, len(reqs))
	for r := range results {
		m[r.hotelID] = r.minRate
	}
	return m
}

// SBRequest carries parameters for the SimpleBooking XML API call.
type SBRequest struct {
	StartDate string // YYYY-MM-DD
	EndDate   string // YYYY-MM-DD
	Username  string // XMLHotelAgent Name
	Password  string // XMLHotelAgent Pwd
	SBID      int    // Filter HotelCode
}

// SBClient talks to the SimpleBooking XML API.
type SBClient struct {
	endpoint    string
	provider    string
	providerPwd string
	httpDo      func(req any) ([]byte, error)
	cb          circuitBreaker
}

// SBRate is a single room rate from SimpleBooking.
type SBRate struct {
	RoomName       string
	AmountAfterTax float64
	TotalAfterTax  float64
}

// NewSBClient creates a client targeting the production SimpleBooking endpoint.
func NewSBClient() *SBClient {
	return &SBClient{
		endpoint:    "https://xml.simplebooking.it/xmlservice.asmx/HotelAvailRQ",
		provider:    providerUsername,
		providerPwd: providerPassword,
	}
}

// Enabled reports whether the circuit breaker allows outbound calls.
func (c *SBClient) Enabled() bool { return c.cb.Allow() }

// GetRates calls the SimpleBooking XML API and parses room rates.
func (c *SBClient) GetRates(ctx context.Context, req SBRequest) ([]SBRate, error) {
	if !c.cb.Allow() {
		return nil, fmt.Errorf("simplebooking: circuit breaker open")
	}

	xmlBody := buildSBXML(req, c.provider, c.providerPwd)
	respBody, err := postXML(ctx, c.endpoint, xmlBody)
	if err != nil {
		c.cb.Failure()
		return nil, fmt.Errorf("simplebooking: request: %w", err)
	}

	rates, err := parseSBResponse(respBody)
	if err != nil {
		c.cb.Failure()
		return nil, fmt.Errorf("simplebooking: parse: %w", err)
	}

	c.cb.Success()
	return rates, nil
}

// circuitBreaker limits SimpleBooking retries.
//
// ponytail: simple time-window circuit breaker. Upgrades to sliding-window if
// false positives are observed (transient errors in burst, then success).
type circuitBreaker struct {
	mu          sync.Mutex
	failures    int
	lastFailure time.Time
	state       string
}

const (
	maxFailures = 5
	cooldown    = 120 * time.Second
)

func (cb *circuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if cb.state == "open" && time.Since(cb.lastFailure) > cooldown {
		cb.state = "closed"
		cb.failures = 0
	}
	return cb.state != "open"
}

func (cb *circuitBreaker) Failure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures++
	cb.lastFailure = time.Now()
	if cb.failures >= maxFailures {
		cb.state = "open"
		slog.Warn("simplebooking: circuit breaker opened", "failures", cb.failures)
	}
}

func (cb *circuitBreaker) Success() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures = 0
}

func credsHaveSB(c *repository.BrandCredentials) bool {
	return c != nil && c.SimpleBookingID > 0 && c.SimpleBookingUser != "" && c.SimpleBookingPass != ""
}

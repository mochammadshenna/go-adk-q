// Package tools provides ADK FunctionTools for the go-adk-q demo agent.
//
// Weather data: wttr.in (free, no API key, worldwide coverage).
// Time data: wttr.in for lat/lon → timeapi.io for timezone + current time.
// No hardcoded city maps. Any city name the LLM passes works.
package tools

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

var httpClient = &http.Client{Timeout: 10 * time.Second}

func getJSON(apiURL string, out any) error {
	resp, err := httpClient.Get(apiURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}

// ── wttr.in response (shared by both tools) ───────────────────────────────────

type wttrResponse struct {
	NearestArea []struct {
		Latitude  string `json:"latitude"`
		Longitude string `json:"longitude"`
	} `json:"nearest_area"`
	CurrentCondition []struct {
		TempC         string `json:"temp_C"`
		FeelsLikeC    string `json:"FeelsLikeC"`
		Humidity      string `json:"humidity"`
		WindspeedKmph string `json:"windspeedKmph"`
		WeatherDesc   []struct {
			Value string `json:"value"`
		} `json:"weatherDesc"`
	} `json:"current_condition"`
}

func fetchWttr(city string) (*wttrResponse, error) {
	var w wttrResponse
	err := getJSON(fmt.Sprintf("https://wttr.in/%s?format=j1", url.PathEscape(city)), &w)
	if err != nil {
		return nil, fmt.Errorf("wttr.in fetch for %q: %w", city, err)
	}
	if len(w.CurrentCondition) == 0 {
		return nil, fmt.Errorf("wttr.in returned no data for %q", city)
	}
	return &w, nil
}

// ── Weather Tool ──────────────────────────────────────────────────────────────

type getWeatherArgs struct {
	City string `json:"city" jsonschema:"The name of the city to get current weather for."`
}

type getWeatherResult struct {
	City        string `json:"city"`
	Temperature string `json:"temperature"`
	FeelsLike   string `json:"feels_like"`
	Condition   string `json:"condition"`
	Humidity    string `json:"humidity"`
	WindSpeed   string `json:"wind_speed"`
}

func getWeather(_ tool.Context, args getWeatherArgs) (getWeatherResult, error) {
	w, err := fetchWttr(strings.TrimSpace(args.City))
	if err != nil {
		return getWeatherResult{}, err
	}
	c := w.CurrentCondition[0]
	condition := ""
	if len(c.WeatherDesc) > 0 {
		condition = c.WeatherDesc[0].Value
	}
	return getWeatherResult{
		City:        args.City,
		Temperature: c.TempC + "°C",
		FeelsLike:   c.FeelsLikeC + "°C",
		Condition:   condition,
		Humidity:    c.Humidity + "%",
		WindSpeed:   c.WindspeedKmph + " km/h",
	}, nil
}

func NewWeatherTool() tool.Tool {
	t, err := functiontool.New(functiontool.Config{
		Name:        "get_weather",
		Description: "Returns the current real weather for any city worldwide.",
	}, getWeather)
	if err != nil {
		panic(fmt.Sprintf("NewWeatherTool: %v", err))
	}
	return t
}

// ── Time Tool ─────────────────────────────────────────────────────────────────

type getCurrentTimeArgs struct {
	City string `json:"city" jsonschema:"The name of the city to get the current local time for."`
}

type getCurrentTimeResult struct {
	City     string `json:"city"`
	DateTime string `json:"datetime"`
	Timezone string `json:"timezone"`
}

type timeAPIResponse struct {
	DateTime string `json:"dateTime"`
	TimeZone string `json:"timeZone"`
}

func getCurrentTime(_ tool.Context, args getCurrentTimeArgs) (getCurrentTimeResult, error) {
	// Step 1: get lat/lon from wttr.in — works for any city name worldwide.
	w, err := fetchWttr(strings.TrimSpace(args.City))
	if err != nil {
		return getCurrentTimeResult{}, err
	}
	if len(w.NearestArea) == 0 {
		return getCurrentTimeResult{}, fmt.Errorf("no coordinates found for %q", args.City)
	}
	lat := w.NearestArea[0].Latitude
	lon := w.NearestArea[0].Longitude

	// Step 2: resolve timezone + current time from coordinates via timeapi.io.
	var tr timeAPIResponse
	err = getJSON(fmt.Sprintf(
		"https://timeapi.io/api/time/current/coordinate?latitude=%s&longitude=%s", lat, lon,
	), &tr)
	if err != nil || tr.DateTime == "" {
		return getCurrentTimeResult{}, fmt.Errorf("time lookup failed for %q: %w", args.City, err)
	}

	// Parse and reformat.
	t, err := time.Parse("2006-01-02T15:04:05.999999999", tr.DateTime)
	if err != nil {
		t, _ = time.Parse("2006-01-02T15:04:05", tr.DateTime)
	}

	return getCurrentTimeResult{
		City:     args.City,
		DateTime: t.Format("Monday, 02 Jan 2006 15:04:05"),
		Timezone: tr.TimeZone,
	}, nil
}

func NewTimeTool() tool.Tool {
	t, err := functiontool.New(functiontool.Config{
		Name:        "get_current_time",
		Description: "Returns the current real local time for any city worldwide.",
	}, getCurrentTime)
	if err != nil {
		panic(fmt.Sprintf("NewTimeTool: %v", err))
	}
	return t
}

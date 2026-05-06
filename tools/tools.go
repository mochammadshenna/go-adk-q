// Package tools provides ADK FunctionTools for the go-adk-q demo agent.
//
// Each tool is a typed Go function wrapped by functiontool.New.
// Args structs use `json` + `jsonschema` struct tags so that ADK auto-generates
// the correct JSON schema that the LLM reads to decide when and how to call a tool.
//
// Pattern:
//
//	type myArgs struct {
//	    Field string `json:"field" jsonschema:"Description for the LLM."`
//	}
//	func myFn(_ tool.Context, args myArgs) (ReturnType, error) { ... }
//	tool, _ := functiontool.New(functiontool.Config{Name: "...", Description: "..."}, myFn)
package tools

import (
	"fmt"
	"strings"
	"time"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

// ── Weather Tool ─────────────────────────────────────────────────────────────

// getWeatherArgs defines the required parameters for get_weather.
// Fields without `json:",omitempty"` are required — the LLM must supply them.
type getWeatherArgs struct {
	City string `json:"city" jsonschema:"The name of the city for which to retrieve the weather report."`
}

// getWeatherResult is the structured return type. Returning a struct (not string)
// gives the LLM richer, schema-typed data to reason about.
type getWeatherResult struct {
	City   string `json:"city"`
	Status string `json:"status"`
	Report string `json:"report"`
}

// getWeather returns a simulated weather report.
// In production, replace with a live weather API call (e.g., OpenWeatherMap).
// The first parameter is tool.Context — use it to access session state or artifacts.
func getWeather(_ tool.Context, args getWeatherArgs) (getWeatherResult, error) {
	reports := map[string]string{
		"new york":    "Cloudy, 18°C (64°F), humidity 72%, wind 15 km/h NW",
		"london":      "Light rain, 12°C (54°F), humidity 85%, wind 20 km/h SW",
		"tokyo":       "Sunny, 25°C (77°F), humidity 60%, wind 8 km/h E",
		"sydney":      "Partly cloudy, 22°C (72°F), humidity 65%, wind 12 km/h NE",
		"paris":       "Overcast, 15°C (59°F), humidity 78%, wind 10 km/h SE",
		"san francisco": "Foggy, 16°C (61°F), humidity 90%, wind 18 km/h W",
	}
	city := strings.ToLower(strings.TrimSpace(args.City))
	report, ok := reports[city]
	if !ok {
		report = fmt.Sprintf("Weather data for %q is not available in the simulation.", args.City)
	}
	return getWeatherResult{City: args.City, Status: "success", Report: report}, nil
}

// NewWeatherTool creates and returns the get_weather ADK FunctionTool.
// Panics at startup (not at runtime) if construction fails — fail-fast pattern.
func NewWeatherTool() tool.Tool {
	t, err := functiontool.New(functiontool.Config{
		Name:        "get_weather",
		Description: "Retrieves the current weather report for a specified city.",
	}, getWeather)
	if err != nil {
		panic(fmt.Sprintf("NewWeatherTool: %v", err))
	}
	return t
}

// ── Time Tool ─────────────────────────────────────────────────────────────────

// getCurrentTimeArgs defines the required parameter for get_current_time.
type getCurrentTimeArgs struct {
	City string `json:"city" jsonschema:"The name of the city for which to retrieve the current local time."`
}

// getCurrentTimeResult is the structured return type for the time tool.
type getCurrentTimeResult struct {
	City     string `json:"city"`
	DateTime string `json:"datetime"`
	Timezone string `json:"timezone"`
}

// cityUTCOffset maps common cities to their UTC hour offset.
// A real implementation should use the IANA timezone database.
var cityUTCOffset = map[string]int{
	"new york":      -5,
	"los angeles":   -8,
	"chicago":       -6,
	"london":        0,
	"paris":         1,
	"berlin":        1,
	"amsterdam":     1,
	"tokyo":         9,
	"seoul":         9,
	"beijing":       8,
	"singapore":     8,
	"sydney":        10,
	"dubai":         4,
	"mumbai":        5,
	"san francisco": -8,
}

// getCurrentTime returns the simulated local time for a given city using a
// fixed UTC offset. For production, use time.LoadLocation with the IANA DB.
func getCurrentTime(_ tool.Context, args getCurrentTimeArgs) (getCurrentTimeResult, error) {
	city := strings.ToLower(strings.TrimSpace(args.City))
	offset, known := cityUTCOffset[city]
	utcNow := time.Now().UTC()

	var localTime time.Time
	var tzLabel string
	if known {
		localTime = utcNow.Add(time.Duration(offset) * time.Hour)
		tzLabel = fmt.Sprintf("UTC%+d", offset)
	} else {
		localTime = utcNow
		tzLabel = "UTC (city offset unknown)"
	}

	return getCurrentTimeResult{
		City:     args.City,
		DateTime: localTime.Format("2006-01-02 15:04:05 MST"),
		Timezone: tzLabel,
	}, nil
}

// NewTimeTool creates and returns the get_current_time ADK FunctionTool.
func NewTimeTool() tool.Tool {
	t, err := functiontool.New(functiontool.Config{
		Name:        "get_current_time",
		Description: "Returns the current local time in the specified city.",
	}, getCurrentTime)
	if err != nil {
		panic(fmt.Sprintf("NewTimeTool: %v", err))
	}
	return t
}

// Package tools defines all MCP tool registrations for the dino-mcp server.
//
// Each file in this package owns one tool:
//   - think.go  → dino_think (random dinosaur fact)
//   - ask.go    → dino_ask (question answering)
//   - dashboard.go → dino_dashboard (MCP App dashboard trigger)
//
// The tools package owns the shared domain types and the resource URI
// constants that the resources package imports. The server package (composition
// root) calls Register* functions to wire tools into the server instance.
package tools

// MCP Apps resource identifiers — shared between tools (who reference the URI
// in _meta.ui.resourceUri) and resources (who satisfy the URI with HTML).
const (
	ResourceURI      = "ui://dino-dashboard/mcp-app.html"
	ResourceMIMEType = "text/html;profile=mcp-app"
)

// -- tool argument types --

// DinoAskArgs is the single argument for the dino_ask tool.
type DinoAskArgs struct {
	Question string `json:"question" jsonschema:"The dinosaur question to answer."`
}

// DinoDashboardArgs is the optional filter for the dashboard tool.
type DinoDashboardArgs struct {
	Filter string `json:"filter,omitempty" jsonschema:"Optional filter for dinosaur types (e.g., 'carnivore', 'herbivore', 'jurassic')."`
}

// -- tool result types --

// DinoThinkResult returns a random fact and the species it's about.
type DinoThinkResult struct {
	Fact    string `json:"fact" jsonschema:"A fun dinosaur fact."`
	Species string `json:"species" jsonschema:"The dinosaur species this fact is about."`
}

// DinoAskResult returns the answer to a dinosaur question.
type DinoAskResult struct {
	Question string `json:"question" jsonschema:"The original question."`
	Answer   string `json:"answer" jsonschema:"The detailed answer."`
	Species  string `json:"species,omitempty" jsonschema:"The dinosaur species referenced, if any."`
}

// DinoDashboardResult is the structured payload the dashboard UI renders.
type DinoDashboardResult struct {
	Filter    string        `json:"filter" jsonschema:"Applied filter."`
	Dinosaurs []DinoSummary `json:"dinosaurs" jsonschema:"List of matching dinosaurs."`
	Timestamp string        `json:"timestamp" jsonschema:"ISO 8601 timestamp of the data."`
}

// DinoSummary is one row in the dashboard card grid — every field is consumed
// by the TypeScript View (mcp-app.ts).
type DinoSummary struct {
	Name       string `json:"name" jsonschema:"Dinosaur name."`
	Period     string `json:"period" jsonschema:"Geological period."`
	Diet       string `json:"diet" jsonschema:"Diet type."`
	Length     string `json:"length" jsonschema:"Length description."`
	Weight     string `json:"weight" jsonschema:"Weight description."`
	FunFact    string `json:"funFact" jsonschema:"A fun fact."`
	ImageStyle string `json:"imageStyle" jsonschema:"CSS style hint for image rendering."`
}

// -- helpers shared across tools --

// ifEmpty returns the fallback when s is empty.
func ifEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// dinoData is the built-in dinosaur database — 12 species with period, diet,
// dimensions, fun facts, and a CSS style hint for the UI avatar.
var dinoData = []DinoSummary{
	{
		Name: "Tyrannosaurus Rex", Period: "Cretaceous", Diet: "Carnivore",
		Length: "40 ft (12 m)", Weight: "9 tons (8,000 kg)",
		FunFact:    "T-Rex had the strongest bite force of any land animal ever — over 12,000 pounds!",
		ImageStyle: "bg-red-900",
	},
	{
		Name: "Triceratops", Period: "Cretaceous", Diet: "Herbivore",
		Length: "30 ft (9 m)", Weight: "12 tons (11,000 kg)",
		FunFact:    "Triceratops had up to 800 teeth in its lifetime, arranged in battery-like columns.",
		ImageStyle: "bg-green-800",
	},
	{
		Name: "Stegosaurus", Period: "Jurassic", Diet: "Herbivore",
		Length: "30 ft (9 m)", Weight: "5 tons (4,500 kg)",
		FunFact:    "Stegosaurus had a brain the size of a walnut — only about 80 grams!",
		ImageStyle: "bg-amber-800",
	},
	{
		Name: "Velociraptor", Period: "Cretaceous", Diet: "Carnivore",
		Length: "6 ft (1.8 m)", Weight: "33 lbs (15 kg)",
		FunFact:    "Velociraptors were feathered and about the size of a turkey — much smaller than in the movies.",
		ImageStyle: "bg-orange-800",
	},
	{
		Name: "Brachiosaurus", Period: "Jurassic", Diet: "Herbivore",
		Length: "85 ft (26 m)", Weight: "62 tons (56,000 kg)",
		FunFact:    "Brachiosaurus held its neck like a giraffe, reaching heights of up to 40 feet.",
		ImageStyle: "bg-teal-800",
	},
	{
		Name: "Spinosaurus", Period: "Cretaceous", Diet: "Carnivore",
		Length: "59 ft (18 m)", Weight: "20 tons (18,000 kg)",
		FunFact:    "Spinosaurus was the largest carnivorous dinosaur — even bigger than T-Rex!",
		ImageStyle: "bg-blue-900",
	},
	{
		Name: "Ankylosaurus", Period: "Cretaceous", Diet: "Herbivore",
		Length: "30 ft (9 m)", Weight: "8 tons (7,200 kg)",
		FunFact:    "Ankylosaurus had a tail club that could swing with enough force to shatter bone.",
		ImageStyle: "bg-gray-700",
	},
	{
		Name: "Diplodocus", Period: "Jurassic", Diet: "Herbivore",
		Length: "92 ft (28 m)", Weight: "25 tons (22,700 kg)",
		FunFact:    "Diplodocus could grow longer than a basketball court with its whip-like tail.",
		ImageStyle: "bg-emerald-800",
	},
	{
		Name: "Parasaurolophus", Period: "Cretaceous", Diet: "Herbivore",
		Length: "30 ft (9 m)", Weight: "3 tons (2,700 kg)",
		FunFact:    "Parasaurolophus had a long crest that may have been used like a trumpet to communicate.",
		ImageStyle: "bg-indigo-800",
	},
	{
		Name: "Pachycephalosaurus", Period: "Cretaceous", Diet: "Herbivore",
		Length: "15 ft (4.5 m)", Weight: "990 lbs (450 kg)",
		FunFact:    "Its 10-inch thick skull could withstand head-butting contests at speeds of 15 mph!",
		ImageStyle: "bg-stone-700",
	},
	{
		Name: "Raptor", Period: "Cretaceous", Diet: "Carnivore",
		Length: "6 ft (1.8 m)", Weight: "33 lbs (15 kg)",
		FunFact:    "Raptors had a sickle-shaped claw on each foot used for gripping prey.",
		ImageStyle: "bg-rose-800",
	},
	{
		Name: "Maiasaura", Period: "Cretaceous", Diet: "Herbivore",
		Length: "30 ft (9 m)", Weight: "3 tons (2,700 kg)",
		FunFact:    "Maiasaura means 'good mother lizard' — evidence shows they cared for their young in nesting colonies.",
		ImageStyle: "bg-lime-800",
	},
}

// RegisterDashboardTool registers the dino_dashboard tool — an MCP App-enhanced
// tool with _meta.ui.resourceUri linking to the dashboard resource.
func RegisterDashboardTool(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "dino_dashboard",
		Description: "Opens an interactive dinosaur dashboard with searchable dinosaurs, fun facts, and visual cards. The dashboard supports filtering by period, diet, and type.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"filter": map[string]any{
					"type":        "string",
					"description": "Optional filter: 'carnivore', 'herbivore', 'jurassic', 'cretaceous', 'triassic', or leave empty for all.",
				},
			},
		},
		Meta: mcp.Meta{
			"ui": map[string]any{
				"resourceUri": ResourceURI,
			},
		},
	}, func(_ context.Context, _ *mcp.CallToolRequest, args DinoDashboardArgs) (*mcp.CallToolResult, DinoDashboardResult, error) {
		dinos := FilteredDinosaurs(args.Filter)
		return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{
						Text: fmt.Sprintf("Displaying dinosaur dashboard with %d dinosaurs (filter: %s)",
							len(dinos), ifEmpty(args.Filter, "all")),
					},
				},
			}, DinoDashboardResult{
				Filter:    args.Filter,
				Dinosaurs: dinos,
				Timestamp: time.Now().UTC().Format(time.RFC3339),
			}, nil
	})
}

// FilteredDinosaurs returns dinosaurs matching the optional filter.
// Filter can match by period, diet, or name. Exported for use by the
// standalone HTTP API endpoint in the server package.
func FilteredDinosaurs(filter string) []DinoSummary {
	if filter == "" {
		return dinoData
	}
	fl := toLower(filter)
	var result []DinoSummary
	for _, d := range dinoData {
		if contains(toLower(d.Diet), fl) ||
			contains(toLower(d.Period), fl) ||
			contains(toLower(d.Name), fl) {
			result = append(result, d)
		}
	}
	return result
}

// -- string helpers (no dependency on strings package) --

func toLower(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		b[i] = c
	}
	return string(b)
}

func contains(s, substr string) bool {
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			if s[i+j] != substr[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

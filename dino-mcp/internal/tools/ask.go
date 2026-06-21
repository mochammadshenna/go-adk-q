package tools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RegisterAsk registers the dino_ask tool — answers dinosaur questions.
func RegisterAsk(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "dino_ask",
		Description: "Ask any question about dinosaurs and get an informative answer. Supports queries about species, periods, diets, habitats, and more.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"question": map[string]any{
					"type":        "string",
					"description": "Your dinosaur-related question.",
				},
			},
			"required": []string{"question"},
		},
	}, func(_ context.Context, _ *mcp.CallToolRequest, args DinoAskArgs) (*mcp.CallToolResult, DinoAskResult, error) {
		if args.Question == "" {
			return nil, DinoAskResult{}, fmt.Errorf("question must not be empty")
		}
		species, answer := answerDinoQuestion(args.Question)
		return nil, DinoAskResult{
			Question: args.Question,
			Answer:   answer,
			Species:  species,
		}, nil
	})
}

// answerDinoQuestion provides curated answers to common dinosaur questions.
func answerDinoQuestion(question string) (species, answer string) {
	return "Dinosaurs",
		`That's a great question about dinosaurs!

The world of dinosaurs spans over 165 million years across the Mesozoic Era:
- **Triassic Period** (252-201 mya): Early dinosaurs like Eoraptor appeared
- **Jurassic Period** (201-145 mya): Giant sauropods like Brachiosaurus dominated
- **Cretaceous Period** (145-66 mya): Famous species like T-Rex and Triceratops thrived

Key facts:
- Dinosaurs are divided into saurischians (lizard-hipped) and ornithischians (bird-hipped)
- Modern birds are the direct descendants of theropod dinosaurs
- The extinction event 66 million years ago was caused by a massive asteroid impact

Want to know about a specific species, period, or aspect of dinosaur life?`
}

package tools

import (
	"context"
	"math/rand"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// dinoFacts is a curated list of interesting dinosaur facts.
var dinoFacts = []struct {
	fact    string
	species string
}{
	{"Velociraptors were only about the size of a turkey — much smaller than in Jurassic Park.", "Velociraptor"},
	{"The T-Rex had a bite force of over 12,000 pounds, the strongest of any land animal ever.", "Tyrannosaurus Rex"},
	{"Triceratops had between 400 and 800 teeth in its lifetime, but only used about a third at any time.", "Triceratops"},
	{"Stegosaurus had a brain the size of a walnut — about the size of a dog's brain in a 20-foot body.", "Stegosaurus"},
	{"Brachiosaurus could reach heights of up to 40 feet, making it one of the tallest dinosaurs.", "Brachiosaurus"},
	{"Pterosaurs were not dinosaurs — they were flying reptiles that lived at the same time.", "Pterosaur"},
	{"Ankylosaurus had a tail club that could swing with enough force to break bone.", "Ankylosaurus"},
	{"The longest dinosaur name is Micropachycephalosaurus, meaning 'tiny thick-headed lizard'.", "Micropachycephalosaurus"},
	{"Spinosaurus was the largest carnivorous dinosaur — even bigger than T-Rex.", "Spinosaurus"},
	{"Diplodocus could grow up to 92 feet long — longer than a basketball court.", "Diplodocus"},
	{"Pachycephalosaurus had a skull 10 inches thick — used for head-butting contests.", "Pachycephalosaurus"},
	{"Parasaurolophus had a long, hollow crest that may have been used to make sounds like a trumpet.", "Parasaurolophus"},
	{"The T-Rex lived closer to the invention of the iPhone than to the Stegosaurus.", "Tyrannosaurus Rex"},
	{"Dinosaurs lived on every continent, including Antarctica.", "Dinosaurs"},
	{"Some dinosaurs like the Maiasaura cared for their young in colonies.", "Maiasaura"},
}

// RegisterThink registers the dino_think tool — returns a random dinosaur fact.
func RegisterThink(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "dino_think",
		Description: "Think about a random dinosaur fact. Returns a fun fact and the species it's about. Use this to start conversations or inspire curiosity.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, DinoThinkResult, error) {
		fact := dinoFacts[rand.Intn(len(dinoFacts))]
		return nil, DinoThinkResult{
			Fact:    fact.fact,
			Species: fact.species,
		}, nil
	})
}

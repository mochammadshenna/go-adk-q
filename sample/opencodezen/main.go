// Command opencodezen demonstrates the Genkit+ADK bridge pattern.
//
// It initialises Genkit, registers the AnalyzeFlow (which runs an ADK
// LlmAgent with a real tool), and runs the flow with a prompt supplied on
// the command line.
//
// Usage:
//
//	OPENCODE_API_KEY=sk-... go run ./sample/opencodezen/ "Summarise: The quick brown fox"
//	OPENCODE_API_KEY=sk-... go run ./sample/opencodezen/ "What is 2+2?"
//
// The summarise_text tool is invoked automatically when the agent decides
// to summarise content — watch for the "[tool call]" log lines.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/compat_oai"

	"go-adk-q/sample/opencodezen/agents"
)

func main() {
	// ── Prompt ───────────────────────────────────────────────────────────
	prompt := strings.Join(os.Args[1:], " ")
	if prompt == "" {
		prompt = "Summarise this text in at most 10 words: " +
			"The Google Agent Development Kit (ADK) is an open-source framework " +
			"for building, evaluating, and deploying AI agents in Go and Python."
	}

	// ── API key ──────────────────────────────────────────────────────────
	key := os.Getenv("OPENCODE_API_KEY")
	if key == "" {
		log.Fatal("OPENCODE_API_KEY is not set")
	}

	ctx := context.Background()

	// ── Genkit init ──────────────────────────────────────────────────────
	// The compat_oai plugin is registered so Genkit's registry is populated.
	// The bridge builds its own ADK model internally via opencode.NewModel,
	// but Genkit.Init must run first to set up the tracer.
	g := genkit.Init(ctx,
		genkit.WithPlugins(&compat_oai.OpenAICompatible{
			Provider: "opencode",
			APIKey:   key,
			BaseURL:  "https://opencode.ai/zen/v1",
		}),
	)

	// ── Register flow ────────────────────────────────────────────────────
	// RegisterAnalysisFlow wires:
	//   genkit.DefineFlow  →  genkit.Run("adk-agent")  →  ADK LlmAgent
	//                                                       └─ summarise_text tool
	flow := bridge.RegisterAnalysisFlow(g)

	// ── Run ──────────────────────────────────────────────────────────────
	fmt.Printf("prompt : %s\n", prompt)
	fmt.Println("running AnalyzeFlow via Genkit→ADK bridge…")

	result, err := flow.Run(ctx, prompt)
	if err != nil {
		log.Fatalf("flow error: %v", err)
	}

	fmt.Printf("\nresult : %s\n", result)
}

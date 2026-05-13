// Command openrouter demonstrates a minimal Genkit flow using OpenRouter as
// the model provider.
//
// Usage:
//
//	OPENROUTER_API_KEY=sk-... go run ./sample/openrouter/
//	OPENROUTER_API_KEY=sk-... go run ./sample/openrouter/ "Explain what a closure is"
//
// To switch models, set OPENROUTER_MODEL:
//
//	OPENROUTER_MODEL=openrouter/openai/gpt-4o \
//	  OPENROUTER_API_KEY=sk-... go run ./sample/openrouter/
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/compat_oai"
)

// defaultModel is a free-tier model that supports tool/function calling.
// Override at runtime with OPENROUTER_MODEL=openrouter/<org>/<name>.
//
// Other well-tested free options (uncomment to try):
//   - openrouter/google/gemma-4-31b-it:free
//   - openrouter/qwen/qwen3-coder:free          (strong on code)
//   - openrouter/nvidia/nemotron-3-super-120b-a12b:free
//   - openrouter/minimax/minimax-m2.5:free
//   - openrouter/nousresearch/hermes-3-llama-3.1-405b:free
//
// Paid (higher quality, no rate limits):
//   - openrouter/meta-llama/llama-3.3-70b-instruct
//   - openrouter/openai/gpt-4o
//   - openrouter/anthropic/claude-3-5-sonnet
//   - openrouter/google/gemini-2.0-flash-001
const defaultModel = "openrouter/meta-llama/llama-3.3-70b-instruct:free"

func main() {
	// ── API key ──────────────────────────────────────────────────────────
	key := os.Getenv("OPENROUTER_API_KEY")
	if key == "" {
		log.Fatal("OPENROUTER_API_KEY is not set — get one at https://openrouter.ai/keys")
	}

	// ── Model selection ──────────────────────────────────────────────────
	model := os.Getenv("OPENROUTER_MODEL")
	if model == "" {
		model = defaultModel
	}

	// ── Prompt ───────────────────────────────────────────────────────────
	prompt := strings.Join(os.Args[1:], " ")
	if prompt == "" {
		prompt = `func add(a, b int) int { return a + b }`
	}

	ctx := context.Background()

	// ── Genkit init ──────────────────────────────────────────────────────
	g := genkit.Init(ctx,
		genkit.WithPlugins(&compat_oai.OpenAICompatible{
			Provider: "openrouter",
			APIKey:   key,
			BaseURL:  "https://openrouter.ai/api/v1",
		}),
		genkit.WithDefaultModel(model),
	)

	// ── Flow ─────────────────────────────────────────────────────────────
	analyzeFlow := genkit.DefineFlow(g, "analyzeCodeFlow",
		func(ctx context.Context, code string) (string, error) {
			resp, err := genkit.Generate(ctx, g,
				ai.WithModelName(model),
				ai.WithPrompt("Analyze the following code and explain what it does:\n\n%s", code),
			)
			if err != nil {
				return "", err
			}
			return resp.Text(), nil
		},
	)

	// ── Run ──────────────────────────────────────────────────────────────
	fmt.Printf("model  : %s\n", model)
	fmt.Printf("prompt : %s\n", prompt)
	fmt.Println("running analyzeCodeFlow…")

	result, err := analyzeFlow.Run(ctx, prompt)
	if err != nil {
		log.Fatalf("flow error: %v", err)
	}
	fmt.Printf("\nresult : %s\n", result)
}

package main

import (
	"context"
	"log"
	"os"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/compat_oai"
	"github.com/openai/openai-go"
)

func main() {
	ctx := context.Background()

	// 1. Fetch the GitHub Personal Access Token (PAT).
	// Never hardcode this — inject at runtime via environment variable.
	githubPat := os.Getenv("GITHUB_PAT")
	if githubPat == "" {
		log.Fatal("GITHUB_PAT is missing. Please set your GitHub Token.")
	}

	// 2. Initialize Genkit with the GitHub Models API endpoint.
	g := genkit.Init(ctx,
		genkit.WithPlugins(&compat_oai.OpenAICompatible{
			Provider: "github-models",
			APIKey:   githubPat,
			BaseURL:  "https://models.inference.ai.azure.com",
		}),
		genkit.WithDefaultModel("github-models/gpt-4o"),
	)

	config := &openai.ChatCompletionNewParams{
		MaxCompletionTokens: openai.Int(1000),
		Temperature:         openai.Float(0.7),
		TopP:                openai.Float(0.9),
		ReasoningEffort:     openai.ReasoningEffortLow,
	}

	// Examples: "gpt-4o", "gpt-4o-mini", "o3-mini", "Meta-Llama-3.1-405B-Instruct", "Meta-Llama-3.1-8B-Instruct", "Cohere-command-r-plus-08-2024", "DeepSeek-V3-0324", "gemini-2.0-flash", "Llama-4-Scout-17B-16E-Instruct"
	// 3. Define a flow that sends a prompt to the model and returns the text.
	analyzeFlow := genkit.DefineFlow(g, "analyzeCodeFlow",
		func(ctx context.Context, code string) (string, error) {
			resp, err := genkit.Generate(ctx, g,
				ai.WithModelName("github-models/Llama-4-Scout-17B-16E-Instruct"),
				ai.WithPrompt("Analyze the following code and explain what it does:\n\n%s", code),
				ai.WithConfig(config),
			)
			if err != nil {
				return "", err
			}

			return resp.Text(), nil
		},
	)

	// 4. Run the flow with a simple test input.
	log.Println("Running analyzeCodeFlow...")
	result, err := analyzeFlow.Run(ctx, `func add(a, b int) int { return a + b }`)
	if err != nil {
		log.Fatalf("flow error: %v", err)
	}
	log.Println("Result:", result)
}

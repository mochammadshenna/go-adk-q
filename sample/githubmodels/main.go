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
		Temperature:         openai.Float(1.0),
		TopP:                openai.Float(1.0),
		ReasoningEffort:     openai.ReasoningEffortLow,
	}

	// TESTED MODELS:
	// TODO : DO NOT NEED config completion tokens 1000, reasoning effort low, temperature 0.7, top_p 0.9 for some models, need to verify with GitHub Models team which models need those configs and which do not
	// githubModel := "github-models/Codestral-2501"
	// githubModel := "github-models/Ministral-3B"
	// githubModel := "github-models/mistral-small-2503"
	// githubModel := "github-models/mistral-medium-2505"
	// githubModel := "github-models/gpt-4o"
	// githubModel := "github-models/gpt-4o-mini"
	// githubModel := "github-models/gpt-4.1"
	// githubModel := "github-models/gpt-4.1-mini"
	// githubModel := "github-models/gpt-4.1-nano"
	// githubModel := "github-models/gpt-5"
	// githubModel := "github-models/gpt-5-mini"
	// githubModel := "github-models/gpt-5-nano"
	// githubModel := "github-models/gpt-5-chat"

	// TODO : can with and without config completion tokens
	// githubModel := "github-models/o3-mini"
	// githubModel := "github-models/Meta-Llama-3.1-405B-Instruct"
	// githubModel := "github-models/Meta-Llama-3.1-8B-Instruct"
	// githubModel := "github-models/Llama-3.2-11B-Vision-Instruct"
	// githubModel := "github-models/Llama-3.2-90B-Vision-Instruct"
	// githubModel := "github-models/Llama-3.3-70B-Instruct"
	// githubModel := "github-models/Llama-4-Maverick-17B-128E-Instruct-FP8"
	// githubModel := "github-models/Llama-4-Scout-17B-16E-Instruct"
	// githubModel := "github-models/Cohere-command-r-08-2024"
	// githubModel := "github-models/Cohere-command-r-plus-08-2024"
	// githubModel := "github-models/Cohere-command-a"
	// githubModel := "github-models/DeepSeek-R1"
	// githubModel := "github-models/DeepSeek-R1-0528"
	// githubModel := "github-models/DeepSeek-V3-0324"
	// githubModel := "github-models/Phi-4"
	// githubModel := "github-models/Phi-4-mini-instruct"
	// githubModel := "github-models/Phi-4-multimodal-instruct"
	// githubModel := "github-models/Phi-4-mini-reasoning"
	githubModel := "github-models/Phi-4-reasoning"
	// githubModel := "github-models/AI21-Jamba-1.5-Large"
	// githubModel := "github-models/AI21-Jamba-Instruct"
	// TODO: still not available on GitHub Models, need to verify with GitHub Models team when it will be available
	// githubModel := "github-models/claude-3-5-sonnet@20240620"
	// githubModel := "github-models/xai/grok-3-mini"
	// githubModel := "github-models/microsoft/MAI-DS-R1"

	// 3. Define a flow that sends a prompt to the model and returns the text.
	analyzeFlow := genkit.DefineFlow(g, "analyzeCodeFlow",
		func(ctx context.Context, code string) (string, error) {
			resp, err := genkit.Generate(ctx, g,
				ai.WithModelName(githubModel),
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

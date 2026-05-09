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

	// 1. Fetch the OPENROUTER API Key.
	// Never hardcode this — inject at runtime via environment variable.
	openRouterApiKey := os.Getenv("OPENROUTER_API_KEY")
	if openRouterApiKey == "" {
		log.Fatal("OPENROUTER_API_KEY is missing. Please set your OpenRouter API Key.")
	}

	// 2. Initialize Genkit with the OpenRouter API endpoint.
	g := genkit.Init(ctx,
		genkit.WithPlugins(&compat_oai.OpenAICompatible{
			Provider: "openrouter",
			APIKey:   openRouterApiKey,
			BaseURL:  "https://openrouter.ai/api/v1",
		}),
		genkit.WithDefaultModel("openrouter/meta-llama/llama-3.3-70b-instruct"),
	)

	config := &openai.ChatCompletionNewParams{
		MaxCompletionTokens: openai.Int(1000),
		Temperature:         openai.Float(0.7),
		TopP:                openai.Float(0.9),
	}

	///	DefaultModel
	// See https://openrouter.ai/models for the full list and pricing.
	// Note: model names must be prefixed with "openrouter/" — the provider name
	// openrouterModel := "openrouter/meta-llama/llama-3.3-70b-instruct" // best open-weight option
	// openrouterModel := "openrouter/openai/gpt-4o" // GPT-4o
	//	openrouterModel := "openrouter/openai/gpt-4o-mini"                                 // GPT-4o mini, low cost
	// openrouterModel := "openrouter/anthropic/claude-3-5-sonnet" // Claude 3.5 Sonnet
	//	openrouterModel := "openrouter/google/gemini-2.0-flash-001"                        // Gemini 2.0 Flash
	//	openrouterModel := "openrouter/mistralai/mistral-large-2411"                       // Mistral Large
	// openrouterModel := "openrouter/deepseek/deepseek-r1" // DeepSeek R1 (reasoning)
	// registered by compat_oai (e.g. "openrouter/meta-llama/llama-3.3-70b-instruct").
	// openrouterModel := "openrouter/tencent/hy3-preview:free"
	// openrouterModel := "openrouter/nvidia/nemotron-3-super-120b-a12b:free"
	// openrouterModel := "openrouter/poolside/laguna-m.1:free"
	// openrouterModel := "openrouter/openai/gpt-oss-120b:free"
	// openrouterModel := "openrouter/z-ai/glm-4.5-air:free"
	// openrouterModel := "openrouter/minimax/minimax-m2.5:free"
	// openrouterModel := "openrouter/openai/gpt-oss-20b:free"
	// openrouterModel := "openrouter/poolside/laguna-xs.2:free"
	// openrouterModel := "openrouter/google/gemma-4-31b-it:free"
	// openrouterModel := "openrouter/google/gemma-4-26b-a4b-it:free"
	// openrouterModel := "openrouter/baidu/cobuddy:free"
	// openrouterModel := "openrouter/qwen/qwen3-coder:free"
	// openrouterModel := "openrouter/liquid/lfm-2.5-1.2b-thinking:free"
	// openrouterModel := "openrouter/qwen/qwen3-next-80b-a3b-instruct:free"
	// openrouterModel := "openrouter/liquid/lfm-2.5-1.2b-instruct:free"
	// openrouterModel := "openrouter/baidu/qianfan-ocr-fast:free"
	// openrouterModel := "openrouter/meta-llama/llama-3.3-70b-instruct:free"
	// openrouterModel := "openrouter/cognitivecomputations/dolphin-mistral-24b-venice-edition:free"
	// openrouterModel := "openrouter/nousresearch/hermes-3-llama-3.1-405b:free"
	openrouterModel := "openrouter/meta-llama/llama-3.2-3b-instruct:free"
	// openrouterModel := "openrouter/"
	// openrouterModel := "openrouter/"

	// 3. Define a flow that sends a prompt to the model and returns the text.
	analyzeFlow := genkit.DefineFlow(g, "analyzeCodeFlow",
		func(ctx context.Context, code string) (string, error) {
			resp, err := genkit.Generate(ctx, g,
				ai.WithModelName(openrouterModel),
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

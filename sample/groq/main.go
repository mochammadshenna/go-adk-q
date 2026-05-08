package main

import (
	"context"
	"log"
	"os"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/compat_oai"
)

func main() {
	ctx := context.Background()

	// 1. Fetch the GROQ API Key.
	// Never hardcode this — inject at runtime via environment variable.
	groqApiKey := os.Getenv("GROQ_API_KEY")
	if groqApiKey == "" {
		log.Fatal("GROQ_API_KEY is missing. Please set your GROQ API Key.")
	}

	// 2. Initialize Genkit with the GitHub Models API endpoint.
	g := genkit.Init(ctx,
		genkit.WithPlugins(&compat_oai.OpenAICompatible{
			Provider: "groq",
			APIKey:   groqApiKey,
			BaseURL:  "https://api.groq.com/openai/v1",
		}),
		genkit.WithDefaultModel("groq/grok-3-mini"),
	)

	// config := &openai.ChatCompletionNewParams{
	// 	MaxCompletionTokens: openai.Int(1000),
	// 	Temperature:         openai.Float(0.7),
	// 	TopP:                openai.Float(0.9),
	// 	ReasoningEffort:     openai.ReasoningEffortLow,
	// }

	// groqModel := "groq/llama-3.1-8b-instant"
	// groqModel := "groq/llama-3.3-70b-versatile"
	// groqModel := "groq/meta-llama/llama-4-scout-17b-16e-instruct"
	// groqModel := "groq/meta-llama/llama-prompt-guard-2-22m"
	// groqModel := "groq/meta-llama/llama-prompt-guard-2-86m"
	// groqModel := "groq/qwen/qwen3-32b"
	// groqModel := "groq/openai/gpt-oss-120b"
	// groqModel := "groq/openai/gpt-oss-20b"
	// groqModel := "groq/openai/gpt-oss-safeguard-20b"
	// groqModel := "groq/groq/compound"
	groqModel := "groq/groq/compound-mini"

	// TODO : this model for TTS
	// groqModel := "groq/canopylabs/orpheus-v1-english"
	// TODO : this for audio transcription
	// groqModel := "groq/whisper-large-v3"
	// groqModel := "groq/whisper-large-v3-turbo"

	// 3. Define a flow that sends a prompt to the model and returns the text.
	analyzeFlow := genkit.DefineFlow(g, "analyzeCodeFlow",
		func(ctx context.Context, code string) (string, error) {
			resp, err := genkit.Generate(ctx, g,
				ai.WithModelName(groqModel),
				ai.WithPrompt("Analyze the following code and explain what it does:\n\n%s", code),
				// ai.WithConfig(config),
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

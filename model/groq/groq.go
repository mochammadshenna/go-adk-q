// Package groq provides an ADK model.LLM backed by Groq's OpenAI-compatible
// inference API.
//
// Groq runs open-weight models (LLaMA 3, Mistral, Gemma, etc.) on its custom
// LPU (Language Processing Unit) hardware, delivering significantly lower
// latency than GPU-based providers for the same models.
//
// # Configuration
//
// The idiomatic way to configure this package is via environment variables
// and [ConfigFromEnv]:
//
//	export GROQ_API_KEY=gsk_...          # required
//	export GROQ_MODEL=llama-3.1-8b-instant  # optional; overrides DefaultModel
//
// For programmatic configuration, construct a [Config] directly:
//
//	cfg := groq.Config{
//	    APIKey:    "gsk_...",
//	    ModelName: "mixtral-8x7b-32768",
//	}
//	m, err := groq.NewModel(ctx, cfg)
//
// # Supported models (as of mid-2025)
//
//	DefaultModel              = "llama-3.3-70b-versatile"   // best balance
//	"llama-3.1-8b-instant"                                   // fastest
//	"llama-3.1-70b-versatile"                                // alternative 70B
//	"mixtral-8x7b-32768"                                     // long context MoE
//	"gemma2-9b-it"                                           // Google Gemma 2
//
// All listed models support OpenAI-style function / tool calling.
package groq

import (
	"context"
	"os"

	"go-adk-q/model/oaibridge"

	"google.golang.org/adk/model"
)

const (
	// DefaultModel is the recommended starting point: Meta's LLaMA 3.3 70B
	// offers strong instruction following and tool use at very low latency
	// on Groq's LPU hardware.
	DefaultModel = "llama-3.3-70b-versatile"

	baseURL  = "https://api.groq.com/openai/v1"
	provider = "groq"

	// EnvAPIKey and EnvModel are the environment variable names read by
	// [ConfigFromEnv]. Exported so callers can reference them in docs/tests.
	EnvAPIKey = "GROQ_API_KEY"
	EnvModel  = "GROQ_MODEL"
)

// Config holds all configuration for a Groq-backed model.LLM.
// All fields are exported so the struct can be constructed, cloned, and
// overridden freely without helper functions.
type Config struct {
	// APIKey is the Groq API key. Required.
	// Obtain one at https://console.groq.com/keys.
	APIKey string

	// ModelName is the Groq model identifier to use.
	// Defaults to [DefaultModel] when empty.
	ModelName string
}

// ConfigFromEnv returns a Config populated from environment variables.
// GROQ_API_KEY is required; GROQ_MODEL is optional and falls back to
// [DefaultModel] when unset.
//
// Use the APIKey field to check whether the config is usable:
//
//	if cfg := groq.ConfigFromEnv(); cfg.APIKey != "" {
//	    m, err := groq.NewModel(ctx, cfg)
//	    ...
//	}
func ConfigFromEnv() Config {
	modelName := os.Getenv(EnvModel)
	if modelName == "" {
		modelName = DefaultModel
	}
	return Config{
		APIKey:    os.Getenv(EnvAPIKey),
		ModelName: modelName,
	}
}

// NewModel returns a model.LLM that delegates inference to Groq.
//
// The returned model is safe for concurrent use and may be shared across
// multiple [llmagent.LlmAgent] instances.
func NewModel(ctx context.Context, cfg Config) (model.LLM, error) {
	modelName := cfg.ModelName
	if modelName == "" {
		modelName = DefaultModel
	}
	return oaibridge.NewModel(ctx, oaibridge.Config{
		Provider:  provider,
		BaseURL:   baseURL,
		APIKey:    cfg.APIKey,
		ModelName: modelName,
	})
}

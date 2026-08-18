// Package nvidia provides an ADK model.LLM backed by NVIDIA NIM (NVIDIA
// Inference Microservices) via its OpenAI-compatible API.
//
// NVIDIA NIM hosts optimised, curated variants of open-weight models on
// NVIDIA-accelerated infrastructure, including models from NVIDIA's own
// research teams (Nemotron), Meta (LLaMA), Mistral, MiniMax, and others.
// NIM is available as a cloud service or for on-premises deployment.
//
// # Configuration
//
// The idiomatic way to configure this package is via environment variables
// and [ConfigFromEnv]:
//
//	export NVIDIA_API_KEY=nvapi-...          # required
//	export NVIDIA_MODEL=meta/llama-3.1-70b-instruct  # optional; overrides DefaultModel
//
// For programmatic configuration, construct a [Config] directly:
//
//	cfg := nvidia.Config{
//	    APIKey:    "nvapi-...",
//	    ModelName: "mistralai/mistral-large-2-instruct",
//	}
//	m, err := nvidia.NewModel(ctx, cfg)
//
// # On-premises NIM
//
// For on-premises NIM deployments override BaseURL in the Config:
//
//	cfg := nvidia.Config{
//	    APIKey:    "...",
//	    ModelName: "...",
//	    BaseURL:   "http://nim-host:8000/v1",
//	}
//
// # Supported models (selection, as of mid-2025)
//
//	DefaultModel                               = "minimaxai/minimax-m1"
//	"nvidia/llama-3.1-nemotron-70b-instruct"  // NVIDIA reasoning flagship
//	"meta/llama-3.1-405b-instruct"            // Meta LLaMA 3.1 405B
//	"meta/llama-3.1-70b-instruct"             // Meta LLaMA 3.1 70B
//	"mistralai/mistral-large-2-instruct"      // Mistral Large 2
//	"microsoft/phi-3-medium-128k-instruct"    // Microsoft Phi-3 Medium
//	"google/gemma-2-27b-it"                   // Google Gemma 2 27B
//
// See https://build.nvidia.com for the complete model catalogue and
// access requirements. All listed models support function / tool calling.
package nvidia

import (
	"context"
	"os"

	"go-adk-q/model/catalog"
	"go-adk-q/model/oaibridge"
	"google.golang.org/adk/model"
)

// KnownModels is the curated catalog of models available on NVIDIA NIM
// (https://build.nvidia.com).
//
// Add new entries here to make them available in the /model TUI picker.
var KnownModels = catalog.ProviderCatalog{
	Provider: "nvidia",
	Label:    "NVIDIA NIM",
	EnvVar:   EnvAPIKey,
	Models: []catalog.ModelEntry{
		{ID: "minimaxai/minimax-m1", Label: "MiniMax M1", Default: true},
		{ID: "nvidia/llama-3.1-nemotron-70b-instruct", Label: "Nemotron 70B", Tags: []string{"reasoning"}},
		{ID: "meta/llama-3.1-405b-instruct", Label: "Llama 3.1 405B", Tags: []string{"large"}},
		{ID: "meta/llama-3.1-70b-instruct", Label: "Llama 3.1 70B"},
		{ID: "mistralai/mistral-large-2-instruct", Label: "Mistral Large 2"},
		{ID: "microsoft/phi-3-medium-128k-instruct", Label: "Phi-3 Medium 128K"},
		{ID: "google/gemma-2-27b-it", Label: "Gemma 2 27B"},
	},
}

const (
	// DefaultModel is MiniMax's M1 model, available on NVIDIA NIM.
	// MiniMax M1 is a frontier mixture-of-experts model with strong
	// reasoning and long-context capabilities, running on NVIDIA hardware.
	DefaultModel = "minimaxai/minimax-m1"

	// cloudBaseURL is NVIDIA NIM's hosted cloud inference endpoint.
	cloudBaseURL = "https://integrate.api.nvidia.com/v1"

	provider = "nvidia"

	// EnvAPIKey, EnvModel, and EnvBaseURL are the environment variable names
	// read by [ConfigFromEnv]. Exported so callers can reference them.
	EnvAPIKey  = "NVIDIA_API_KEY"
	EnvModel   = "NVIDIA_MODEL"
	EnvBaseURL = "NVIDIA_BASE_URL" // override for on-premises NIM
)

// Config holds all configuration for a NVIDIA NIM-backed model.LLM.
// All fields are exported so the struct can be constructed, cloned, and
// overridden freely without helper functions.
type Config struct {
	// APIKey is the NVIDIA API key. Required for the cloud endpoint.
	// Obtain one at https://build.nvidia.com.
	APIKey string

	// ModelName is the NIM model identifier (e.g. "minimaxai/minimax-m1").
	// Defaults to [DefaultModel] when empty.
	ModelName string

	// BaseURL overrides the inference endpoint.
	// Leave empty to use the NVIDIA NIM cloud (cloudBaseURL).
	// Set to your local endpoint for on-premises NIM, e.g. "http://nim:8000/v1".
	BaseURL string
}

// ConfigFromEnv returns a Config populated from environment variables:
//   - NVIDIA_API_KEY  — required
//   - NVIDIA_MODEL    — optional; falls back to [DefaultModel]
//   - NVIDIA_BASE_URL — optional; falls back to the NVIDIA NIM cloud endpoint
//
// Use the APIKey field to check whether the config is usable:
//
//	if cfg := nvidia.ConfigFromEnv(); cfg.APIKey != "" {
//	    m, err := nvidia.NewModel(ctx, cfg)
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
		BaseURL:   os.Getenv(EnvBaseURL),
	}
}

// NewModel returns a model.LLM that delegates inference to NVIDIA NIM.
//
// The returned model is safe for concurrent use and may be shared across
// multiple agent instances.
func NewModel(ctx context.Context, cfg Config) (model.LLM, error) {
	modelName := cfg.ModelName
	if modelName == "" {
		modelName = DefaultModel
	}
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = cloudBaseURL
	}
	return oaibridge.NewModel(ctx, oaibridge.Config{
		Provider:  provider,
		BaseURL:   baseURL,
		APIKey:    cfg.APIKey,
		ModelName: modelName,
	})
}

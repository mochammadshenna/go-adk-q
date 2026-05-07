// Package githubmodels provides an ADK model.LLM backed by the GitHub Models
// inference API (https://models.inference.ai.azure.com).
//
// GitHub Models exposes an OpenAI-compatible chat completions endpoint and
// supports a wide catalogue of models including OpenAI GPT series, Anthropic
// Claude, Meta LLaMA, Google Gemini, Mistral, Cohere, and DeepSeek — all
// accessible with a single GitHub Personal Access Token (PAT).
//
// # Authentication
//
// A GitHub PAT with the "models" permission is required.
// Create one at https://github.com/settings/personal-access-tokens/new and
// select Permissions → Models → Read.
//
// # Configuration
//
// The idiomatic way to configure this package is via environment variables
// and [ConfigFromEnv]:
//
//	export GITHUB_PAT=github_pat_...          # required
//	export GITHUB_MODEL=gpt-4o                # optional; overrides DefaultModel
//
// For programmatic configuration, construct a [Config] directly:
//
//	cfg := githubmodels.Config{
//	    PAT:       "github_pat_...",
//	    ModelName: "Llama-4-Scout-17B-16E-Instruct",
//	}
//	m, err := githubmodels.NewModel(ctx, cfg)
//
// # Supported models (as of mid-2025)
//
// OpenAI:
//
//	DefaultModel = "gpt-4o"                        // best balance, widely supported
//	"gpt-4o-mini"                                  // faster, cheaper
//	"gpt-4.1"                                      // latest GPT-4 series
//	"o3-mini"                                      // reasoning model
//
// Meta LLaMA:
//
//	"Llama-4-Scout-17B-16E-Instruct"               // fast MoE, tool-capable
//	"Meta-Llama-3.1-405B-Instruct"                 // largest open-weight
//	"Meta-Llama-3.1-8B-Instruct"                   // fastest open-weight
//
// Anthropic Claude:
//
//	"claude-sonnet-4-5"                            // Claude Sonnet (check exact ID on marketplace)
//
// Google:
//
//	"gemini-2.0-flash"                             // Gemini Flash
//
// Others:
//
//	"DeepSeek-V3-0324"                             // DeepSeek V3
//	"Cohere-command-r-plus-08-2024"                // Cohere Command R+
//	"Mistral-Large-2411"                           // Mistral Large
//
// Verify exact IDs at https://github.com/marketplace/models.
//
// # Tool calling
//
// All models listed above support OpenAI-style function/tool calling via the
// compat_oai bridge. Note: skills toolset (skilltoolset) is not enabled for
// this provider because non-Gemini models cannot reliably call load_skill.
package githubmodels

import (
	"context"
	"os"

	"go-adk-q/model/oaibridge"

	"google.golang.org/adk/model"
)

const (
	// DefaultModel is the recommended starting point: GPT-4o offers the best
	// balance of speed, capability, and tool-calling reliability on the
	// GitHub Models endpoint.
	// TESTED MODELS: "gpt-4o",
	// "gpt-4o-mini", "o3-mini",
	// "Meta-Llama-3.1-405B-Instruct",
	// "Meta-Llama-3.1-8B-Instruct",
	// "Cohere-command-r-plus-08-2024",
	// "DeepSeek-V3-0324",
	// "Llama-4-Scout-17B-16E-Instruct"
	DefaultModel = "gpt-4o"

	baseURL  = "https://models.inference.ai.azure.com"
	provider = "github-models"

	// EnvPAT and EnvModel are the environment variable names read by
	// [ConfigFromEnv]. Exported so callers can reference them in docs/tests.
	EnvPAT   = "GITHUB_PAT"
	EnvModel = "GITHUB_MODEL"
)

// Config holds all configuration for a GitHub Models-backed model.LLM.
// All fields are exported so the struct can be constructed, cloned, and
// overridden freely without helper functions.
type Config struct {
	// PAT is the GitHub Personal Access Token with the "models" permission.
	// Required. Obtain one at https://github.com/settings/personal-access-tokens/new.
	PAT string

	// ModelName is the GitHub Models model identifier to use.
	// Defaults to [DefaultModel] when empty.
	// Verify exact IDs at https://github.com/marketplace/models.
	ModelName string
}

// ConfigFromEnv returns a Config populated from environment variables.
// GITHUB_PAT is required; GITHUB_MODEL is optional and falls back to
// [DefaultModel] when unset.
//
// Use the PAT field to check whether the config is usable:
//
//	if cfg := githubmodels.ConfigFromEnv(); cfg.PAT != "" {
//	    m, err := githubmodels.NewModel(ctx, cfg)
//	    ...
//	}
func ConfigFromEnv() Config {
	modelName := os.Getenv(EnvModel)
	if modelName == "" {
		modelName = DefaultModel
	}
	return Config{
		PAT:       os.Getenv(EnvPAT),
		ModelName: modelName,
	}
}

// NewModel returns a model.LLM that delegates inference to the GitHub Models
// inference API.
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
		APIKey:    cfg.PAT,
		ModelName: modelName,
	})
}

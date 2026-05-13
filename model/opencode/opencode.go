// Package opencode provides an ADK model.LLM backed by OpenCode's AI gateway.
//
// OpenCode (https://opencode.ai) exposes an OpenAI-compatible endpoint at
// https://opencode.ai/zen/v1 that routes to a curated set of models, many of
// which are available on a free tier. This makes it a convenient zero-cost
// fallback in a multi-provider failover chain.
//
// # Authentication
//
// Set the OPENCODE_API_KEY environment variable or pass the key directly via
// [Config.APIKey].  Keys are obtained from https://opencode.ai.
//
// # Supported models (selection)
//
//	DefaultModel = "minimax-m2.5-free"  // MiniMax M2.5, free tier
//	"big-pickle"                        // big-pickle (free)
//	"hy3-preview-free"                  // HunYuan 3 preview (free)
//	"nemotron-3-super-free"             // NVIDIA Nemotron 3 (free)
//
// See https://opencode.ai/models for the current model list.
//
// # Usage
//
//	m, err := opencode.NewModel(ctx, opencode.ConfigFromEnv())
//	agent, err := llmagent.New(llmagent.Config{Model: m, ...})
//
// Or with explicit config:
//
//	m, err := opencode.NewModel(ctx, opencode.Config{
//	    APIKey:    os.Getenv("OPENCODE_API_KEY"),
//	    ModelName: "minimax-m2.5-free",
//	})
package opencode

import (
	"context"
	"os"

	"go-adk-q/model/catalog"
	"go-adk-q/model/oaibridge"

	"google.golang.org/adk/model"
)

// KnownModels is the curated catalog of models available via OpenCode's
// AI gateway (https://opencode.ai/zen/v1).  Many are on a free tier;
// capacity may be limited.
//
// Model IDs are sent as-is to the API — do NOT include the "opencode/"
// provider prefix here; the API expects bare model names (e.g. "minimax-m2.5-free",
// not "opencode/minimax-m2.5-free").
//
// Add new entries here to make them available in the /model TUI picker.
var KnownModels = catalog.ProviderCatalog{
	Provider: "opencode",
	Label:    "OpenCode",
	Models: []catalog.ModelEntry{
		// ── Default (free) ────────────────────────────────────────────────
		{ID: "minimax-m2.5-free", Label: "MiniMax M2.5 (free)", Tags: []string{"free"}, Default: true},
		// ── HunYuan ───────────────────────────────────────────────────────
		{ID: "hy3-preview-free", Label: "HunYuan 3 Preview (free)", Tags: []string{"free"}},
		// ── NVIDIA ────────────────────────────────────────────────────────
		{ID: "nemotron-3-super-free", Label: "Nemotron 3 Super (free)", Tags: []string{"free"}},
		// ── Big Pickle ────────────────────────────────────────────────────
		{ID: "big-pickle", Label: "Big Pickle (free)", Tags: []string{"free"}},
	},
}

const (
	// DefaultModel is MiniMax M2.5 (free tier) — a capable free-tier model
	// and a good zero-cost fallback option.
	DefaultModel = "minimax-m2.5-free"

	baseURL  = "https://opencode.ai/zen/v1"
	provider = "opencode"

	// Env var names read by [ConfigFromEnv]. Exported so callers can reference
	// them in tests and documentation.
	EnvAPIKey = "OPENCODE_API_KEY"
	EnvModel  = "OPENCODE_MODEL"
)

// Config holds OpenCode-specific configuration.
// All fields are exported so the struct can be constructed, cloned, and
// overridden freely without helper functions.
type Config struct {
	// APIKey is the OpenCode API key. Required.
	// Obtain one at https://opencode.ai.
	APIKey string

	// ModelName is the OpenCode model identifier, e.g. "minimax-m2.5-free".
	// Do NOT include the "opencode/" prefix — model IDs are bare names.
	// Defaults to [DefaultModel] when empty.
	// See https://opencode.ai/models for the full list.
	ModelName string
}

// ConfigFromEnv returns a Config populated from environment variables:
//   - OPENCODE_API_KEY — required
//   - OPENCODE_MODEL   — optional; falls back to [DefaultModel]
//
// Use the APIKey field to check whether the config is usable before calling
// [NewModel]:
//
//	if cfg := opencode.ConfigFromEnv(); cfg.APIKey != "" {
//	    m, err := opencode.NewModel(ctx, cfg)
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

// NewModel returns a model.LLM that delegates inference to OpenCode's AI
// gateway at https://opencode.ai/zen/v1.
//
// Returns an error if APIKey is empty — check [ConfigFromEnv] or set
// OPENCODE_API_KEY before calling.
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

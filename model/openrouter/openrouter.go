// Package openrouter provides an ADK model.LLM backed by OpenRouter's unified
// model routing API.
//
// OpenRouter is a meta-provider: it exposes a single OpenAI-compatible endpoint
// that routes requests to the best available provider for any given model.
// This lets you access models from OpenAI, Anthropic, Google, Meta, Mistral,
// and dozens of other providers through one API key and one billing account.
//
// # Authentication
//
// Set the OPENROUTER_API_KEY environment variable or pass the key directly.
// Keys are obtained from https://openrouter.ai/keys.
//
// # Attribution headers
//
// OpenRouter asks that client applications identify themselves via two
// optional HTTP headers:
//   - HTTP-Referer: the URL of your site or project
//   - X-Title:      a human-readable name for your app
//
// These headers are used for request attribution in OpenRouter's dashboard
// and are required for some rate-limit tiers. Pass them via Config.SiteURL
// and Config.AppName; they are omitted when empty.
//
// # Supported models (selection)
//
//	DefaultModel = "meta-llama/llama-3.3-70b-instruct"  // best open-weight option
//	"openai/gpt-4o"                                      // GPT-4o
//	"openai/gpt-4o-mini"                                 // GPT-4o mini, low cost
//	"anthropic/claude-3-5-sonnet"                        // Claude 3.5 Sonnet
//	"google/gemini-2.0-flash-001"                        // Gemini 2.0 Flash
//	"mistralai/mistral-large-2411"                       // Mistral Large
//	"deepseek/deepseek-r1"                               // DeepSeek R1 (reasoning)
//
// See https://openrouter.ai/models for the full list and pricing.
//
// # Usage
//
//	m, err := openrouter.NewModel(ctx, openrouter.Config{
//	    APIKey:    os.Getenv("OPENROUTER_API_KEY"),
//	    ModelName: openrouter.DefaultModel,
//	    SiteURL:   "https://myapp.example.com",
//	    AppName:   "My ADK App",
//	})
//	agent, err := llmagent.New(llmagent.Config{Model: m, ...})
package openrouter

import (
	"context"
	"os"

	"go-adk-q/model/catalog"
	"go-adk-q/model/oaibridge"

	"google.golang.org/adk/model"
)

// KnownModels is the curated catalog of models available via OpenRouter
// (https://openrouter.ai/models).  The :free suffix indicates zero-cost
// tier models; capacity may be limited.
//
// Add new entries here to make them available in the /model TUI picker.
var KnownModels = catalog.ProviderCatalog{
	Provider: "openrouter",
	Label:    "OpenRouter",
	Models: []catalog.ModelEntry{
		// ── Default (free tier) ───────────────────────────────────────────
		{ID: "meta-llama/llama-3.2-3b-instruct:free", Label: "Llama 3.2 3B (free)", Tags: []string{"free"}, Default: true},
		// ── Meta LLaMA ────────────────────────────────────────────────────
		{ID: "meta-llama/llama-3.3-70b-instruct", Label: "Llama 3.3 70B"},
		{ID: "meta-llama/llama-3.3-70b-instruct:free", Label: "Llama 3.3 70B (free)", Tags: []string{"free"}},
		// ── OpenAI ────────────────────────────────────────────────────────
		{ID: "openai/gpt-4o", Label: "GPT-4o"},
		{ID: "openai/gpt-4o-mini", Label: "GPT-4o mini", Tags: []string{"fast"}},
		{ID: "openai/gpt-oss-120b:free", Label: "GPT-OSS 120B (free)", Tags: []string{"free"}},
		{ID: "openai/gpt-oss-20b:free", Label: "GPT-OSS 20B (free)", Tags: []string{"free"}},
		// ── Anthropic ─────────────────────────────────────────────────────
		{ID: "anthropic/claude-3-5-sonnet", Label: "Claude 3.5 Sonnet"},
		// ── Google ────────────────────────────────────────────────────────
		{ID: "google/gemini-2.0-flash-001", Label: "Gemini 2.0 Flash", Tags: []string{"fast"}},
		{ID: "google/gemma-4-31b-it:free", Label: "Gemma 4 31B (free)", Tags: []string{"free"}},
		{ID: "google/gemma-4-26b-a4b-it:free", Label: "Gemma 4 26B MoE (free)", Tags: []string{"free"}},
		// ── Mistral ───────────────────────────────────────────────────────
		{ID: "mistralai/mistral-large-2411", Label: "Mistral Large 2411"},
		// ── DeepSeek ──────────────────────────────────────────────────────
		{ID: "deepseek/deepseek-r1", Label: "DeepSeek R1", Tags: []string{"reasoning"}},
		// ── Qwen ──────────────────────────────────────────────────────────
		{ID: "qwen/qwen3-coder:free", Label: "Qwen3 Coder (free)", Tags: []string{"free", "code"}},
		{ID: "qwen/qwen3-next-80b-a3b-instruct:free", Label: "Qwen3 80B (free)", Tags: []string{"free"}},
		// ── Liquid ────────────────────────────────────────────────────────
		{ID: "liquid/lfm-2.5-1.2b-thinking:free", Label: "LFM 2.5 1.2B Thinking (free)", Tags: []string{"free"}},
		{ID: "liquid/lfm-2.5-1.2b-instruct:free", Label: "LFM 2.5 1.2B (free)", Tags: []string{"free"}},
		// ── NVIDIA ────────────────────────────────────────────────────────
		{ID: "nvidia/nemotron-3-super-120b-a12b:free", Label: "Nemotron 120B (free)", Tags: []string{"free"}},
		// ── Minimax ───────────────────────────────────────────────────────
		{ID: "minimax/minimax-m2.5:free", Label: "MiniMax M2.5 (free)", Tags: []string{"free"}},
		// ── NousResearch ──────────────────────────────────────────────────
		{ID: "nousresearch/hermes-3-llama-3.1-405b:free", Label: "Hermes 3 405B (free)", Tags: []string{"free"}},
	},
}

const (
	// DefaultModel is Meta's LLaMA 3.3 70B Instruct model, routed through
	// OpenRouter. It supports function calling and offers a good balance of
	// quality, speed, and cost across most tasks.
	// DefaultModel = "meta-llama/llama-3.3-70b-instruct"
	DefaultModel = "google/gemma-4-31b-it:free"

	baseURL  = "https://openrouter.ai/api/v1"
	provider = "openrouter"

	// Env var names read by [ConfigFromEnv]. Exported so callers can reference
	// them in tests and documentation.
	EnvAPIKey  = "OPENROUTER_API_KEY"
	EnvModel   = "OPENROUTER_MODEL"
	EnvSiteURL = "OPENROUTER_SITE_URL"
	EnvAppName = "OPENROUTER_APP_NAME"
)

// Config holds OpenRouter-specific configuration.
// All fields are exported so the struct can be constructed, cloned, and
// overridden freely without helper functions.
type Config struct {
	// APIKey is the OpenRouter API key. Required.
	// Obtain one at https://openrouter.ai/keys.
	APIKey string

	// ModelName is the OpenRouter model identifier in "org/name" format.
	// Defaults to [DefaultModel] when empty.
	// See https://openrouter.ai/models for the full list.
	ModelName string

	// SiteURL is your application's URL, sent as the HTTP-Referer header for
	// usage attribution in OpenRouter's dashboard. Optional but recommended.
	// Can also be set via OPENROUTER_SITE_URL.
	SiteURL string

	// AppName is your application's display name, sent as the X-Title header.
	// Optional but recommended.
	// Can also be set via OPENROUTER_APP_NAME.
	AppName string
}

// ConfigFromEnv returns a Config populated from environment variables:
//   - OPENROUTER_API_KEY  — required
//   - OPENROUTER_MODEL    — optional; falls back to [DefaultModel]
//   - OPENROUTER_SITE_URL — optional attribution header
//   - OPENROUTER_APP_NAME — optional attribution header
//
// Use the APIKey field to check whether the config is usable:
//
//	if cfg := openrouter.ConfigFromEnv(); cfg.APIKey != "" {
//	    m, err := openrouter.NewModel(ctx, cfg)
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
		SiteURL:   os.Getenv(EnvSiteURL),
		AppName:   os.Getenv(EnvAppName),
	}
}

// NewModel returns a model.LLM that delegates inference to OpenRouter.
//
// OpenRouter routes each request to the most cost-effective available provider
// for the requested model. If a specific model or provider is temporarily
// unavailable, OpenRouter automatically fails over to equivalent alternatives.
func NewModel(ctx context.Context, cfg Config) (model.LLM, error) {
	// Build provider-specific headers. Both are optional per OpenRouter's docs
	// but are required by some rate-limit tiers and improve request attribution.
	headers := make(map[string]string, 2)
	if cfg.SiteURL != "" {
		headers["HTTP-Referer"] = cfg.SiteURL
	}
	if cfg.AppName != "" {
		headers["X-Title"] = cfg.AppName
	}

	modelName := cfg.ModelName
	if modelName == "" {
		modelName = DefaultModel
	}

	return oaibridge.NewModel(ctx, oaibridge.Config{
		Provider:     provider,
		BaseURL:      baseURL,
		APIKey:       cfg.APIKey,
		ModelName:    modelName,
		ExtraHeaders: headers,
	})
}

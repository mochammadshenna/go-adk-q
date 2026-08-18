// Package huggingface provides an ADK model.LLM backed by the HuggingFace
// Serverless Inference API or a dedicated HuggingFace Inference Endpoint,
// both served via their OpenAI-compatible endpoint.
//
// # Serverless vs dedicated endpoints
//
// HuggingFace offers two inference modes:
//
//  1. Serverless Inference API (api-inference.huggingface.co/v1): shared,
//     rate-limited, free tier available. Good for development and testing.
//
//  2. Dedicated Inference Endpoints (custom URL per deployment): isolated,
//     auto-scaling, pay-per-compute. No rate limits. Required for production.
//
// Both modes are configured via the same [Config] struct. Set Config.EndpointURL
// to select a dedicated endpoint; leave it empty for serverless.
//
// # Authentication
//
// Set the HF_TOKEN environment variable or populate Config.Token directly.
// Tokens are obtained from https://huggingface.co/settings/tokens.
//
// # Configuration
//
// The idiomatic way to configure this package is via environment variables
// and [ConfigFromEnv]:
//
//	export HF_TOKEN=hf_...                    # required
//	export HF_MODEL=mistralai/Mistral-7B-Instruct-v0.3  # optional
//	export HF_ENDPOINT_URL=https://xyz.endpoints.huggingface.cloud  # optional; selects dedicated endpoint
//
// For programmatic configuration, construct a [Config] directly:
//
//	// Serverless:
//	cfg := huggingface.Config{
//	    Token:     "hf_...",
//	    ModelName: "NousResearch/Hermes-2-Pro-Llama-3-8B",
//	}
//
//	// Dedicated endpoint:
//	cfg := huggingface.Config{
//	    Token:       "hf_...",
//	    ModelName:   "my-model",
//	    EndpointURL: "https://xyz.endpoints.huggingface.cloud",
//	}
//
//	m, err := huggingface.NewModel(ctx, cfg)
//
// # Function calling support
//
// Not all models on the Hub support OpenAI-style function / tool calling.
// The following are confirmed to work:
//
//	DefaultModel = "mistralai/Mistral-7B-Instruct-v0.3"  // Mistral 7B, tool use enabled
//	"NousResearch/Hermes-2-Pro-Llama-3-8B"               // strong function calling
//	"NousResearch/Hermes-2-Pro-Mistral-7B"               // Mistral-based, reliable tools
//	"microsoft/Phi-3.5-mini-instruct"                    // compact, supports tools
//
// For text-only tasks (no function calling) a much wider range of models works,
// including any model in the "text-generation" category on the Hub.
package huggingface

import (
	"context"
	"fmt"
	"os"
	"strings"

	"go-adk-q/model/catalog"
	"go-adk-q/model/oaibridge"

	"google.golang.org/adk/model"
)

// KnownModels is the curated catalog of HuggingFace models that are confirmed
// to support function calling via the serverless inference API.
//
// Add new entries here to make them available in the /model TUI picker.
var KnownModels = catalog.ProviderCatalog{
	Provider: "huggingface",
	Label:    "HuggingFace",
	EnvVar:   EnvToken,
	Models: []catalog.ModelEntry{
		{ID: "thinkingmachines/Inkling:together", Label: "Inkling Together", Default: true},
		{ID: "zai-org/GLM-5.2:novita", Label: "GLM 5.2 Novita", Tags: []string{"text-generation"}},
		{ID: "prism-ml/Ternary-Bonsai-27B-gguf:together", Label: "Ternary Bonsai 27B", Tags: []string{"text-generation"}},
		// Text Generation
		{ID: "Qwen/Qwen3-14B:nscale", Label: "Qwen3 14B", Tags: []string{"text-generation"}},
		{ID: "meta-llama/Llama-3.2-1B", Label: "Llama 3.2 1B", Tags: []string{"text-generation"}},
		{ID: "zai-org/GLM-4.7-Flash:novita", Label: "GLM 4.7 Flash", Tags: []string{"text-generation", "fast"}},
		{ID: "Qwen/Qwen3-8B:nscale", Label: "Qwen3 8B", Tags: []string{"text-generation"}},
		{ID: "DeepHat/DeepHat-V1-7B:featherless-ai", Label: "DeepHat V1 7B", Tags: []string{"text-generation"}},
		{ID: "nvidia/NVIDIA-Nemotron-3-Ultra-550B-A55B-BF16:deepinfra", Label: "Nemotron 3 Ultra 550B", Tags: []string{"text-generation"}},
		{ID: "deepseek-ai/DeepSeek-R1:novita", Label: "DeepSeek R1", Tags: []string{"text-generation", "reasoning"}},
		{ID: "Qwen/Qwen3-Coder-30B-A3B-Instruct:featherless-ai", Label: "Qwen3 Coder 30B", Tags: []string{"text-generation", "coding"}},
		{ID: "meta-llama/Llama-3.3-70B-Instruct:groq", Label: "Llama 3.3 70B", Tags: []string{"text-generation"}},
		{ID: "meta-llama/Llama-3.1-8B", Label: "Llama 3.1 8B", Tags: []string{"text-generation"}},
		{ID: "Qwen/Qwen3-0.6B:featherless-ai", Label: "Qwen3 0.6B", Tags: []string{"text-generation"}},
		{ID: "meta-llama/Meta-Llama-3-8B-Instruct:featherless-ai", Label: "Meta Llama 3 8B", Tags: []string{"text-generation"}},
		{ID: "meta-llama/Llama-3.2-3B-Instruct:featherless-ai", Label: "Llama 3.2 3B", Tags: []string{"text-generation"}},
		{ID: "zai-org/GLM-5.2-FP8:zai-org", Label: "GLM 5.2 FP8", Tags: []string{"text-generation"}},
		{ID: "openai/gpt-oss-20b:groq", Label: "GPT-OSS 20B", Tags: []string{"text-generation"}},
		{ID: "openai/gpt-oss-120b:groq", Label: "GPT-OSS 120B", Tags: []string{"text-generation"}},
		{ID: "meta-llama/Llama-3.1-8B-Instruct:novita", Label: "Llama 3.1 8B Novita", Tags: []string{"text-generation"}},
		{ID: "deepreinforce-ai/Ornith-1.0-35B:deepinfra", Label: "Ornith 1.0 35B", Tags: []string{"text-generation"}},
		{ID: "deepseek-ai/DeepSeek-V4-Flash:novita", Label: "DeepSeek V4 Flash", Tags: []string{"text-generation", "fast"}},
		{ID: "deepseek-ai/DeepSeek-V4-Pro:novita", Label: "DeepSeek V4 Pro", Tags: []string{"text-generation"}},
		{ID: "deepreinforce-ai/Ornith-1.0-9B:featherless-ai", Label: "Ornith 1.0 9B", Tags: []string{"text-generation"}},
		{ID: "empero-ai/Qwythos-9B-Claude-Mythos-5-1M:featherless-ai", Label: "Qwythos 9B", Tags: []string{"text-generation"}},
		// Image-Text-to-Text
		{ID: "bottlecapai/ThinkingCap-Qwen3.6-27B:featherless-ai", Label: "ThinkingCap Qwen3.6 27B", Tags: []string{"image-text-to-text"}},
		{ID: "google/gemma-4-31B-it:novita", Label: "Gemma 4 31B", Tags: []string{"image-text-to-text"}},
		{ID: "moonshotai/Kimi-K2.7-Code:novita", Label: "Kimi K2.7 Code", Tags: []string{"image-text-to-text", "coding"}},
		{ID: "Qwen/Qwen3.6-35B-A3B:featherless-ai", Label: "Qwen3.6 35B", Tags: []string{"image-text-to-text"}},
		{ID: "MiniMaxAI/MiniMax-M3:novita", Label: "MiniMax M3", Tags: []string{"image-text-to-text"}},
		// Text-to-Image
		{ID: "krea/Krea-2-Turbo", Label: "Krea 2 Turbo", Tags: []string{"text-to-image"}},
		{ID: "Tongyi-MAI/Z-Image-Turbo", Label: "Z-Image Turbo", Tags: []string{"text-to-image"}},
		{ID: "stabilityai/stable-diffusion-xl-base-1.0", Label: "SDXL 1.0", Tags: []string{"text-to-image"}},
		{ID: "black-forest-labs/FLUX.1-dev", Label: "FLUX.1 dev", Tags: []string{"text-to-image"}},
		// Existing entries
		{ID: "NousResearch/Hermes-2-Pro-Llama-3-8B", Label: "Hermes 2 Pro Llama 3 8B"},
		{ID: "NousResearch/Hermes-2-Pro-Mistral-7B", Label: "Hermes 2 Pro Mistral 7B"},
		{ID: "microsoft/Phi-3.5-mini-instruct", Label: "Phi-3.5 mini", Tags: []string{"fast"}},
	},
}

const (
	// DefaultModel is Mistral 7B Instruct v0.3, which reliably supports
	// function calling via HuggingFace's serverless inference API.
	DefaultModel = "thinkingmachines/Inkling:together"

	// serverlessBaseURL is HuggingFace's shared serverless inference endpoint.
	serverlessBaseURL = "https://router.huggingface.co/v1"

	serverlessProvider = "huggingface"
	endpointProvider   = "huggingface-endpoint"

	// EnvToken, EnvModel, EnvEndpointURL are the environment variable names
	// read by [ConfigFromEnv]. Exported so callers can reference them.
	EnvToken       = "HF_TOKEN"
	EnvModel       = "HF_MODEL"
	EnvEndpointURL = "HF_ENDPOINT_URL"
)

// Config holds all configuration for a HuggingFace-backed model.LLM.
// All fields are exported so the struct can be constructed, cloned, and
// overridden freely without helper functions.
type Config struct {
	// Token is the HuggingFace access token. Required.
	// Obtain one at https://huggingface.co/settings/tokens.
	Token string

	// ModelName is the HuggingFace model ID in "org/name" format, e.g.
	// "mistralai/Mistral-7B-Instruct-v0.3". Defaults to [DefaultModel] when empty.
	ModelName string

	// EndpointURL is the base URL of a dedicated HuggingFace Inference Endpoint,
	// e.g. "https://xyz.endpoints.huggingface.cloud".
	// When non-empty, inference is routed to this dedicated endpoint (which has
	// no rate limits) instead of the shared serverless API.
	// Leave empty to use the serverless API.
	//
	// The path "/v1" is appended automatically if not already present.
	EndpointURL string
}

// ConfigFromEnv returns a Config populated from environment variables:
//   - HF_TOKEN        — required
//   - HF_MODEL        — optional; falls back to [DefaultModel]
//   - HF_ENDPOINT_URL — optional; selects a dedicated inference endpoint
//
// Use the Token field to check whether the config is usable:
//
//	if cfg := huggingface.ConfigFromEnv(); cfg.Token != "" {
//	    m, err := huggingface.NewModel(ctx, cfg)
//	    ...
//	}
func ConfigFromEnv() Config {
	modelName := os.Getenv(EnvModel)
	if modelName == "" {
		modelName = DefaultModel
	}
	return Config{
		Token:       os.Getenv(EnvToken),
		ModelName:   modelName,
		EndpointURL: os.Getenv(EnvEndpointURL),
	}
}

// NewModel returns a model.LLM backed by HuggingFace inference.
//
// If cfg.EndpointURL is set, the dedicated Inference Endpoint at that URL is
// used (no rate limits, pay-per-compute). Otherwise the shared serverless API
// is used (rate-limited, free tier available).
//
// The returned model is safe for concurrent use.
func NewModel(ctx context.Context, cfg Config) (model.LLM, error) {
	modelName := cfg.ModelName
	if modelName == "" {
		modelName = DefaultModel
	}

	if cfg.EndpointURL != "" {
		return newEndpointModel(ctx, cfg.EndpointURL, modelName, cfg.Token)
	}
	return oaibridge.NewModel(ctx, oaibridge.Config{
		Provider:  serverlessProvider,
		BaseURL:   serverlessBaseURL,
		APIKey:    cfg.Token,
		ModelName: modelName,
	})
}

// newEndpointModel creates a model backed by a dedicated HuggingFace
// Inference Endpoint. It appends "/v1" to endpointURL if not already present.
func newEndpointModel(ctx context.Context, endpointURL, modelName, token string) (model.LLM, error) {
	if endpointURL == "" {
		return nil, fmt.Errorf("huggingface: endpointURL is required for inference endpoint model")
	}
	baseURL := endpointURL
	if !strings.HasSuffix(baseURL, "/v1") {
		baseURL = strings.TrimRight(baseURL, "/") + "/v1"
	}
	return oaibridge.NewModel(ctx, oaibridge.Config{
		Provider:  endpointProvider,
		BaseURL:   baseURL,
		APIKey:    token,
		ModelName: modelName,
		Label:     fmt.Sprintf("HuggingFace Endpoint / %s", modelName),
	})
}

// Package chain builds the multi-provider failover model.LLM from environment
// variables. It is the SINGLE source of truth for provider priority order and
// is shared by both the root binary (console/web) and the TUI.
//
// Before this package existed, the candidate-LLM construction was copy-pasted
// into main.go and cmd/tui/main.go — which silently drifted (the root binary
// omitted OpenCode). Centralising it here guarantees both front-ends expose
// the identical provider set and priority order.
//
// Priority order (highest → lowest):
//
//	GitHub Models → Gemini → Groq → NVIDIA NIM → OpenRouter → OpenCode → HuggingFace → echo (test only)
//
// Every provider is optional; unconfigured providers cost nothing (skipped).
package chain

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"google.golang.org/adk/model"
	"google.golang.org/adk/model/gemini"
	"google.golang.org/genai"

	"go-adk-q/model/echo"
	"go-adk-q/model/failover"
	"go-adk-q/model/githubmodels"
	"go-adk-q/model/groq"
	"go-adk-q/model/huggingface"
	"go-adk-q/model/nvidia"
	"go-adk-q/model/opencode"
	"go-adk-q/model/openrouter"
)

// order is the canonical provider priority. The tag is matched (case-
// insensitively, both directions) against a selected provider string such as
// "github-models", "gemini", "groq", "nvidia", "openrouter", "opencode",
// "huggingface", or "echo".
var order = []string{
	"github", "gemini", "groq", "nvidia",
	"openrouter", "opencode", "huggingface", "echo",
}

// options configures Build.
type options struct {
	selectedProvider string // catalog provider id, e.g. "groq"
	selectedModel    string // model id override for the selected provider
	attemptTimeout   time.Duration
}

// Option customises Build.
type Option func(*options)

// WithSelected moves the named provider to the front of the chain and overrides
// its model id. Used by the TUI /model picker so switching models keeps the
// rest of the failover chain intact (it does NOT drop to a single provider).
func WithSelected(provider, modelID string) Option {
	return func(o *options) { o.selectedProvider = provider; o.selectedModel = modelID }
}

// WithAttemptTimeout sets the per-provider attempt deadline (0 = disabled).
func WithAttemptTimeout(d time.Duration) Option {
	return func(o *options) { o.attemptTimeout = d }
}

// buildFunc constructs a provider model from the environment, applying
// modelOverride (if non-empty) to that provider's model name. Returns nil when
// the provider is not configured.
type buildFunc func(ctx context.Context, modelOverride string) model.LLM

var builders = map[string]buildFunc{
	"github":      buildGitHub,
	"gemini":      buildGemini,
	"groq":        buildGroq,
	"nvidia":      buildNVIDIA,
	"openrouter":  buildOpenRouter,
	"opencode":    buildOpenCode,
	"huggingface": buildHuggingFace,
}

// Build constructs the failover chain from environment variables.
//
// Returns an error if no provider is configured. The result is always a
// *failover.Model so callers can read Stats()/Names() for observability.
func Build(ctx context.Context, opts ...Option) (*failover.Model, error) {
	o := &options{attemptTimeout: 90 * time.Second}
	for _, opt := range opts {
		opt(o)
	}

	var llms []model.LLM
	for _, tag := range order {
		if tag == "echo" {
			continue // handled below so it is always last
		}
		override := ""
		if o.selectedProvider != "" && matchProvider(tag, o.selectedProvider) {
			override = o.selectedModel
		}
		if m := builders[tag](ctx, override); m != nil {
			llms = append(llms, m)
		}
	}

	// Echo stub — zero-credential last resort; activated by ECHO_FALLBACK_ENABLED=1.
	if echo.Enabled() {
		llms = append(llms, echo.Default())
	}

	if len(llms) == 0 {
		return nil, fmt.Errorf(
			"no model providers configured — set at least one of: " +
				"GITHUB_PAT, GOOGLE_API_KEY, GROQ_API_KEY, NVIDIA_API_KEY, OPENROUTER_API_KEY, OPENCODE_API_KEY, HF_TOKEN",
		)
	}

	// PROVIDER_SELECTED env (root binary / bare TUI) re-orders the chain unless
	// an explicit WithSelected was supplied (the /model picker takes precedence).
	if o.selectedProvider == "" {
		if sel := strings.ToLower(strings.TrimSpace(os.Getenv("PROVIDER_SELECTED"))); sel != "" {
			if idx := findProvider(llms, sel); idx > 0 {
				selected := llms[idx]
				llms = append(llms[:idx], llms[idx+1:]...)    // drop it
				llms = append([]model.LLM{selected}, llms...) // make it primary
			}
		}
	}

	// Move a selected provider to the front (only if present and not already first).
	if o.selectedProvider != "" {
		if idx := findProvider(llms, o.selectedProvider); idx > 0 {
			selected := llms[idx]
			llms = append(llms[:idx], llms[idx+1:]...)    // drop it
			llms = append([]model.LLM{selected}, llms...) // make it primary
		}
	}

	f := failover.New(llms[0], llms[1:]...)
	if o.attemptTimeout > 0 {
		f.SetAttemptTimeout(o.attemptTimeout)
	}
	return f, nil
}

// findProvider returns the index of the first model whose name matches sel, or
// -1. Matches by case-insensitive substring in both directions.
func findProvider(llms []model.LLM, sel string) int {
	for i, m := range llms {
		if matchProvider(m.Name(), sel) {
			return i
		}
	}
	return -1
}

// matchProvider reports whether a and b denote the same provider, comparing
// case-insensitively with substring matching in both directions. This tolerates
// the "github" tag vs the "github-models" catalog id, etc.
func matchProvider(a, b string) bool {
	a, b = strings.ToLower(strings.TrimSpace(a)), strings.ToLower(strings.TrimSpace(b))
	if a == "" || b == "" {
		return false
	}
	return strings.Contains(a, b) || strings.Contains(b, a)
}

// ── Per-provider builders ────────────────────────────────────────────────────

func buildGitHub(ctx context.Context, override string) model.LLM {
	cfg := githubmodels.ConfigFromEnv()
	if cfg.PAT == "" {
		return nil
	}
	if override != "" {
		cfg.ModelName = override
	}
	m, err := githubmodels.NewModel(ctx, cfg)
	if err != nil {
		slog.Warn("chain: githubmodels init failed", "error", err)
		return nil
	}
	return m
}

func buildGemini(_ context.Context, override string) model.LLM {
	key := os.Getenv("GOOGLE_API_KEY")
	if key == "" {
		return nil
	}
	name := override
	if name == "" {
		name = os.Getenv("GOOGLE_MODEL")
	}
	if name == "" {
		name = "gemini-2.0-flash"
	}
	m, err := gemini.NewModel(context.Background(), name, &genai.ClientConfig{APIKey: key})
	if err != nil {
		slog.Warn("chain: gemini init failed", "error", err)
		return nil
	}
	return m
}

func buildGroq(ctx context.Context, override string) model.LLM {
	cfg := groq.ConfigFromEnv()
	if cfg.APIKey == "" {
		return nil
	}
	if override != "" {
		cfg.ModelName = override
	}
	m, err := groq.NewModel(ctx, cfg)
	if err != nil {
		slog.Warn("chain: groq init failed", "error", err)
		return nil
	}
	return m
}

func buildNVIDIA(ctx context.Context, override string) model.LLM {
	cfg := nvidia.ConfigFromEnv()
	if cfg.APIKey == "" {
		return nil
	}
	if override != "" {
		cfg.ModelName = override
	}
	m, err := nvidia.NewModel(ctx, cfg)
	if err != nil {
		slog.Warn("chain: nvidia init failed", "error", err)
		return nil
	}
	return m
}

func buildOpenRouter(ctx context.Context, override string) model.LLM {
	cfg := openrouter.ConfigFromEnv()
	if cfg.APIKey == "" {
		return nil
	}
	if cfg.SiteURL == "" {
		cfg.SiteURL = "https://github.com/example/go-adk-q"
	}
	if cfg.AppName == "" {
		cfg.AppName = "go-adk-q ADK reference"
	}
	if override != "" {
		cfg.ModelName = override
	}
	m, err := openrouter.NewModel(ctx, cfg)
	if err != nil {
		slog.Warn("chain: openrouter init failed", "error", err)
		return nil
	}
	return m
}

func buildOpenCode(ctx context.Context, override string) model.LLM {
	cfg := opencode.ConfigFromEnv()
	if cfg.APIKey == "" {
		return nil
	}
	if override != "" {
		cfg.ModelName = override
	}
	m, err := opencode.NewModel(ctx, cfg)
	if err != nil {
		slog.Warn("chain: opencode init failed", "error", err)
		return nil
	}
	return m
}

func buildHuggingFace(ctx context.Context, override string) model.LLM {
	cfg := huggingface.ConfigFromEnv()
	if cfg.Token == "" {
		return nil
	}
	if override != "" {
		cfg.ModelName = override
	}
	m, err := huggingface.NewModel(ctx, cfg)
	if err != nil {
		slog.Warn("chain: huggingface init failed", "error", err)
		return nil
	}
	return m
}

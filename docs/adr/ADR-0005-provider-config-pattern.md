# ADR-0005: Provider Config struct pattern

**Status:** Accepted  
**Date:** 2025  
**Deciders:** Project leads

---

## Context

Each LLM provider package needs a way to:
1. Accept configuration (API key, base URL, model ID).
2. Read configuration from environment variables.
3. Construct a `model.LLM` instance.

Several Go patterns exist for this: positional arguments, functional options,
a config struct, environment-variable-only.

## Decision

Every provider package exports exactly:

```go
type Config struct {
    APIKey  string
    BaseURL string
    Model   string
}

func ConfigFromEnv() Config
func NewModel(ctx context.Context, cfg Config) (model.LLM, error)
```

No positional arguments to `NewModel`. No package-level `New(apiKey, model string)`.

## Rationale

| Property | Positional args | Functional options | Config struct |
|---|---|---|---|
| Self-documenting call sites | ✗ | ✓ | ✓ |
| Easy to extend without breaking callers | ✗ | ✓ | ✓ |
| Works with `ConfigFromEnv()` pattern | awkward | awkward | ✓ |
| Consistent across all providers | depends | depends | ✓ |
| Serialisable / testable | harder | harder | ✓ |

The `Config` struct is self-documenting at the call site:

```go
// Clear and readable — no need to look up argument order:
m, err := groq.NewModel(ctx, groq.Config{
    APIKey: os.Getenv("GROQ_API_KEY"),
    Model:  "llama-3.3-70b-versatile",
})

// vs unclear positional:
m, err := groq.NewModel(ctx, apiKey, model, baseURL)
```

`ConfigFromEnv()` enables zero-friction usage in the common case while
remaining overridable:

```go
cfg := groq.ConfigFromEnv()
cfg.Model = "llama-3.1-8b-instant" // override one field
m, err := groq.NewModel(ctx, cfg)
```

## Consequences

- All provider packages follow the same three-export pattern — easy to learn once.
- Adding a new config field (e.g. `Timeout`) is a non-breaking change.
- `failover.New(primary, backup1, backup2)` can safely receive `nil` (from
  providers that returned an error) because nil models are dropped silently.
- The Gemini provider is an exception: it uses the ADK-native `gemini.New`
  which takes a positional model ID. This is intentional — the ADK's Gemini
  client is not wrapped in a custom package.

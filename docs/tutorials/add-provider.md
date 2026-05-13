# Tutorial: Add a new LLM provider

**Goal:** Implement a new provider package that integrates with the ADK model
interface, registers with the catalog, and appears in the TUI `/model` picker.

**Prerequisites:**
- Completed [get-started.md](get-started.md)
- Familiarity with the existing providers in `model/`

---

## Step 1 — Understand the provider contract

Every provider package in this repo exposes exactly three things:

```go
// Config holds all provider-specific settings.
type Config struct {
    APIKey  string
    BaseURL string // optional; defaults to provider's public endpoint
    Model   string // model ID; e.g. "llama-3.3-70b-versatile"
}

// ConfigFromEnv returns a Config populated from environment variables.
func ConfigFromEnv() Config

// NewModel constructs a model.LLM that satisfies the ADK interface.
func NewModel(ctx context.Context, cfg Config) (model.LLM, error)
```

No positional arguments. No package-level globals other than `KnownModels`.

## Step 2 — Create the package

```sh
mkdir model/myprovider
```

Create `model/myprovider/myprovider.go`:

```go
// Package myprovider integrates MyProvider's OpenAI-compatible API with the
// ADK model.LLM interface via Genkit's compat_oai bridge.
package myprovider

import (
    "context"
    "fmt"
    "os"

    "go-adk-q/model/catalog"
    "go-adk-q/model/oaibridge"
)

const (
    defaultBaseURL = "https://api.myprovider.example/v1"
    defaultModel   = "my-model-id"
    envAPIKey      = "MYPROVIDER_API_KEY"
    envModel       = "MYPROVIDER_MODEL"
    envBaseURL     = "MYPROVIDER_BASE_URL"
)

// KnownModels is the static model catalog for this provider.
// Register it in cmd/tui/main.go's init().
var KnownModels = catalog.ProviderCatalog{
    Provider: "myprovider",
    Label:    "My Provider",
    Models: []catalog.ModelEntry{
        {ID: "my-model-id", Label: "My Model", Default: true},
        {ID: "my-model-mini", Label: "My Model Mini", Tags: []string{"fast"}},
    },
}

// Config holds all settings for the MyProvider backend.
type Config struct {
    APIKey  string
    BaseURL string
    Model   string
}

// ConfigFromEnv returns a Config populated from environment variables.
// Falls back to defaults for BaseURL and Model if not set.
func ConfigFromEnv() Config {
    baseURL := os.Getenv(envBaseURL)
    if baseURL == "" {
        baseURL = defaultBaseURL
    }
    model := os.Getenv(envModel)
    if model == "" {
        model = defaultModel
    }
    return Config{
        APIKey:  os.Getenv(envAPIKey),
        BaseURL: baseURL,
        Model:   model,
    }
}

// NewModel constructs a model.LLM that calls MyProvider's OpenAI-compatible API.
// Returns an error if APIKey is empty.
func NewModel(ctx context.Context, cfg Config) (model.LLM, error) {
    if cfg.APIKey == "" {
        return nil, fmt.Errorf("myprovider: %s is not set", envAPIKey)
    }
    return oaibridge.New(ctx, oaibridge.Config{
        APIKey:    cfg.APIKey,
        BaseURL:   cfg.BaseURL,
        ModelName: cfg.Model,
    })
}
```

## Step 3 — Register the catalog

Open `cmd/tui/main.go` and add to `init()`:

```go
import "go-adk-q/model/myprovider"

func init() {
    // existing registrations ...
    catalog.Register(myprovider.KnownModels)
}
```

## Step 4 — Add to the failover chain

In `buildRunner()` inside `cmd/tui/main.go`, add your provider as a fallback:

```go
myModel, _ := myprovider.NewModel(ctx, myprovider.ConfigFromEnv())

chain := failover.New(primary, backup1, myModel)
```

Nil models (from missing API keys) are safely ignored by `failover.New`.

## Step 5 — Test

```sh
export MYPROVIDER_API_KEY=your_key_here
go build ./...
go run ./cmd/tui chat
```

Type `/model` and you should see "My Provider" listed with your models.

---

## Checklist

- [ ] `Config` struct with `APIKey`, `BaseURL`, `Model`
- [ ] `ConfigFromEnv()` reads env vars with sensible defaults
- [ ] `NewModel(ctx, Config)` returns `(model.LLM, error)`
- [ ] `KnownModels` of type `catalog.ProviderCatalog`
- [ ] `catalog.Register(KnownModels)` in `cmd/tui/main.go init()`
- [ ] Provider added to failover chain in `buildRunner()`
- [ ] `go build ./...` passes with no errors

---

## Notes on non-OpenAI-compatible APIs

If the provider does not expose an OpenAI-compatible chat completions endpoint,
you cannot use `oaibridge.New`. Instead, implement `model.LLM` directly:

```go
type myModel struct{ cfg Config }

func (m *myModel) Name() string { return "myprovider/" + m.cfg.Model }

func (m *myModel) GenerateContent(
    ctx context.Context,
    req *model.LLMRequest,
    stream bool,
) iter.Seq2[*model.LLMResponse, error] {
    return func(yield func(*model.LLMResponse, error) bool) {
        // Call the native API, convert response to *model.LLMResponse.
    }
}
```

See `model/echo/echo.go` for the simplest possible implementation.

---

## Next steps

- [How-to: Add a colour theme](../how-to/add-theme.md)
- [Explanation: Failover design](../explanation/failover.md)
- [Reference: Provider reference](../reference/providers.md)

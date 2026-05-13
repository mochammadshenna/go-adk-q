# Reference: Providers

This page is the authoritative reference for every LLM provider package in
`model/`. Each package follows the same three-export contract.

---

## Contract

```go
type Config struct { ... }
func ConfigFromEnv() Config
func NewModel(ctx context.Context, cfg Config) (model.LLM, error)
```

All providers also export `KnownModels catalog.ProviderCatalog`.

---

## Google Gemini

**Package:** `google.golang.org/adk/model/gemini` (ADK native)

**Key variable:** `GEMINI_API_KEY`

**Usage:**

```go
import "google.golang.org/adk/model/gemini"

m, err := gemini.New(ctx, "gemini-2.0-flash", &genai.ClientConfig{
    APIKey: os.Getenv("GEMINI_API_KEY"),
})
```

Note: Gemini uses the ADK-native client, not oaibridge. The model ID is
passed as a positional argument (ADK convention for the Gemini client).

**Supported models (partial):**

| ID | Notes |
|---|---|
| `gemini-2.0-flash` | Default; fast |
| `gemini-2.5-flash` | Fast; latest |
| `gemini-2.5-pro` | High quality |
| `gemini-1.5-pro` | Long context |
| `gemini-1.5-flash` | Fast |

---

## GitHub Models

**Package:** `go-adk-q/model/githubmodels`

**Key variable:** `GITHUB_TOKEN`

**Config:**

```go
cfg := githubmodels.ConfigFromEnv()
m, err := githubmodels.NewModel(ctx, cfg)
```

**Notes:** Uses the Azure AI Inference API endpoint at
`https://models.inference.ai.azure.com`. GitHub Models are free for personal
accounts within rate limits.

---

## Groq

**Package:** `go-adk-q/model/groq`

**Key variable:** `GROQ_API_KEY`

**Config:**

```go
cfg := groq.ConfigFromEnv()
// cfg.Model defaults to "llama-3.3-70b-versatile"
m, err := groq.NewModel(ctx, cfg)
```

**Supported models (partial):**

| ID | Notes |
|---|---|
| `llama-3.3-70b-versatile` | Default; fast |
| `llama-3.1-8b-instant` | Fastest |
| `mixtral-8x7b-32768` | Long context |

---

## NVIDIA NIM

**Package:** `go-adk-q/model/nvidia`

**Key variable:** `NVIDIA_API_KEY`

**Config:**

```go
cfg := nvidia.ConfigFromEnv()
m, err := nvidia.NewModel(ctx, cfg)
```

**Notes:** Uses `https://integrate.api.nvidia.com/v1` by default. Override
with `NVIDIA_BASE_URL`.

---

## OpenRouter

**Package:** `go-adk-q/model/openrouter`

**Key variable:** `OPENROUTER_API_KEY`

**Config:**

```go
cfg := openrouter.ConfigFromEnv()
m, err := openrouter.NewModel(ctx, cfg)
```

**Notes:** OpenRouter proxies hundreds of models. Set `OPENROUTER_MODEL` to
any model ID listed at openrouter.ai/models.

---

## Hugging Face

**Package:** `go-adk-q/model/huggingface`

**Key variable:** `HUGGINGFACE_API_KEY`

**Config:**

```go
cfg := huggingface.ConfigFromEnv()
m, err := huggingface.NewModel(ctx, cfg)
```

---

## Echo (testing)

**Package:** `go-adk-q/model/echo`

A deterministic model that echoes the user's input. No API key required.
Used in unit tests and as a development placeholder.

```go
import "go-adk-q/model/echo"

m := echo.New()
```

---

## oaibridge

**Package:** `go-adk-q/model/oaibridge`

Internal adapter that wraps any OpenAI-compatible endpoint (via Genkit
`compat_oai`) to produce a `model.LLM`. Used by most provider packages.

```go
import "go-adk-q/model/oaibridge"

m, err := oaibridge.New(ctx, oaibridge.Config{
    APIKey:    "...",
    BaseURL:   "https://api.example.com/v1",
    ModelName: "my-model",
})
```

---

## failover

**Package:** `go-adk-q/model/failover`

Wraps multiple `model.LLM` instances into a single model that tries each
in order, returning the first successful response.

```go
import "go-adk-q/model/failover"

chain := failover.New(primary, backup1, backup2)
// chain.Name() == "failover(primary → backup1 → backup2)"
```

See [explanation/failover.md](../explanation/failover.md) for design details.

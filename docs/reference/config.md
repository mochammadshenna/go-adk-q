# Reference: Configuration

All runtime configuration is via environment variables. No config file is
required. Missing provider keys cause that provider to be skipped silently;
the failover chain continues to the next available provider.

---

## Provider API keys

| Variable | Provider |
|---|---|
| `GEMINI_API_KEY` | Google Gemini (via ADK native client) |
| `GITHUB_TOKEN` | GitHub Models (OpenAI-compatible) |
| `GROQ_API_KEY` | Groq |
| `NVIDIA_API_KEY` | NVIDIA NIM |
| `OPENROUTER_API_KEY` | OpenRouter |
| `HUGGINGFACE_API_KEY` | Hugging Face Inference API |

---

## Provider model overrides

Each provider reads a `_MODEL` variable to override the default model ID:

| Variable | Default | Provider |
|---|---|---|
| `GEMINI_MODEL` | `gemini-2.0-flash` | Google Gemini |
| `GITHUB_MODEL` | (catalog default) | GitHub Models |
| `GROQ_MODEL` | `llama-3.3-70b-versatile` | Groq |
| `NVIDIA_MODEL` | (catalog default) | NVIDIA |
| `OPENROUTER_MODEL` | (catalog default) | OpenRouter |
| `HUGGINGFACE_MODEL` | (catalog default) | Hugging Face |

---

## Provider base URL overrides

Useful for pointing at local proxies, VLLM deployments, or private endpoints:

| Variable | Provider |
|---|---|
| `GROQ_BASE_URL` | Groq |
| `NVIDIA_BASE_URL` | NVIDIA |
| `OPENROUTER_BASE_URL` | OpenRouter |
| `HUGGINGFACE_BASE_URL` | Hugging Face |

---

## Application settings

| Variable | Default | Description |
|---|---|---|
| `GO_ADK_Q_LOG` | `` | Path to write `slog` JSON logs; empty = stderr at WARN |
| `GO_ADK_Q_THEME` | `0` | Starting theme index (0–7); overridden by `--theme` flag |
| `GO_ADK_Q_SESSION_DIR` | `~/.go-adk-q/sessions` | Directory for persisted session files |

---

## Failover priority

Providers are tried in registration order (defined in `cmd/tui/main.go init()`):

1. GitHub Models
2. Google Gemini
3. Groq
4. NVIDIA
5. OpenRouter
6. Hugging Face

The first provider with a non-empty API key and a successful response wins.
Subsequent providers are never called.

Reorder the `catalog.Register` and `failover.New` calls in `cmd/tui/main.go`
to change priority.

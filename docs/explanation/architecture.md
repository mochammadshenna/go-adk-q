# Explanation: Architecture

This document explains **why** go-adk-q is structured the way it is, and
how the major components relate to each other.

---

## High-level diagram

```
┌─────────────────────────────────────────────────────────┐
│                    cmd/tui (Bubbletea TUI)               │
│  ┌─────────┐  ┌──────────┐  ┌────────┐  ┌───────────┐  │
│  │  chat   │  │  slash   │  │ model  │  │ settings  │  │
│  │  .go    │  │  .go     │  │ picker │  │ overlay   │  │
│  └────┬────┘  └──────────┘  └────────┘  └───────────┘  │
│       │                                                   │
└───────┼───────────────────────────────────────────────────┘
        │ ADK runner.Run / RunAsync
        ▼
┌───────────────────────────────────────────────────────────┐
│                     ADK Layer                             │
│  ┌───────────────────┐  ┌────────────────────────────┐   │
│  │ runner.Runner     │  │ session.SessionService     │   │
│  │                   │  │ memory.MemoryService       │   │
│  │ agent.Agent       │  │ artifact.ArtifactService   │   │
│  │ (LlmAgent or      │  └────────────────────────────┘   │
│  │  custom Run func) │                                    │
│  └─────────┬─────────┘                                   │
│            │ model.LLM.GenerateContent                    │
└────────────┼──────────────────────────────────────────────┘
             │
┌────────────┼──────────────────────────────────────────────┐
│            ▼         model/ layer                         │
│  ┌──────────────────┐                                     │
│  │ failover.Model   │ tries each provider left→right      │
│  └──────┬───────────┘                                     │
│         │                                                  │
│  ┌──────┴─────────────────────────────────────────────┐   │
│  │ github  │ gemini │ groq │ nvidia │ openrouter │ hf  │  │
│  └─────────┴────────┴──────┴────────┴────────────┴────┘   │
│         │                                                  │
│  ┌──────┴──────────────┐                                   │
│  │  oaibridge (Genkit  │ OpenAI-compat → genai.Client      │
│  │  compat_oai)        │                                   │
│  └─────────────────────┘                                   │
└───────────────────────────────────────────────────────────┘
```

---

## Layer responsibilities

### cmd/tui — presentation layer

The Bubbletea TUI is a pure view/update layer. It:
- Renders the conversation history, input area, overlays, and footer.
- Sends user input to the ADK runner via `runAsync`.
- Handles slash commands (`/theme`, `/model`, `/settings`, `/help`, `/clear`,
  `/skills`) locally without going through the agent.
- Manages visual state: theme index, picker modes, settings form.

The TUI does **not** contain any model or agent logic.

### ADK layer — agent execution

`google.golang.org/adk` handles:
- Agent lifecycle (`LlmAgent`, `SequentialAgent`, `ParallelAgent`, `LoopAgent`).
- Tool invocation and result injection.
- Session management (conversation history).
- Memory retrieval and preloading.
- Artifact storage and loading.
- Skill toolset exposure.

The runner bridges the TUI and the agent: it owns the session, calls
`agent.Run`, and streams back `LLMResponse` events.

### model/ layer — provider abstraction

Each provider package is a thin adapter that translates between the ADK
`model.LLM` interface and the provider's wire protocol. Most providers use
`oaibridge` to delegate to Genkit's `compat_oai` client, which handles
OpenAI-compatible chat completions.

`failover.Model` wraps multiple `model.LLM` instances and provides
transparent retry across providers.

---

## ADK agent patterns in use

| Pattern | Where | Description |
|---|---|---|
| `LlmAgent` | `cmd/tui/main.go buildRunner()` | Main conversational agent |
| Custom `Run` | `agents/llmauditor.go` | LLM output quality auditor |
| `agenttool` | `agents/collaboration/` | Agent-to-agent delegation |
| `SequentialAgent` | samples | State flows via `OutputKey` + `{key}` interpolation |
| `ParallelAgent` | samples | Concurrent sub-agent execution |
| `LoopAgent` | samples | Iterative refinement |

---

## Skill toolset

<a name="skill-toolset"></a>

The `localskilltoolset` package (`tool/skilltoolset/`) implements
`tool.Toolset`. It:
1. Walks `skills/` at startup, finding all `SKILL.md` files.
2. Creates one `tool.Tool` per file, named by the skill's directory path.
3. When invoked, returns the skill's Markdown content to the agent.

The agent can then follow the skill's instructions directly in its response.

---

## Data flow for a single user message

```
User types message → Enter key
    → TUI sends message to runner via runAsync goroutine
    → runner.RunAsync(ctx, userID, sessionID, content)
    → ADK loads session history
    → LlmAgent builds LLMRequest (history + tools + instruction)
    → failover.Model.GenerateContent tries providers in order
        → provider calls API, buffers full response
        → if error, tries next provider
    → LLMResponse yielded back through iterator
    → TUI receives response parts, renders Markdown
    → Session updated with assistant turn
```

---

## Why Genkit only for oaibridge?

Genkit (`github.com/firebase/genkit/go`) provides a mature, well-tested
OpenAI-compatible client via `compat_oai`. Rather than writing a raw HTTP
client for each provider, `oaibridge` delegates to Genkit's client.

Genkit is **not** used for agent orchestration, flow management, or
telemetry — that is all handled by the ADK. Genkit appears only in the
`model/oaibridge` package.

See [ADR-0002](../adr/ADR-0002-genkit-compat-oai.md) for the decision record.

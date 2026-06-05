# AGENTS.md — go-adk-q AI Coding Instructions

> **Canonical AI memory file.** Every AI coding agent (Claude Code, Cursor,
> Copilot, Gemini, Aider, ChatGPT) must read this before touching code.
> Supersedes any contradictory instruction in the conversation.

---

## §1 Project Identity

**Name**: `go-adk-q`
**Purpose**: Reference implementation of every Google ADK Go agent pattern
with multi-provider LLM failover and a Bubbletea TUI.

**Stack**:
| Layer | Technology |
|---|---|
| Agent orchestration | `google.golang.org/adk` v1.2.0 |
| TUI | Bubbletea + Lipgloss + Glamour (charmbracelet) |
| Provider bridge | Genkit `compat_oai` (oaibridge only) |
| Observability | OpenTelemetry (OTLP) |
| Language | Go 1.24+ |

### Terminology — what to call things

| Correct term | Forbidden alternatives |
|---|---|
| **provider** | backend, vendor, service |
| **LlmAgent** | LLM agent, chat agent, AI agent |
| **FunctionTool** | tool function, action, capability |
| **SkillToolset** | skill plugin, tool plugin, skill loader |
| **failover chain** | fallback chain, retry chain, redundancy |
| **session state** | agent state, memory, context store |
| **OutputKey + `{key}` interpolation** | state passing, variable substitution |
| **oaibridge** | OpenAI bridge, compat bridge, GenKit bridge |
| **SKILL.md** | skill file, skill definition, plugin file |
| **SubAgent** | child agent, nested agent |
| **ExitLoopTool** | break tool, stop tool, loop exit |
| **TUI** | CLI, terminal app, shell UI |

---

## §2 Repository Layout

```
go-adk-q/
├── main.go                        # ALL agent definitions + ADK launcher
│                                  # Do NOT split agents into separate files
├── Makefile                       # All run/test/build targets
├── go.mod                         # Module root: go-adk-q
├── go.sum
├── AGENTS.md                      # ← you are here
├── llms.txt                       # Machine-readable project index (llms.txt standard)
├── llms-full.txt                  # Full RAG context (~15 k tokens)
│
├── cmd/
│   └── tui/
│       ├── main.go                # TUI binary entry point, Cobra subcommands
│       ├── chat.go                # Bubbletea model, rendering, themes
│       ├── markdown.go            # Glamour style configs per theme
│       ├── slash.go               # Slash command autocomplete
│       ├── model_picker.go        # /model overlay (provider + model selection)
│       ├── settings.go            # /settings overlay
│       └── session.go             # Session persistence
│
├── model/
│   ├── catalog/catalog.go         # ModelEntry, ProviderCatalog, global registry
│   ├── echo/echo.go               # Echo stub (TESTING ONLY — never production)
│   ├── failover/
│   │   ├── failover.go            # Multi-provider failover chain (buffered)
│   │   └── failover_test.go       # Unit tests with mock LLMs
│   ├── oaibridge/bridge.go        # ONLY Genkit import; ADK↔Genkit type mapping
│   ├── githubmodels/              # GitHub Models provider
│   ├── groq/                      # Groq LPU provider
│   ├── nvidia/                    # NVIDIA NIM provider
│   ├── openrouter/                # OpenRouter meta-provider
│   ├── huggingface/               # HuggingFace Inference provider
│   └── opencode/                  # OpenCode AI gateway provider
│
├── tools/
│   └── tools.go                   # get_weather + get_current_time FunctionTools
│                                  # Pattern: typed args struct + json+jsonschema tags
│
├── tool/
│   └── skilltoolset/
│       └── skilltoolset.go        # Schema-patched SkillToolset (OpenAI-compat fix)
│
├── skills/                        # SKILL.md library (auto-discovered by SkillToolset)
│   ├── agents/                    # Agent role skills (analyzer, comparator, grader)
│   ├── collaboration/             # Multi-agent collaboration workflows
│   ├── design/                    # Design consultation and review skills
│   ├── documentation/             # Doc drafting, review, iteration skills
│   └── engineering/               # Code review, refactor, debug, go-expert skills
│
├── agents/
│   ├── llmauditor.go              # Custom Run agent (quality auditor)
│   └── collaboration/             # Multi-agent via agenttool patterns
│
├── sample/                        # Standalone usage examples
│   ├── groq/                      # Groq provider usage
│   ├── openrouter/                # OpenRouter with model selection
│   └── opencodezen/               # Multi-agent collaboration example
│
├── tui/                           # TUI helper packages
│
└── docs/                          # Diátaxis documentation (see §13)
    ├── README.md
    ├── ai-memory/                 # AI agent memory files (this tier)
    ├── tutorials/
    ├── how-to/
    ├── reference/
    ├── explanation/
    └── adr/
```

---

## §3 Agent Pattern Rules

### All agents live in `main.go`
- Never move agent definitions to separate files unless creating a new `agents/` sub-package
- The `main()` function is the composition root — all wiring happens there
- Name agents with snake_case: `weather_time_agent`, `doc_refinement_loop`

### FunctionTool skeleton (mandatory pattern)
```go
// Args struct: json tag = parameter name, jsonschema tag = description for LLM
type myToolArgs struct {
    City string `json:"city" jsonschema:"The city name to look up."`
}

// Result struct: json tags define the response shape
type myToolResult struct {
    City   string `json:"city"`
    Result string `json:"result"`
}

// Handler: always _ for tool.Context unless you need session access
func myHandler(_ tool.Context, args myToolArgs) (myToolResult, error) {
    // ... implementation
    return myToolResult{City: args.City, Result: "..."}, nil
}

// Constructor: panic on error — misconfigured tools are programming errors
func NewMyTool() tool.Tool {
    t, err := functiontool.New(functiontool.Config{
        Name:        "my_tool",
        Description: "One-sentence description of what this tool does.",
    }, myHandler)
    if err != nil {
        panic(fmt.Sprintf("NewMyTool: %v", err))
    }
    return t
}
```

### LlmAgent skeleton
```go
myAgent, err := llmagent.New(llmagent.Config{
    Name:  "my_agent",   // snake_case, unique across the session
    Model: m,            // always use failover model m, not a raw provider
    Description: "One sentence used by orchestrator agents to route tasks.",
    Instruction: "System prompt. Reference state with {key} syntax.",
    OutputKey: "result_key",   // omit if agent is a terminal/endpoint
    Tools:    []tool.Tool{tools.NewWeatherTool()},
    Toolsets: agentToolsets,  // nil-safe; add only when skills are needed
})
mustOK(err, "create my_agent")
```

### SequentialAgent state flow
```go
// Stage 1 writes state["step1_output"]
stage1, _ := llmagent.New(llmagent.Config{
    OutputKey: "step1_output",
    Instruction: "...",
})
// Stage 2 reads it via {step1_output} interpolation
stage2, _ := llmagent.New(llmagent.Config{
    Instruction: "Process this: {step1_output}",
})
pipeline, _ := sequentialagent.New(sequentialagent.Config{
    AgentConfig: agent.Config{
        Name:      "my_pipeline",
        SubAgents: []agent.Agent{stage1, stage2},
    },
})
```

### LoopAgent — ALWAYS include ExitLoopTool
```go
// Every agent inside a LoopAgent MUST have ExitLoopTool in its Tools
// Otherwise it runs MaxIterations unconditionally (silent logic error)
checker, _ := llmagent.New(llmagent.Config{
    Tools: []tool.Tool{exitlooptool.New()},   // ← mandatory
    Instruction: "If done, call exit_loop. Otherwise suggest improvement.",
})
loop, _ := loopagent.New(loopagent.Config{
    MaxIterations: 3,
    AgentConfig: agent.Config{SubAgents: []agent.Agent{writer, checker}},
})
```

### Provider agents (Groq, NVIDIA, OpenRouter, HuggingFace)
```go
// NEVER assign both Tools and Toolsets to provider-specific agents
// Groq/NVIDIA return tool_use_failed for multi-tool prompts
groqAgent, _ := llmagent.New(llmagent.Config{
    Name:  "groq_agent",
    Model: groqLLM,      // ← raw provider model, NOT the failover chain m
    // NO Tools, NO Toolsets
})
```

---

## §4 Provider / Model Layer Rules

### The failover chain `m` is the canonical model
```
GitHub Models → Gemini → Groq → NVIDIA NIM → OpenRouter → HuggingFace → Echo (dev only)
```
- All production agents use `m` (the failover chain), never a raw provider
- Provider-specific agents (groq_agent etc.) exist only for explicit comparison
- `failover.New` silently skips nil entries — safe to pass unconfigured providers

### oaibridge boundary (CRITICAL)
```
model/oaibridge   ← ONLY file that may import github.com/firebase/genkit
model/{groq,...}  ← import oaibridge, not genkit directly
all other code    ← must NEVER import firebase/genkit
```

### Provider package pattern
Each provider package must expose:
- `Config` struct with `APIKey`, `ModelName`, optionally `BaseURL`
- `ConfigFromEnv() Config` reading from env vars
- `NewModel(ctx, cfg) (model.LLM, error)` returning nil if not configured
- `KnownModels catalog.ProviderCatalog` for the /model picker

### echo model — never in production
```go
// ONLY activate via ECHO_FALLBACK_ENABLED=1 in local dev/CI
// The echo model returns static text — it cannot reason
if os.Getenv("ECHO_FALLBACK_ENABLED") == "1" {
    // ... append echo to chain
}
```

---

## §5 Standard Code Skeletons

### New FunctionTool (copy-paste ready)
```go
// In tools/tools.go or a new tools/mytool/mytool.go

type myThingArgs struct {
    Input string `json:"input" jsonschema:"The input value to process."`
}

type myThingResult struct {
    Output string `json:"output"`
    Source string `json:"source"`
}

func myThing(_ tool.Context, args myThingArgs) (myThingResult, error) {
    if args.Input == "" {
        return myThingResult{}, fmt.Errorf("input must not be empty")
    }
    return myThingResult{Output: args.Input, Source: "example"}, nil
}

func NewMyThingTool() tool.Tool {
    t, err := functiontool.New(functiontool.Config{
        Name:        "my_thing",
        Description: "Processes input and returns an output.",
    }, myThing)
    if err != nil {
        panic(fmt.Sprintf("NewMyThingTool: %v", err))
    }
    return t
}
```

### New provider package (copy-paste ready)
```go
// model/myprovider/myprovider.go
package myprovider

import (
    "context"
    "go-adk-q/model/catalog"
    "go-adk-q/model/oaibridge"
    "google.golang.org/adk/model"
)

const DefaultModel = "my-model-v1"

type Config struct {
    APIKey    string
    ModelName string
    BaseURL   string
}

func ConfigFromEnv() Config {
    return Config{
        APIKey:    os.Getenv("MYPROVIDER_API_KEY"),
        ModelName: firstNonEmpty(os.Getenv("MYPROVIDER_MODEL"), DefaultModel),
        BaseURL:   firstNonEmpty(os.Getenv("MYPROVIDER_BASE_URL"), "https://api.myprovider.com/v1"),
    }
}

func NewModel(_ context.Context, cfg Config) (model.LLM, error) {
    if cfg.APIKey == "" {
        return nil, nil // unconfigured — failover skips nil entries
    }
    return oaibridge.NewModel(oaibridge.Config{
        Name:    "myprovider/" + cfg.ModelName,
        BaseURL: cfg.BaseURL,
        APIKey:  cfg.APIKey,
        Model:   cfg.ModelName,
    })
}

var KnownModels = catalog.ProviderCatalog{
    Provider: "myprovider",
    Models: []catalog.ModelEntry{
        {ID: DefaultModel, Description: "My Provider default model"},
    },
}
```

### New agent in main.go (copy-paste ready)
```go
// Add after existing agent definitions in main()

myNewAgent, err := llmagent.New(llmagent.Config{
    Name:  "my_new_agent",
    Model: m,    // always the failover chain, not a raw provider
    Description: "Single sentence for root agent routing. Be specific.",
    Instruction: `You are a specialist for <domain>.
<Context from state if needed: {prior_output_key}>
<Hard rules for this agent.>`,
    OutputKey: "my_new_output",    // omit for endpoint agents
    Tools:    []tool.Tool{tools.NewWeatherTool()},
    Toolsets: agentToolsets,
})
mustOK(err, "create my_new_agent")

// Then add to rootAgent.SubAgents and update rootAgent.Instruction routing table
```

---

## §6 Naming Conventions

| Thing | Convention | Example |
|---|---|---|
| Agent Name field | `snake_case` | `weather_time_agent` |
| OutputKey | `snake_case` noun | `generated_code`, `quality_verdict` |
| Tool Name | `snake_case` verb_noun | `get_weather`, `save_artifact` |
| Provider package | `lowercase` | `model/groq`, `model/nvidia` |
| Provider env vars | `UPPER_SNAKE_CASE` with provider prefix | `GROQ_API_KEY`, `NVIDIA_MODEL` |
| TUI theme | `lowercase` word | `dark`, `light`, `ocean` |
| SKILL.md directory | `kebab-case` | `skills/engineering/go-expert/` |
| ADR files | `ADR-NNNN-slug.md` | `ADR-0006-skills-toolset.md` |

---

## §7 Data / State Model Conventions

### Session state
- Session state is `map[string]any` under `ctx.Session().State()`
- Read: `state.Get("key")` → `(any, error)`
- Write: set via `OutputKey` on LlmAgent (ADK manages it); manual via `state.Set()`
- Keys must be globally unique within a pipeline — duplicate `OutputKey` silently overwrites
- State persists across turns in the same session — use unique prefixes for turn-scoped data

### Artifacts
- Binary/large data → `ctx.Session().Artifacts()` (InMemoryService in dev)
- Tools can call `artifact.Save(ctx, name, data)` / `artifact.Load(ctx, name)`

### No external DB
- This project has no database — all state is in-memory or session-scoped
- For persistent storage patterns, see `sample/` or extend with your own store

---

## §8 Testing Conventions

```sh
go test ./...          # all unit tests
make test              # same via Makefile
make test-failover-echo    # failover demo without real API keys
make test-failover-live    # failover demo with real providers
```

- Test files: `*_test.go` co-located with the package under test
- Mock LLMs: implement `model.LLM` interface (see `model/failover/failover_test.go`)
- No test framework — standard `testing.T` + `t.Errorf`
- Test names: `Test<What>_<Condition>` e.g. `TestFailover_FirstProviderFails`
- Provider integration tests require real API keys; use `t.Skip` when unset

```go
// Standard skip pattern for provider tests
func TestGroqRoundtrip(t *testing.T) {
    if os.Getenv("GROQ_API_KEY") == "" {
        t.Skip("GROQ_API_KEY not set")
    }
    // ...
}
```

---

## §9 Git / PR Workflow

```
main        — always deployable / runnable
feature/*   — new agents, providers, TUI features
fix/*       — bug fixes
docs/*      — documentation-only changes
```

- Commits: imperative present tense — "Add Groq provider", "Fix echo nil check"
- PR title format: `[type] short description` e.g. `[feat] Add OpenCode provider`
- One logical change per commit
- Run `make vet && make lint && make test` before push
- ADR required for: new providers, agent pattern changes, dependency additions

---

## §10 Environment Setup

```sh
# 1. Clone
git clone <repo-url> go-adk-q && cd go-adk-q

# 2. Ensure Go 1.24+
go version

# 3. Set at least one provider key (minimum for a working agent)
export GOOGLE_API_KEY=<key>      # from aistudio.google.com/apikey (free tier)
# OR
export GITHUB_TOKEN=<PAT>        # GitHub Models (free with GitHub account)

# 4. Optionally source a .env file (never commit)
# echo 'export GOOGLE_API_KEY=...' > .env && source .env

# 5. Run the TUI
make run                          # Bubbletea interactive TUI
# OR
make console                      # ADK built-in text console
# OR
make web                          # Browser dev-UI at http://localhost:8080

# 6. Run tests
make test

# 7. Check which env vars are set
make env
```

Required: at least one provider key. The failover chain tries configured providers in priority order:
`GitHub Models → Gemini → Groq → NVIDIA NIM → OpenRouter → HuggingFace → [Echo if enabled]`

Full env var reference: [`docs/reference/config.md`](docs/reference/config.md)

---

## §11 Skills System

Skills are plain Markdown files (`SKILL.md`) that inject system instructions into an LLM via the SkillToolset.

### SKILL.md format
```markdown
---
name: my-skill
description: One sentence — shown to LLM in list_skills, used for routing.
license: MIT
compatibility: google.golang.org/adk v1+
---

# My Skill Name

(Full instructions the LLM follows when this skill is loaded.)
```

### Registration
Place `SKILL.md` under `skills/<category>/<skill-name>/SKILL.md`. The SkillToolset discovers all files automatically — no code changes needed.

### Schema patch (important)
The local `tool/skilltoolset` package wraps the upstream ADK SkillToolset with two fixes:
1. `list_skills`: adds explicit `"properties": {}` — required by OpenAI-compatible APIs
2. `load_skill`: explicit `name` field schema — prevents small model misreads

Never bypass this wrapper. Always use `localskilltoolset.New(...)`.

---

## §12 Companion Repos / Related

| Resource | URL |
|---|---|
| Google ADK Go | `google.golang.org/adk` (go module) |
| ADK Go source | https://github.com/google/adk-go |
| Genkit Go | `github.com/firebase/genkit/go` |
| Bubbletea | `github.com/charmbracelet/bubbletea` |
| wttr.in (weather) | https://wttr.in (no key needed) |
| timeapi.io (time) | https://timeapi.io (no key needed) |

---

## §13 Documentation Map

| File | Type (Diátaxis) | Contents |
|---|---|---|
| `docs/README.md` | Index | Navigation hub, personas, quick reference |
| `docs/ai-memory/QUICK_CONTEXT.md` | AI Memory | ~300 token project summary for one-off prompts |
| `docs/ai-memory/SYSTEM_PROMPT.md` | AI Memory | ~2000 token full system prompt for chat sessions |
| `docs/ai-memory/README.md` | AI Memory | Tool-by-tool AI onboarding guide |
| `docs/tutorials/get-started.md` | Tutorial | Zero to running TUI in 5 minutes |
| `docs/tutorials/first-agent.md` | Tutorial | Build and run a custom FunctionTool agent |
| `docs/tutorials/add-provider.md` | Tutorial | Add a new LLM provider end-to-end |
| `docs/how-to/add-skill.md` | How-To | Add a SKILL.md to the skill library |
| `docs/how-to/add-agent.md` | How-To | Add a new agent to main.go |
| `docs/how-to/add-theme.md` | How-To | Add a TUI colour theme |
| `docs/how-to/add-provider.md` | How-To | Provider checklist (quick reference) |
| `docs/how-to/debug-tui.md` | How-To | Debug TUI rendering bugs |
| `docs/reference/cli.md` | Reference | CLI flags, keyboard shortcuts, slash commands |
| `docs/reference/config.md` | Reference | All environment variables |
| `docs/reference/api.md` | Reference | Go package API reference |
| `docs/reference/providers.md` | Reference | Provider package reference |
| `docs/reference/themes.md` | Reference | Theme index and palette fields |
| `docs/explanation/architecture.md` | Explanation | System diagram, data flow, layer responsibilities |
| `docs/explanation/failover.md` | Explanation | Failover design, buffering trade-off, nil handling |
| `docs/explanation/tui-rendering.md` | Explanation | Bubbletea rendering model, themes, markdown |
| `docs/explanation/pitfalls.md` | Explanation | 10 documented gotchas with root cause + fix |
| `docs/adr/ADR-0001-adk-as-agent-layer.md` | ADR | Why ADK (not Genkit flows or custom framework) |
| `docs/adr/ADR-0002-genkit-compat-oai.md` | ADR | Genkit used only for oaibridge |
| `docs/adr/ADR-0003-failover-buffered.md` | ADR | Buffered failover rationale |
| `docs/adr/ADR-0004-lipgloss-join-vertical.md` | ADR | Why JoinVertical was removed |
| `docs/adr/ADR-0005-provider-config-pattern.md` | ADR | Config struct + ConfigFromEnv pattern |
| `docs/adr/ADR-0006-skills-toolset.md` | ADR | Why skills use SKILL.md + SkillToolset |
| `docs/adr/ADR-0007-skilltoolset-schema-patch.md` | ADR | Why list_skills/load_skill schemas are patched |
| `llms.txt` | Machine-readable | Project index for LLM crawlers |
| `llms-full.txt` | Machine-readable | Full RAG context (~15k tokens) |
| `.cursorrules` | IDE | Cursor auto-loaded coding rules |
| `.github/copilot-instructions.md` | IDE | Copilot auto-loaded instructions |
| `.aider.conf.yml` | IDE | Aider session pre-load config |

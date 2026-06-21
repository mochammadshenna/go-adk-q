# go-adk-q — Google ADK Go Reference Implementation

Minimal, complete reference implementation of **[google.golang.org/adk](https://adk.dev)**
for Go. Every core ADK pattern in one codebase.

## What's covered

| # | Pattern | File | ADK Docs |
|---|---------|------|---------|
| 1 | **FunctionTool** — typed Go fn → ADK tool | `tools/tools.go` | [function-tools](https://adk.dev/tools-custom/function-tools/) |
| 2 | **LlmAgent** — LLM-driven, non-deterministic | `main.go` | [llm-agents](https://adk.dev/agents/llm-agents/) |
| 3 | **SequentialAgent** — strict ordered pipeline | `main.go` | [sequential-agents](https://adk.dev/agents/workflow-agents/sequential-agents/) |
| 4 | **LoopAgent** — iterative refinement | `main.go` | [loop-agents](https://adk.dev/agents/workflow-agents/loop-agents/) |
| 5 | **ParallelAgent** — concurrent execution | `main.go` | [parallel-agents](https://adk.dev/agents/workflow-agents/parallel-agents/) |
| 6 | **Custom agent** — direct `Run` func | `main.go` | [custom-agents](https://adk.dev/agents/custom-agents/) |
| 7 | **Multi-agent system** — `agenttool.New` | `main.go` | [multi-agents](https://adk.dev/agents/multi-agents/) |
| 8 | **Launcher** — run/web/api_server modes | `main.go` | [multi-tool tutorial](https://adk.dev/tutorials/multi-tool-agent/) |

## Companion Projects

| Project | Description | Tools |
|---------|-------------|-------|
| [`dino-mcp/`](dino-mcp/) | Go MCP server with interactive HTML dashboard | `dino_think`, `dino_ask`, `dino_dashboard` |

Built with Gin, ext-apps SDK, and the MCP Go SDK. All 13 docs indexed in context-mode:

```
ctx_search(queries: ["dino dashboard", "add tool", "MCP Apps"], source: "dino-mcp-docs")
```

| Doc | Purpose |
|-----|---------|
| [`docs/tutorials/get-started.md`](dino-mcp/docs/tutorials/get-started.md) | Build + run in 5 min |
| [`docs/tutorials/first-tool.md`](dino-mcp/docs/tutorials/first-tool.md) | Add a tool to the server |
| [`docs/how-to/add-dinosaur.md`](dino-mcp/docs/how-to/add-dinosaur.md) | Extend the dino data model |
| [`docs/reference/cli.md`](dino-mcp/docs/reference/cli.md) | CLI flags + routes reference |
| [`docs/explanation/architecture.md`](dino-mcp/docs/explanation/architecture.md) | System design + data flow |
| [`docs/adr/`](dino-mcp/docs/adr/) | 6 Architecture Decision Records |

Add to Pi Agent:

```json
{
  "mcpServers": {
    "dino-mcp": {
      "command": "/path/to/dino-mcp/bin/dino-mcp",
      "args": ["stdio"],
      "lifecycle": "lazy"
    }
  }
}
```

---

## Architecture

root_agent  (LlmAgent — coordinator)
├── weather_time_agent   LlmAgent + get_weather + get_current_time tools
├── code_pipeline        SequentialAgent: CodeWriter → CodeReviewer → CodeRefactorer
│                        (OutputKey passes state between stages)
├── doc_refinement_loop  LoopAgent(max=3): DocDrafter → QualityChecker
├── parallel_analysis    ParallelAgent: TechResearcher ∥ BizAnalyst
└── router_agent         Custom agent: Run func with session.State conditional logic

```

## Run

```sh
export GOOGLE_API_KEY=<your-key>

go run . adk run          # interactive terminal
go run . adk web          # browser dev-UI → http://localhost:8000
go run . adk api_server   # REST API      → http://localhost:8080
```

Get a free API key at [aistudio.google.com](https://aistudio.google.com).

## Key patterns quick-reference

### FunctionTool (typed args + json/jsonschema tags)

```go
type getWeatherArgs struct {
    City string `json:"city" jsonschema:"The city name."`
}
func getWeather(_ tool.Context, args getWeatherArgs) (Result, error) { ... }
t, _ := functiontool.New(functiontool.Config{Name: "get_weather", Description: "..."}, getWeather)
```

### LlmAgent

```go
a, _ := llmagent.New(llmagent.Config{
    Name: "my_agent", Model: m, Instruction: "...", Tools: []tool.Tool{t},
    OutputKey: "result",  // saves reply to session state
})
```

### SequentialAgent — state flows via OutputKey + {key} interpolation

```go
seq, _ := sequentialagent.New(sequentialagent.Config{
    AgentConfig: agent.Config{Name: "pipeline", SubAgents: []agent.Agent{a, b, c}},
})
```

### LoopAgent

```go
loop, _ := loopagent.New(loopagent.Config{
    MaxIterations: 3,
    AgentConfig: agent.Config{Name: "loop", SubAgents: []agent.Agent{drafter, checker}},
})
```

### ParallelAgent

```go
par, _ := parallelagent.New(parallelagent.Config{
    AgentConfig: agent.Config{Name: "parallel", SubAgents: []agent.Agent{a, b}},
})
```

### Custom agent (direct Run func)

```go
custom, _ := agent.New(agent.Config{
    Name: "router",
    Run: func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
        return func(yield func(*session.Event, error) bool) {
            v, _ := ctx.Session().State().Get("key")
            yield(&session.Event{LLMResponse: model.LLMResponse{...}}, nil)
        }
    },
})
```

### Multi-agent via agenttool

```go
root, _ := llmagent.New(llmagent.Config{
    Name: "root", Model: m,
    Tools: []tool.Tool{
        agenttool.New(subAgent, nil),  // sub-agent callable by name
    },
})
```

## Project structure

```
go-adk-q/
├── main.go                    # All agent definitions + ADK launcher
├── tools/
│   └── tools.go               # FunctionTools: get_weather, get_current_time
├── cmd/
│   └── tui/                   # Bubbletea TUI (chat, themes, markdown, slash cmds)
│       ├── main.go            # Cobra entry point
│       ├── chat.go            # Bubbletea model + rendering (~95KB)
│       ├── markdown.go        # Glamour renderer per theme
│       ├── slash.go           # Slash command autocomplete
│       ├── model_picker.go    # /model provider overlay
│       ├── settings.go        # /settings overlay
│       ├── session.go         # Session persistence
│       └── acp_server.go      # Alternative Compute Protocol server
├── model/                     # LLM providers + failover chain
├── agents/                    # Custom agent implementations
├── skills/                    # SKILL.md library (auto-discovered)
├── tool/                      # SkillToolset schema patch
├── go.mod                     # google.golang.org/adk v1.2.0
└── dino-mcp/                  # Companion: Go MCP server (see above)
```

## Style

- [Effective Go](https://go.dev/doc/effective_go)
- [Google Go Style Guide](https://google.github.io/styleguide/go/)
- Fail-fast construction (`mustOK`), no global state, no `init()`

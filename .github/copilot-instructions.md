# go-adk-q — GitHub Copilot Instructions

This is a Google ADK Go reference implementation. Stack: Go 1.24 ·
`google.golang.org/adk` v1.2.0 · Bubbletea TUI · multi-provider LLM failover.

## Non-negotiable rules

- **All agent definitions live in `main.go`** — the composition root. Do not create separate files for individual agents.
- **Always use `m` as Model** — `m` is the `failover.Model` wrapping all providers. Never use a raw provider variable (groqLLM, nvidiaLLM, etc.) in a production agent.
- **FunctionTool args structs require both tags** — `json:"field"` for parameter name AND `jsonschema:"Description."` for LLM description. Missing `jsonschema` makes tools invisible to the LLM.
- **Never import `firebase/genkit` outside `model/oaibridge`** — doing so causes silent duplicate init. Use `oaibridge.NewModel(...)` instead.
- **Never add Tools or Toolsets to Groq/NVIDIA agents** — these providers return `tool_use_failed` for multi-tool prompts.
- **Always add `exitlooptool.New()` to LoopAgent sub-agents** that should terminate early. Without it the loop always runs MaxIterations.
- **Unique OutputKey per SequentialAgent stage** — duplicate keys silently overwrite each other.
- **`ECHO_FALLBACK_ENABLED=1` is dev/CI only** — never deploy with it.

## Terminology

| Use | Avoid |
|---|---|
| provider | backend, vendor |
| LlmAgent | LLM agent, chat agent |
| FunctionTool | tool function, action |
| SkillToolset | skill plugin, tool plugin |
| failover chain | fallback chain, retry chain |
| session state | agent state, memory |
| oaibridge | OpenAI bridge, compat bridge |
| SKILL.md | skill file, plugin file |

## Code patterns

### FunctionTool (complete pattern)

```go
type myArgs struct {
    City string `json:"city" jsonschema:"The city name to look up."`
}
type myResult struct {
    Output string `json:"output"`
}
func myHandler(_ tool.Context, args myArgs) (myResult, error) {
    if args.City == "" {
        return myResult{}, fmt.Errorf("city required")
    }
    return myResult{Output: "result for " + args.City}, nil
}
func NewMyTool() tool.Tool {
    t, err := functiontool.New(functiontool.Config{
        Name:        "my_tool",
        Description: "Returns result for the given city.",
    }, myHandler)
    if err != nil {
        panic(fmt.Sprintf("NewMyTool: %v", err))
    }
    return t
}
```

### New agent (complete pattern)

```go
myAgent, err := llmagent.New(llmagent.Config{
    Name:        "my_agent",
    Model:       m,               // always failover chain
    Description: "Handles <specific task>. Be precise for root agent routing.",
    Instruction: "You are a specialist. Use {prior_key} for state from prior stage.",
    OutputKey:   "my_output",     // omit for terminal agents
    Tools:       []tool.Tool{tools.NewWeatherTool()},
    Toolsets:    agentToolsets,   // nil-safe; provides list_skills + load_skill
})
mustOK(err, "create my_agent")
```

### New provider (complete pattern)

```go
// model/myprovider/myprovider.go
func NewModel(_ context.Context, cfg Config) (model.LLM, error) {
    if cfg.APIKey == "" {
        return nil, nil  // unconfigured — failover.New skips nil entries safely
    }
    return oaibridge.NewModel(oaibridge.Config{
        Name:    "myprovider/" + cfg.ModelName,
        BaseURL: cfg.BaseURL,
        APIKey:  cfg.APIKey,
        Model:   cfg.ModelName,
    })
}
```

## Key file locations

| File | Purpose |
|---|---|
| `main.go` | All agent definitions + ADK launcher |
| `tools/tools.go` | FunctionTools (get_weather, get_current_time) |
| `model/failover/failover.go` | Multi-provider failover chain |
| `model/oaibridge/bridge.go` | ONLY location for firebase/genkit import |
| `cmd/tui/main.go` | Bubbletea TUI entry point |
| `tool/skilltoolset/skilltoolset.go` | Schema-patched SkillToolset |
| `skills/` | SKILL.md library auto-discovered at runtime |
| `AGENTS.md` | Full AI coding instructions |
| `llms-full.txt` | Full project context for RAG |

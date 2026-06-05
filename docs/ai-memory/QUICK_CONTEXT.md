# go-adk-q — Quick Context (~300 tokens)

**Stack**: Go 1.24 · `google.golang.org/adk` v1.2.0 · Bubbletea TUI · multi-provider LLM failover

## Hard Rules

- **All agent definitions live in `main.go`** — never split unless adding an `agents/` sub-package
- **Always use `m` (failover chain)** as the model in new agents — never a raw provider model
- **FunctionTool args structs require both `json:` and `jsonschema:` tags** — missing `jsonschema` breaks LLM tool descriptions
- **Never assign Tools or Toolsets to Groq/NVIDIA agents** — they return `tool_use_failed` for multi-tool prompts
- **LoopAgent sub-agents must include `exitlooptool.New()`** in their Tools — otherwise loops always run MaxIterations
- **`firebase/genkit` may only be imported in `model/oaibridge`** — importing it elsewhere causes silent init conflicts
- **`ECHO_FALLBACK_ENABLED=1` is dev/CI only** — the echo model returns static text, never use in production
- **Duplicate OutputKey between SequentialAgent stages silently overwrites** — use unique keys

## Code Skeleton

```go
// New FunctionTool (in tools/tools.go or tools/<name>/<name>.go)
type myArgs struct {
    Input string `json:"input" jsonschema:"Description for the LLM."`
}
type myResult struct { Output string `json:"output"` }

func myHandler(_ tool.Context, a myArgs) (myResult, error) {
    return myResult{Output: a.Input}, nil
}
func NewMyTool() tool.Tool {
    t, _ := functiontool.New(functiontool.Config{Name:"my_tool", Description:"..."}, myHandler)
    return t
}
```

## Key File Paths

| What | Where |
|---|---|
| All agents | `main.go` |
| FunctionTools | `tools/tools.go` |
| Failover chain | `model/failover/failover.go` |
| TUI entry | `cmd/tui/main.go` |
| SkillToolset patch | `tool/skilltoolset/skilltoolset.go` |
| Skills library | `skills/<category>/<name>/SKILL.md` |
| Provider packages | `model/<providerName>/` |
| Full AI context | `llms-full.txt` |
| All rules | `AGENTS.md` |

## Failover Priority

`GitHub Models → Gemini → Groq → NVIDIA NIM → OpenRouter → HuggingFace → Echo (dev only)`

Required env: at least one of `GITHUB_TOKEN`, `GOOGLE_API_KEY`, `GROQ_API_KEY`, `NVIDIA_API_KEY`, `OPENROUTER_API_KEY`, `HF_TOKEN`.

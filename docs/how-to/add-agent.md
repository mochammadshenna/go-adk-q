# How-To: Add a New Agent to `main.go`

> **Goal**: Add a working agent to the root agent's routing table, following
> all mandatory patterns. Time required: ~15 minutes.

---

## Prerequisites

- Go 1.24+ installed
- At least one provider API key set (`make env` to check)
- Repo cloned and `make test` passing

---

## Step 1 — Understand where agents live

All agents are defined in `main.go`. The file is structured in sections:

```
// ── 1. Models ──────────  provider LLMs + failover chain m
// ── 2. Skills ──────────  agentToolsets (SkillToolset init)
// ── 3. Tools ───────────  FunctionTool instances
// ── 4. LlmAgent ────────  single conversational agents
// ── 5. SequentialAgent ─  pipeline agents
// ── 6. LoopAgent ────────  refinement loops
// ── 7. ParallelAgent ────  concurrent analysis
// ── 8. Custom agent ─────  custom Run func agents
// ── 9. Root agent ───────  rootAgent wiring all SubAgents
// ── 10. Launcher ────────  launcher.Config + launcher.Run
```

Add your agent in the appropriate section (§4–§8) and register it in §9.

---

## Step 2 — Define the agent

Open `main.go` and add your agent after the existing agents of the same type.

### Simple LlmAgent

```go
// ── Add in §4 — LlmAgent ────────────────────────────────────────────────────
myExpertAgent, err := llmagent.New(llmagent.Config{
    Name:  "my_expert_agent",          // snake_case, unique in this session
    Model: m,                          // ALWAYS the failover chain
    Description: "Answers questions about <your domain>. " +
        "Use when the user asks about <specific topics>.",
    Instruction: `You are an expert in <domain>.

Rules:
- Always cite your reasoning.
- If uncertain, say so explicitly.
- Keep responses under 500 words unless the user requests more detail.`,
    // OutputKey: "my_output",         // set if downstream agents need this output
    Tools:    []tool.Tool{
        // tools.NewWeatherTool(),     // add tools if needed
    },
    Toolsets: agentToolsets,           // provides list_skills + load_skill
})
mustOK(err, "create my_expert_agent")
```

### If your agent needs a FunctionTool first

Before defining the agent, add the tool. See `AGENTS.md §5` for the full
FunctionTool skeleton, or read [`docs/how-to/add-skill.md`](add-skill.md) for
skill-based capability extension.

---

## Step 3 — Register in the root agent

Find `rootAgent` in `main.go` (§9). Add your agent to:

1. `SubAgents` slice:

```go
rootAgent, err := llmagent.New(llmagent.Config{
    // ...
    SubAgents: []agent.Agent{
        weatherTimeAgent,
        codePipeline,
        docRefinementLoop,
        parallelAnalysis,
        routerAgent,
        notebookAgent,
        myExpertAgent,     // ← add here
    },
})
```

2. The routing table in `rootAgent.Instruction`:

```
IMPORTANT: Only delegate to a sub-agent when the user explicitly requests that type of work:
- Weather or time for a specific city     → weather_time_agent
...
- Questions about <your domain>           → my_expert_agent    ← add this line
```

---

## Step 4 — Test the agent

```sh
# Start the TUI
make run

# Send a message that should route to your agent:
"[Your domain question here]"

# Check the routing in logs (make run shows structured logs):
# INFO  agent selected  agent=my_expert_agent

# One-shot test (faster iteration):
go run ./cmd/tui run "Your test question"
```

---

## Step 5 — Write a unit test (optional but recommended)

```go
// In model/failover/failover_test.go or a new file my_agent_test.go

func TestMyExpertAgent_Routes(t *testing.T) {
    if os.Getenv("GOOGLE_API_KEY") == "" {
        t.Skip("GOOGLE_API_KEY not set")
    }
    // Integration test: verify routing and basic response quality
    // ...
}
```

---

## Checklist

- [ ] Agent defined in `main.go` in the correct section (§4–§8)
- [ ] `Model: m` — using failover chain, not a raw provider
- [ ] `Name` is snake_case and unique across all agents
- [ ] `Description` is precise (root agent uses it for routing)
- [ ] `mustOK(err, "create agent_name")` immediately after construction
- [ ] `OutputKey` is unique if set; not set for terminal agents
- [ ] If Groq/NVIDIA-targeted: no `Tools` or `Toolsets`
- [ ] If inside a LoopAgent: `exitlooptool.New()` in `Tools`
- [ ] Added to `rootAgent.SubAgents`
- [ ] Routing line added to `rootAgent.Instruction`
- [ ] `make test` passes
- [ ] Manual TUI smoke test done

---

## Common mistakes

| Mistake | Symptom | Fix |
|---|---|---|
| `Model: groqLLM` instead of `Model: m` | Agent only works when Groq key is set; fails for all other providers | Use `m` |
| Missing `Description` | Root agent cannot route to this agent; all messages go to default | Add a precise one-sentence description |
| `OutputKey` collision | Prior stage output disappears | Use a unique key |
| Added to SubAgents but not to routing Instruction | Agent is never selected | Add routing line |
| FunctionTool missing `jsonschema:` tag | Tool never called by LLM | Add tag to every args field |

---

## Related

- [`docs/tutorials/first-agent.md`](../tutorials/first-agent.md) — end-to-end tutorial with FunctionTool
- [`AGENTS.md §3`](../../AGENTS.md) — complete agent pattern reference
- [`docs/how-to/add-skill.md`](add-skill.md) — extend agent via SKILL.md instead of new agent
- [`docs/explanation/architecture.md`](../explanation/architecture.md) — how agents are wired

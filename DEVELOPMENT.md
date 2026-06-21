# go-adk-q — Development Workflow

> Step-by-step: change → test → verify → commit.

---

## Prerequisites

```bash
# Go 1.24+
go version

# At least one provider key
export GOOGLE_API_KEY=<key>   # or GITHUB_TOKEN, GROQ_API_KEY, etc.

# Optional: Bubbletea TUI dependencies
# (auto-installed via go.mod)
```

---

## Quick Start

```bash
# Run the interactive TUI
go run . adk run

# Or browser dev-UI
go run . adk web

# Or REST API
go run . adk api_server
```

---

## Change Workflow

### 1. What can you change?

| Change | Files | Example |
|--------|-------|---------|
| New agent | `main.go` | Add a LlmAgent + register in root agent |
| New tool | `tools/tools.go` | Add FunctionTool + handler |
| New provider | `model/<name>/` | Copy existing provider, swap config |
| TUI feature | `cmd/tui/` | Slash command, theme, keybinding |
| Skill | `skills/<category>/<name>/` | Write a SKILL.md |
| Documentation | `docs/` | Diátaxis: tutorial, how-to, reference, explanation |

### 2. Agent workflow

```go
// 1. Define in main.go
myAgent, err := llmagent.New(llmagent.Config{
    Name:  "my_agent",
    Model: m,    // failover chain, not a raw provider
    Instruction: "You are...",
    Tools:   []tool.Tool{tools.NewMyTool()},
    OutputKey: "my_output",
})
mustOK(err, "create my_agent")

// 2. Register in root agent
rootAgent, _ := llmagent.New(llmagent.Config{
    Name:       "root_agent",
    Model:      m,
    SubAgents:  []agent.Agent{myAgent, existingAgents...},
    Instruction: "Route tasks: ...",
})
```

### 3. Build → Run loop

```bash
# Build + run TUI
go build -o /dev/null ./...  # check compilation
go run . adk run              # full TUI

# Or just run tests
go test ./...
```

### 4. Run the TUI

```bash
# Interactive chat
go run . adk run

# Web dev UI
go run . adk web
# → http://localhost:8000

# REST API
go run . adk api_server
# → http://localhost:8080
```

### 5. Environment check

```bash
# Check which provider keys are set
make env
```

Expected output:
```
GOOGLE_API_KEY        set  (✓)
GITHUB_TOKEN          set  (✓)
GROQ_API_KEY         —     (optional)
NVIDIA_API_KEY       —     (optional)
OPENROUTER_API_KEY   —     (optional)
HF_API_TOKEN         —     (optional)
OPENCODE_API_KEY     —     (optional)
```

At least one key must be set for agents to work.

---

## Testing

```bash
# All unit tests
go test ./...

# Specific package
go test ./model/failover/...

# Provider tests (need real keys)
go test -run TestGroq ./model/groq/   # needs GROQ_API_KEY

# TUI tests
go test ./cmd/tui/...

# Lint
make lint         # go vet + formatting checks
```

---

## Adding a New Provider

```bash
# 1. Create package
mkdir -p model/myprovider

# 2. Follow the provider skeleton (see AGENTS.md §4)
#    - Config struct + ConfigFromEnv()
#    - NewModel() returning nil when unconfigured
#    - KnownModels catalog entry

# 3. Register in failover chain (main.go or cmd/tui/main.go)
m, _ := failover.New(ctx, failover.Config{
    Models: []model.LLM{
        myprovider.NewModel(ctx, myprovider.ConfigFromEnv()),
        // ... existing providers
    },
})
```

---

## Adding a New Skill

```bash
# 1. Create SKILL.md
mkdir -p skills/my-category/my-skill

# 2. Write SKILL.md with front matter
cat > skills/my-category/my-skill/SKILL.md << 'EOF'
---
name: my-skill
description: What this skill does in one sentence
license: MIT
---

# My Skill

Instructions for the LLM...
EOF

# 3. That's it — SkillToolset auto-discovers it
```

---

## Committing

```bash
git add -A
git commit -m "feat: add my change

- Description of what changed
- Why it matters
- Breaking changes (if any)"
```

| Prefix | When |
|--------|------|
| `feat:` | New agent, tool, provider, feature |
| `fix:` | Bug fix |
| `docs:` | Documentation |
| `refactor:` | Code change (no feature/fix) |
| `chore:` | Dependencies, tooling, CI |
| `test:` | Adding or fixing tests |

---

## Companion Project: dino-mcp

The repo includes [`dino-mcp/`](dino-mcp/) — a Go MCP server with interactive HTML dashboard.

```bash
cd dino-mcp
make dev-http    # http://localhost:9010/dashboard
```

Full development guide at [`dino-mcp/DEVELOPMENT.md`](dino-mcp/DEVELOPMENT.md).
Documentation indexed in context-mode as `dino-mcp-docs`.

---

## Makefile Reference

```bash
make test                  # All unit tests
make lint                  # go vet + formatting
make env                   # Show which provider keys are set
make run                   # Run the TUI
make console               # ADK built-in text console
make web                   # Browser dev-UI
```

---

## Need Help?

- `make help` — all targets
- `docs/` — Diátaxis documentation
- `AGENTS.md` — AI coding rules and skeletons
- `dino-mcp/DEVELOPMENT.md` — MCP server development

# go-adk-q Documentation

**go-adk-q** is a minimal, idiomatic Go reference implementation built on the
[Google ADK Go SDK](https://pkg.go.dev/google.golang.org/adk) (`v1.2.0`).
It demonstrates every major ADK pattern — agents, tools, skills, memory,
artifacts, multi-provider failover — packaged inside a Bubbletea TUI you can
run today.

---

## Where to start

| I want to… | Go to… |
|---|---|
| Run the TUI in 5 minutes | [tutorials/get-started.md](tutorials/get-started.md) |
| Build my first custom agent | [tutorials/first-agent.md](tutorials/first-agent.md) |
| Add a new LLM provider | [tutorials/add-provider.md](tutorials/add-provider.md) |
| Add a colour theme | [how-to/add-theme.md](how-to/add-theme.md) |
| Add an agent skill | [how-to/add-skill.md](how-to/add-skill.md) |
| Debug a TUI rendering bug | [how-to/debug-tui.md](how-to/debug-tui.md) |
| Look up CLI flags | [reference/cli.md](reference/cli.md) |
| See all config env vars | [reference/config.md](reference/config.md) |
| Browse provider APIs | [reference/providers.md](reference/providers.md) |
| Understand the architecture | [explanation/architecture.md](explanation/architecture.md) |
| Understand TUI rendering | [explanation/tui-rendering.md](explanation/tui-rendering.md) |
| Understand failover design | [explanation/failover.md](explanation/failover.md) |
| Read design decisions | [adr/](adr/) |

---

## Documentation structure

This documentation follows the [Diataxis](https://diataxis.fr/) framework:

```
docs/
├── README.md              ← you are here (navigation index)
├── tutorials/             ← learning-oriented, step-by-step guides
│   ├── get-started.md
│   ├── first-agent.md
│   └── add-provider.md
├── how-to/                ← task-oriented, goal-directed recipes
│   ├── add-theme.md
│   ├── add-skill.md
│   ├── add-provider.md
│   └── debug-tui.md
├── reference/             ← information-oriented, precise and complete
│   ├── cli.md
│   ├── config.md
│   ├── api.md
│   ├── themes.md
│   └── providers.md
├── explanation/           ← understanding-oriented, background and rationale
│   ├── architecture.md
│   ├── tui-rendering.md
│   └── failover.md
└── adr/                   ← Architecture Decision Records
    ├── ADR-0001-adk-as-agent-layer.md
    ├── ADR-0002-genkit-compat-oai.md
    ├── ADR-0003-failover-buffered.md
    ├── ADR-0004-lipgloss-join-vertical.md
    └── ADR-0005-provider-config-pattern.md
```

---

## Personas

**Application developer** — You want to wire ADK agents into your own Go
application. Start with [tutorials/first-agent.md](tutorials/first-agent.md)
then read [explanation/architecture.md](explanation/architecture.md).

**Provider integrator** — You want to add a new LLM backend. Start with
[tutorials/add-provider.md](tutorials/add-provider.md) and use
[reference/providers.md](reference/providers.md) as a checklist.

**TUI contributor** — You want to fix a rendering bug or add a theme. Start
with [explanation/tui-rendering.md](explanation/tui-rendering.md) and
[how-to/debug-tui.md](how-to/debug-tui.md).

**Agent-legible (LLM)** — See [../llms.txt](../llms.txt) for a compact index
and [../llms-full.txt](../llms-full.txt) for full content.

---

## Quick reference

```sh
# Run the interactive TUI
go run ./cmd/tui chat

# One-shot query
go run ./cmd/tui run "What is the capital of France?"

# Build and run
go build -o tui ./cmd/tui && ./tui chat

# Run all tests
go test ./...
```

Required environment variables (at least one provider key):

```sh
export GEMINI_API_KEY=...
export GITHUB_TOKEN=...        # GitHub Models
export GROQ_API_KEY=...
export NVIDIA_API_KEY=...
export OPENROUTER_API_KEY=...
export HUGGINGFACE_API_KEY=...
```

See [reference/config.md](reference/config.md) for the full list.

# AI Agent Memory — go-adk-q

> Zero-config AI onboarding: load the right file for your tool, ask anything.
> No developer explanation required.

---

## Token tier table

| File | Tokens | Use when |
|---|---|---|
| `QUICK_CONTEXT.md` | ~300 | One-off question, quick lint, short task |
| `SYSTEM_PROMPT.md` | ~2 000 | Full chat session (ChatGPT / Claude.ai web) |
| `AGENTS.md` | ~5 000 | Deep feature work, new agent/provider, architecture questions |
| `llms-full.txt` | ~15 000 | RAG indexing, semantic search, batch context |

**Rule**: load the smallest tier that gives the agent enough context for the task.
Scale up only when the smaller tier produces wrong answers.

---

## Tool-by-tool quickstart

### Claude Code (auto-load)

```sh
# AGENTS.md loads automatically when you open the project.
# Verify with:
"What are the hard rules for adding a new FunctionTool in this project?"
# Expected: typed args struct with json + jsonschema tags, panic on error, etc.
```

### Cursor IDE (auto-load)

`.cursorrules` loads automatically from the repo root.

```sh
# Verify Cursor knows the project:
# Open any .go file → ask Cursor:
"Add a FunctionTool that returns the current Bitcoin price."
# Expected output: typed args struct, functiontool.New, correct json+jsonschema tags
```

### GitHub Copilot (auto-load)

`.github/copilot-instructions.md` loads automatically.

```sh
# In any file, type a comment and trigger inline completion:
// New LlmAgent that summarises a document
# Expected: Model: m, Description field, Toolsets: agentToolsets pattern
```

### Aider (auto-load via .aider.conf.yml)

```sh
aider   # AGENTS.md + llms-full.txt pre-loaded automatically
# First prompt:
"Add a new SequentialAgent that rewrites text then checks grammar."
# Aider already knows all patterns — no manual /add needed
```

### ChatGPT / Claude.ai web (manual upload)

**One-off question**:
1. Upload `docs/ai-memory/QUICK_CONTEXT.md`
2. Ask your question

**Full session**:
1. Paste contents of `docs/ai-memory/SYSTEM_PROMPT.md` as your first message
2. Prefix: "Use this as your project context. Follow all rules exactly."

### Gemini (API / AI Studio)

**System instruction** (AI Studio):
- Paste `docs/ai-memory/SYSTEM_PROMPT.md` into the System Instructions field

**API call**:
```python
system_instruction = open("docs/ai-memory/SYSTEM_PROMPT.md").read()
# Pass as system_instruction in GenerateContentConfig
```

### Semantic search / RAG pipeline

Index `llms-full.txt` into your vector store. It contains:
- Project identity + hard rules
- All code skeletons
- Full route/naming conventions
- Pitfalls catalogue
- ADR summaries
- Provider reference

---

## Memory architecture diagram

```
┌─────────────────────────────────────────────────────────────┐
│                    Developer's repo                         │
│                                                             │
│  ┌──────────────┐  ┌─────────────────┐  ┌───────────────┐  │
│  │  QUICK_      │  │  SYSTEM_        │  │  AGENTS.md    │  │
│  │  CONTEXT.md  │  │  PROMPT.md      │  │  ~5 000 tok   │  │
│  │  ~300 tok    │  │  ~2 000 tok     │  │               │  │
│  └──────┬───────┘  └────────┬────────┘  └───────┬───────┘  │
│         │                   │                   │           │
│         ▼                   ▼                   ▼           │
│    One-off query       Web chat UI         IDE agents       │
│    ChatGPT/Claude      (full session)      Claude Code      │
│                                            Cursor           │
│                                            Copilot          │
│                                            Aider            │
│                                                             │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  llms-full.txt  ~15 000 tokens                       │   │
│  │  RAG / vector store / Aider full context             │   │
│  └──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

---

## Keeping memory in sync

The memory files are generated from actual code. When you make structural changes, update them:

| Change | Update these files |
|---|---|
| New agent pattern | `AGENTS.md §3`, `SYSTEM_PROMPT.md`, `llms-full.txt`, `QUICK_CONTEXT.md` (if skeleton changes) |
| New provider | `AGENTS.md §4`, `SYSTEM_PROMPT.md`, `llms-full.txt`, `docs/reference/providers.md` |
| New hard rule | `AGENTS.md §3-4`, `SYSTEM_PROMPT.md`, `QUICK_CONTEXT.md`, `.cursorrules`, `.github/copilot-instructions.md` |
| New pitfall discovered | `docs/explanation/pitfalls.md`, `SYSTEM_PROMPT.md` §Known pitfalls, `llms-full.txt` |
| New env var | `docs/reference/config.md`, `AGENTS.md §10`, `Makefile` env section |
| New ADR | `docs/adr/ADR-NNNN.md`, `AGENTS.md §13`, `llms.txt`, `llms-full.txt` |

**Sync command pattern** (run after structural changes):
```sh
# Regenerate llms-full.txt by concatenating authoritative sources
# (adapt to your toolchain; this is a reference pattern)
cat docs/ai-memory/SYSTEM_PROMPT.md \
    docs/reference/config.md \
    docs/reference/providers.md \
    docs/explanation/pitfalls.md \
    docs/explanation/architecture.md \
    > llms-full.txt
```

---

## Verify the AI knows your project

Ask any AI agent loaded with this context:

1. "What database does this project use?" → Expected: *no database; all state is in-memory session state*
2. "What model should I use in a new agent?" → Expected: *`m` — the failover chain; never a raw provider*
3. "What tags are required on a FunctionTool args struct?" → Expected: *`json:` and `jsonschema:` both required*
4. "Can I import firebase/genkit in a new model package?" → Expected: *no — only `model/oaibridge` may import genkit*
5. "What happens if I skip ExitLoopTool in a LoopAgent?" → Expected: *loop runs MaxIterations unconditionally*

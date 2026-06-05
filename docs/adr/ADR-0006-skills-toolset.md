# ADR-0006: Use SKILL.md Files + SkillToolset for Agent Skills

**Status**: Accepted
**Date**: 2025-06-01
**Deciders**: go-adk-q maintainers

---

## Context

The root agent needs a way to extend its capabilities at runtime without
recompilation. Specifically:

1. Domain-specific reasoning patterns (code review style, documentation
   standards, architectural principles) vary by use-case.
2. Users and teams want to contribute new capabilities without touching Go
   source code.
3. Skills should be composable — a single agent session may need to load
   multiple skills on demand.
4. Skills need to work across model providers (not just Gemini), including
   OpenAI-compatible providers.

Several approaches were considered for delivering these extended system
instructions to the LLM at runtime.

---

## Decision

Use plain Markdown files (`SKILL.md`) stored in a directory tree under
`skills/` as the skill delivery format. Discover and serve them via the
`google.golang.org/adk/tool/skilltoolset` package (wrapped locally to fix
schema issues — see ADR-0007).

Skills are exposed to the agent as two FunctionTools:
- `list_skills` — returns an XML list of available skills with name + description
- `load_skill {"name": "..."}` — returns the full SKILL.md instructions for a skill

The LLM uses `list_skills` to discover what is available, then calls
`load_skill` to inject the skill's system instructions into its context.

---

## Alternatives Considered

### Hard-coded instructions in agent Instruction field

Rejected because:
- System prompt grows unboundedly as skills accumulate.
- All instructions loaded unconditionally, consuming tokens even when not needed.
- Adding a skill requires code changes and recompilation.
- No way to give users/teams control over their own skills.

### Database-backed skill registry

Rejected because:
- Adds an external dependency (database) for what is essentially a text file store.
- Over-engineered for the expected skill count (tens, not thousands).
- Complicates local development and deployment.
- No benefit over filesystem for sequential read access.

### Embedded Go files (go:embed)

Rejected because:
- Skills cannot be added or modified without recompilation.
- Goes against the "zero-code skill authoring" goal.
- Makes the repo harder to navigate for non-Go contributors.

### Plugin system (Go plugins or Lua/Wasm)

Rejected because:
- Go plugin (.so) files are platform-specific, fragile with CGO, and add
  significant operational complexity.
- Scripting runtimes (Lua, Wasm) add dependencies and a new language to learn.
- SKILL.md achieves the same extensibility goal with plain text.

---

## Consequences

### Positive

- **Zero-code skill authoring**: adding a skill is creating a directory with a
  `SKILL.md` file. No Go knowledge required.
- **Dynamic discovery**: `SkillToolset` reads the filesystem at startup; new
  skills appear without restarting once the process is restarted.
- **Token efficiency**: skills are loaded on demand via `load_skill`, not
  injected into every prompt unconditionally.
- **Portable format**: SKILL.md is plain Markdown — readable by humans,
  indexable by AI agents, versionable in git.
- **Works across providers**: skill loading is implemented as standard
  FunctionTools that work with any model that supports function calling.

### Negative / Trade-offs

- Skills are loaded per-conversation turn, not persisted across the session
  automatically. The LLM must call `load_skill` again if context is lost.
- The `list_skills` → `load_skill` two-step adds one round-trip to skill
  invocation latency.
- Skill discovery requires the binary to have filesystem access to `./skills`.
  Containerised deployments must mount or embed the skills directory.

---

## Implementation Notes

```
skills/
├── agents/
│   └── analyzer/SKILL.md
├── engineering/
│   └── go-expert/SKILL.md    ← loaded when user asks for Go expertise
└── documentation/
    └── doc-drafting/SKILL.md

# SKILL.md frontmatter format:
---
name: go-expert
description: Expert Go code review and advice following idiomatic Go style.
license: MIT
compatibility: google.golang.org/adk v1+
---
```

The local `tool/skilltoolset` package wraps the upstream ADK implementation
to fix OpenAI-compat schema validation issues (see ADR-0007).

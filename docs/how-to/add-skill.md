# How-to: Add an agent skill

**Goal:** Create a new skill SKILL.md file that the `/skills` command surfaces
in the TUI, and optionally wire it to an ADK agent or toolset.

---

## What is a skill?

In this repo, a **skill** is a Markdown specification file (`SKILL.md`) that
defines a reusable agent behaviour: its purpose, trigger conditions, inputs,
outputs, and step-by-step process.

Skills live in `skills/` and are organised by domain:

```
skills/
├── agents/
│   ├── analyzer/SKILL.md
│   ├── comparator/SKILL.md
│   └── grader/SKILL.md
├── collaboration/
│   ├── code-review-consensus/SKILL.md
│   └── ...
├── design/
├── documentation/
├── engineering/
└── workflow/
```

The ADK `SkillToolset` (`tool/skilltoolset/`) loads these files and exposes
them as tools that the main agent can invoke.

---

## Step 1 — Create the skill directory and file

```sh
mkdir -p skills/engineering/my-skill
```

Create `skills/engineering/my-skill/SKILL.md`:

```markdown
# my-skill

## Purpose

One sentence: what this skill does and when to use it.

## Trigger conditions

- User asks to do X
- Agent output contains Y pattern
- After completing step Z

## Inputs

| Field | Type | Description |
|---|---|---|
| `context` | string | Background information |
| `target` | string | The thing to act on |

## Process

1. **Analyse** the input for X, Y, Z.
2. **Draft** a response following the template below.
3. **Review** against the checklist.
4. **Output** the final result.

## Output format

\`\`\`
## Summary
...

## Details
...
\`\`\`

## Checklist

- [ ] Addressed all inputs
- [ ] Output matches format
- [ ] No hallucinated facts
```

---

## Step 2 — Register the skill (optional)

If you want the skill to be callable as an ADK tool, add it to the
`localskilltoolset` initialisation in `cmd/tui/main.go`:

```go
import localskilltoolset "go-adk-q/tool/skilltoolset"

// Inside buildRunner():
skillset, err := localskilltoolset.New(localskilltoolset.Config{
    SkillsDir: "skills",
})
```

The `SkillToolset` discovers all `SKILL.md` files under `SkillsDir`
automatically — no explicit registration per skill is needed.

---

## Step 3 — Verify in the TUI

```sh
go run ./cmd/tui chat
```

Type `/skills` — your new skill should appear in the list.

---

## Tips

**Keep SKILL.md agent-legible.** LLMs read these files. Use clear headings,
short sentences, and explicit enumerated steps.

**Checklist = done criteria.** The checklist at the end is evaluated by the
agent to determine whether the skill completed successfully.

**Benchmark mode.** The Analyzer skill supports a benchmark mode where it
compares outputs across two runs. If your skill produces structured output,
consider adding a similar mode.

---

## Related

- [Reference: Skills API](../reference/api.md#skills)
- [Explanation: Architecture — skill toolset](../explanation/architecture.md#skill-toolset)

---
name: doc-drafting
description: Draft a new document from scratch. Covers audience identification, structure, tone, and first-draft principles for go-adk-q documentation.
compatibility: Designed for go-adk-q — a Google ADK Go reference implementation with Bubbletea TUI.
---
# Doc Drafting

Use when creating a new document: README section, guide, reference page, or architecture note.

## Before writing

Answer three questions:
1. **Who reads this?** (New contributor? Experienced user? Future maintainer? The LLM agent?)
2. **What do they need to do after reading it?** (Set up the project? Understand a design decision? Use a skill?)
3. **What's the minimum they need to know?**

If you can't answer these, you're not ready to draft. Talk to the intended reader first.

## Document types in go-adk-q

| Type | Audience | Location | Purpose |
|------|----------|----------|---------|
| README | New user | `README.md` | Get started in < 5 minutes |
| Testing guide | Developer | `docs/TESTING.md` | Verify the app works |
| Skill | Agent / developer | `skills/<category>/<name>/SKILL.md` | Guide an LLM through a workflow |
| Architecture note | Maintainer | `docs/` | Explain a non-obvious design decision |
| Retro | Team | `docs/RETRO.md` | Record what happened |

## Structure for each type

### Skill document
```
---
name: <skill-name>
description: <one sentence, max 120 chars>
---
# Title

One sentence: what this skill does and when to use it.

## When to use
## Core process / checklist
## Examples (concrete, runnable)
## What not to do
```

### Architecture note
```
# Title — Decision Record

Date: YYYY-MM-DD
Status: Active / Superseded by X

## Context
What problem were we solving?

## Decision
What did we choose?

## Consequences
What does this mean going forward? What trade-offs did we accept?

## Alternatives considered
What else did we think about and why did we reject it?
```

### README section
```
## Section title

One sentence: what this section covers.

[Setup/usage steps — numbered, concrete, runnable]

[Code example if applicable]

[Link to more detail if needed]
```

## Drafting principles

**Show, don't tell.** Replace "the agent handles tool calls efficiently" with "the agent batches up to 5 tool calls per turn."

**Runnable examples.** Every code example must compile and run. Untested examples rot.

**One idea per paragraph.** If a paragraph covers two ideas, split it.

**Active voice.** "The model sends a reply" not "A reply is sent by the model."

**Short sentences.** If a sentence needs a semicolon, split it into two sentences.

## First draft checklist

- [ ] Does the title tell you exactly what the document covers?
- [ ] Is the audience stated or obvious from the first sentence?
- [ ] Is there at least one concrete example?
- [ ] Are all code examples runnable?
- [ ] Is there a clear ending? (Next steps, related docs, or "that's everything")

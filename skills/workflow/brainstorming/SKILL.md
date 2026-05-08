---
name: brainstorming
description: Structured ideation before building. Explore the problem space, surface constraints, generate options, and pick a direction before writing any code.
compatibility: Designed for go-adk-q — a Google ADK Go reference implementation with Bubbletea TUI.
---
# Brainstorming

Use this skill before starting any feature you haven't fully thought through. 20 minutes of structured thinking prevents days of rework.

## The rule

Do not open an editor until you can answer: "What am I building, for whom, and how will I know it works?"

## Process

### 1. Restate the problem (5 min)
Write the problem in your own words. If you can't, you don't understand it yet.

Ask: Who is the user of this feature? What are they trying to do? What is stopping them right now?

For go-adk-q, the "user" is usually:
- A developer running the TUI who wants to interact with LLMs from the terminal
- A developer extending the codebase (adding providers, skills, tools)
- An agent runtime that needs tools and memory

### 2. Generate options (10 min)
Produce at least 3 approaches. Don't evaluate yet — just generate.

Example problem: "Users can't copy text from the terminal."

Options:
```
A. Remove mouse mode so terminal handles selection natively (Shift+drag still works)
B. Keep mouse mode; add ctrl+y to copy last reply; ctrl+shift+y for code block
C. Add a per-message copy button rendered as a key binding hint next to each message
D. Pipe output to a separate pane that has no mouse mode
```

### 3. Evaluate (5 min)
For each option, list: pros, cons, implementation complexity, reversibility.

Use a table:

| Option | Pro | Con | Complexity | Reversible? |
|--------|-----|-----|------------|-------------|
| A | native UX | lose scroll | low | yes |
| B | both work | two shortcuts | medium | yes |
| C | discoverable | visual noise | high | yes |
| D | clean separation | new component | high | hard |

### 4. Pick and commit
Choose one option. Write one sentence explaining why.

Write down what you're NOT doing and why. This is as important as what you are doing — it prevents scope creep during implementation.

### 5. Identify unknowns
Before you start: what do you need to verify or learn first?

Example: "Does `tea.WithMouseCellMotion` actually block Shift+drag, or is that terminal-specific?"

Resolve unknowns before planning steps. A 5-minute spike (write throwaway code, read the source, try it) is always worth it.

## For go-adk-q specifically

Common design tensions to think through:

**TUI vs agent:**
- Does this belong in the TUI layer or should it be an agent tool?
- Rule: if a human needs to see it or interact with it, it's TUI. If an LLM needs to call it, it's a tool.

**Provider-specific vs universal:**
- Does this feature require a specific API (e.g., streaming, function calling)?
- If yes: gate it or degrade gracefully for providers that don't support it.

**Stateful vs stateless:**
- Does this need to persist across sessions (file), within a session (chatModel field), or just per-message (local variable)?
- Prefer the smallest scope that works.

## Output
Write your brainstorm output as a comment at the top of the relevant file, or in a `docs/` note, before implementing. It doesn't need to be long — 10-20 lines is enough to capture the decision.

---
name: research
description: Structured research workflow. Define the question, search efficiently, evaluate sources, synthesise findings, and produce an actionable output.
compatibility: Designed for go-adk-q — a Google ADK Go reference implementation with Bubbletea TUI.
---
# Research

Use before making a significant technical decision, adopting a dependency, or designing an architecture you haven't worked with before.

## The research question

Write the question you're trying to answer before starting. A vague question produces vague research.

Bad: "Research glamour"
Good: "What glamour version is compatible with lipgloss v1.1.0, and does it support the tokyo-night style name?"

The question determines what counts as an answer and when you can stop.

## Research process

### 1. Check what's already known
Before searching externally, check:
- This codebase's `docs/` directory
- Existing skill files in `skills/` (especially `engineering/go-expert`)
- `go.mod` for current dependency versions
- Comments in the relevant source files

You may already have the answer.

### 2. Search precisely
Use specific version numbers, package paths, and function names in your searches.

For Go ecosystem questions:
- `pkg.go.dev/<package>@<version>` — canonical API reference
- `github.com/<owner>/<repo>/releases` — release notes and breaking changes
- `github.com/<owner>/<repo>/blob/main/CHANGELOG.md`

For TUI/terminal questions:
- Bubbletea docs: `github.com/charmbracelet/bubbletea`
- Glamour styles: `github.com/charmbracelet/glamour/styles`
- Lipgloss API: `pkg.go.dev/github.com/charmbracelet/lipgloss`

### 3. Verify claims with code
Don't trust documentation alone for anything that will affect a build or test.

Write a minimal reproducer:
```go
// scratch/research_test.go
package scratch

import (
    "testing"
    "github.com/charmbracelet/glamour"
)

func TestStyleExists(t *testing.T) {
    _, err := glamour.NewTermRenderer(glamour.WithStandardStyle("tokyo-night"))
    if err != nil {
        t.Fatal(err)
    }
}
```

Run it. The answer is in the output, not in the docs.

### 4. Record findings

Write findings as assertions with evidence:

```
FINDING: glamour v0.8.1 supports "tokyo-night" as a built-in style name.
EVIDENCE: pkg.go.dev/github.com/charmbracelet/glamour@v0.8.1/styles shows TokyoNightStyleFunc
VERIFIED: go test ./scratch/ passes with WithStandardStyle("tokyo-night")
```

### 5. Make a decision

Research ends with a decision, not a list of options.

"We will use glamour v0.8.1 with the following style mapping: Catppuccin→dracula, Tokyo Night→tokyo-night, Rosé Pine→dracula, Nord→dark, Gruvbox→dark."

## What makes research good vs bad

| Good | Bad |
|------|-----|
| Specific question | Vague topic |
| Verified with code | Only read docs |
| Ends with a decision | Ends with "more options" |
| Time-boxed (30 min) | Open-ended |
| Recorded in a doc or comment | Lives only in your head |

## Time limit

Set a 30-minute limit. If you haven't found an answer in 30 minutes, you're either asking the wrong question or the information isn't publicly available. Pivot: make a decision with what you have, or ask someone who knows.

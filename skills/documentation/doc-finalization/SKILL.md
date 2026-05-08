---
name: doc-finalization
description: Finalize and publish a document. Covers final accuracy check, formatting, merge, and post-publish steps.
compatibility: Designed for go-adk-q — a Google ADK Go reference implementation with Bubbletea TUI.
---
# Doc Finalization

Use when a document has passed review and is ready to publish (merge to main).

## Pre-publish checklist

### Accuracy (must verify, not assume)
```bash
# All file paths mentioned in the doc exist
ls <each path mentioned>

# All commands mentioned produce the described output  
<each command> 2>&1

# All function/type names mentioned exist in current code
grep -rn "<name>" . | head -5

# All version numbers match go.mod
grep "<package>" go.mod
```

### Formatting
- [ ] Front matter present and correct for skill files (`name:`, `description:`)
- [ ] Headings use consistent levels (H2 for top sections, H3 for subsections)
- [ ] Code blocks have language tags: ` ```bash `, ` ```go `, ` ```markdown `
- [ ] No trailing whitespace on lines
- [ ] File ends with a single newline

### Links
- [ ] Any relative links point to files that exist: `[see foo](../foo/bar.md)`
- [ ] No broken anchor links (`#section-name` matches an actual heading)

### Skill-specific checks
- [ ] `name:` in front matter matches the directory name
- [ ] `description:` is one sentence, under 120 characters
- [ ] The skill can be executed by an LLM without additional context
- [ ] No references to external files that won't exist in all environments

## Merge

```bash
git add skills/<category>/<name>/SKILL.md  # or docs/<name>.md
git commit -m "docs: add <name> skill to <category>/"
git push
```

For multiple new documents:
```bash
git commit -m "docs: add 12 new skills across engineering, workflow, collaboration, design, documentation categories"
```

## Post-publish steps

### Update /skills output
After adding a new skill, verify the TUI shows it:
1. Run `make run`
2. Type `/skills` + Enter
3. Confirm the new skill appears with correct name and description

### Update the skills index (if one exists)
If `docs/SKILLS.md` or `README.md` has a skills section, add the new entry.

### Notify if relevant
If the skill was written to help a specific team member or addresses a specific workflow gap, tell them it exists.

## What "done" looks like

- Document is in the correct category directory
- All factual claims verified
- `/skills` in the TUI shows the new skill
- `go build ./...` still passes (docs don't affect builds, but good habit)
- Any related skill cross-references are updated

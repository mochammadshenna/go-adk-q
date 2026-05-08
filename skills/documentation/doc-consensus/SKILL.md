---
name: doc-consensus
description: Reach agreement on contested documentation decisions. Covers scope, structure, and content disputes.
compatibility: Designed for go-adk-q — a Google ADK Go reference implementation with Bubbletea TUI.
---
# Doc Consensus

Use when two people disagree about how a document should be written, structured, or scoped.

## Types of doc disagreements

### Scope disagreement
"This doc should cover X" vs "X belongs in a different doc"

Resolve by asking: who reads this document, and what do they need to do after? If X helps that person do that thing, it belongs here. If X is for a different reader or a different task, it belongs elsewhere.

For go-adk-q, the principle is: **one document per task**. A user setting up providers should not have to read the architecture notes. A maintainer debugging the renderer cache should not wade through the getting-started guide.

### Structure disagreement
"The examples should come first" vs "The concepts should come first"

The rule: **task-oriented documents (how-to guides, skills) lead with the task**. Reference documents (API, architecture) can lead with concepts.

For skills specifically: the skill should tell the LLM what to do, in the order it should do it. Don't front-load background — the LLM doesn't need to understand the history; it needs to know the next action.

### Content disagreement
"This is the correct explanation" vs "No it isn't"

This is a factual dispute. Resolve it the same way as a code review factual dispute: verify with the source (code, tests, official docs). Write a test if needed.

Example: "Does glamour v0.8.1 support the 'tokyo-night' style name?" — check `pkg.go.dev`, write a test, close the dispute.

### Tone disagreement
"This should be more formal" vs "This should be more direct"

The project tone for go-adk-q docs:
- Direct and imperative for skills: "Run `go build ./...`. Expect zero output."
- Conversational for guides: "Before you can test the providers, you'll need at least one API key."
- Technical and precise for architecture notes.

Formal/academic tone is not the goal. Clarity and accuracy are.

## Making the decision

1. State both positions in one sentence each
2. Apply the principle (task-oriented, one document per task, verify facts)
3. Pick one
4. Document the decision if it's not obvious from the principles above

Don't iterate more than twice on the same disagreement. If you can't resolve it in two rounds, pick the option that's easier to change later.

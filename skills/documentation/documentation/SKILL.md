---
name: documentation
description: Write and maintain technical documentation. Use when writing a README, API docs, architecture docs, runbooks, onboarding guides, or any form of technical writing for developers or operators.
compatibility: Designed for software engineering and technical writing workflows.
---
# Technical Documentation

Write clear, maintainable technical documentation for different audiences and purposes.

## Document types

### README
- What this is and why it exists (one paragraph)
- Quick start: first success in < 5 minutes
- Configuration and usage
- Contributing guide

### API documentation
- Endpoint reference with request/response examples
- Authentication method and required credentials
- Error codes and their meanings
- Rate limits and pagination

### Architecture document
- What problem does this system solve?
- How are the major components connected? (diagram)
- Key decisions and why they were made
- What's out of scope

### Runbook
- When to use this runbook (what triggers it)
- Prerequisites (access, tools)
- Step-by-step procedure (numbered, specific commands)
- Expected outcomes and how to verify
- What to do if something goes wrong

### Onboarding guide
- What the new person needs to understand on day 1
- What they can skip until later
- First task: something small and completable
- Who to ask for what

## Writing principles

**Show, don't tell.**
Bad: "The system handles errors gracefully."
Good: "If the API key is invalid, the response is HTTP 401 with body `{"error": "invalid_api_key"}`."

**Runnable examples.**
Every code example must compile and run. Test them. Untested examples rot immediately.

**One idea per paragraph.**
If a paragraph covers two ideas, split it.

**Active voice.**
"The agent sends a reply" — not "A reply is sent by the agent."

**Audience-specific depth.**
New user: no assumed knowledge, show expected output.
Developer: can assume language knowledge, can reference file:line.
Operator: needs exact commands, exact expected output.

## Maintenance rules

- Update docs in the same PR as the code change
- Run code examples in CI if possible
- Delete documentation for deleted features — stale docs are worse than no docs
- Put the date on runbooks and architecture docs

## For go-adk-q

Key docs:
- `README.md` — project overview and quick start
- `docs/TESTING.md` — how to run and write tests
- Skill files: `skills/<category>/<name>/SKILL.md` — agent skill instructions

When updating the TUI, check if `docs/TESTING.md` needs updating (manual test steps may change).

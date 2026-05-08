---
name: architecture
description: Create or evaluate an architecture decision record (ADR). Use when choosing between technologies, documenting a design decision with trade-offs and consequences, reviewing a system design proposal, or designing a new component from requirements and constraints.
compatibility: Designed for software engineering workflows. References CONNECTORS.md for optional integrations.
---
# Architecture Decision Records

Create an Architecture Decision Record (ADR) or evaluate a system design.

## When to use

- Choosing between technologies (e.g., Kafka vs SQS)
- Documenting a design decision with trade-offs
- Reviewing a system design proposal
- Designing a new component from requirements and constraints

## ADR Output Format

```markdown
# ADR-[number]: [Title]

**Status:** Proposed | Accepted | Deprecated | Superseded
**Date:** [Date]
**Deciders:** [Who needs to sign off]

## Context
[What is the situation? What forces are at play?]

## Decision
[What is the change we're proposing?]

## Options Considered

### Option A: [Name]
| Dimension | Assessment |
|-----------|------------|
| Complexity | Low/Med/High |
| Cost | Assessment |
| Scalability | Assessment |
| Team familiarity | Assessment |

**Pros:** [List]
**Cons:** [List]

### Option B: [Name]
[Same format]

## Trade-off Analysis
[Key trade-offs with clear reasoning]

## Consequences
- [What becomes easier]
- [What becomes harder]
- [What we'll need to revisit]

## Action Items
1. [ ] [Implementation step]
2. [ ] [Follow-up]
```

## Process

1. **State constraints upfront** — "We need to ship in 2 weeks" or "Must handle 10K rps" shapes the analysis
2. **Name all options explicitly** — Even if you're leaning one way, explicit alternatives produce better analysis
3. **Include non-functional requirements** — Latency, cost, team expertise, and maintenance burden matter as much as features
4. **Make trade-offs explicit** — Every decision trades something; write it down

## Evaluation Dimensions

For any technology or approach choice, evaluate:
- Correctness fit (does it solve the actual problem?)
- Operational complexity (who runs it and how?)
- Cost (infrastructure, licensing, engineering time)
- Team familiarity (existing skill vs. learning curve)
- Ecosystem maturity (community, docs, known failure modes)
- Exit path (how hard is it to replace if it fails?)

## Tips

- See the **system-design** skill for detailed frameworks on requirements gathering and scalability
- Record rejected options explicitly — future engineers need to know why alternatives were ruled out
- Revisit ADRs when constraints change; a deprecated ADR is still valuable history

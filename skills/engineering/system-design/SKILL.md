---
name: system-design
description: Design systems, services, and architectures. Use when you need to design a system from scratch, evaluate architectural trade-offs, design API contracts, plan data models, or define service boundaries for a new or evolving system.
compatibility: Designed for software engineering and architecture workflows.
---
# System Design

Design systems and evaluate architectural decisions with a structured framework.

## Framework

### 1. Requirements gathering

Before drawing any boxes:

**Functional requirements** (what the system does):
- What are the core user-facing operations?
- What are the boundaries — what is NOT in scope?

**Non-functional requirements** (how well it does it):
- Scale: requests/sec, users, data volume
- Latency: p50/p99 targets
- Availability: what's the acceptable downtime?
- Consistency: strong vs. eventual
- Cost constraints

**Constraints** (what you must work within):
- Team size and expertise
- Timeline
- Existing tech stack and integrations
- Regulatory or compliance requirements

### 2. High-level design

Start simple. One box for each major system boundary.

- Component diagram: what are the major pieces?
- Data flow: how does data move between them?
- API contracts: what does each component expose?
- Storage choices: what data goes where and why?

### 3. Deep dive

For each critical component:
- Data model: entities, relationships, indexes
- API design: endpoints, request/response shapes, versioning
- Caching strategy: what to cache, TTL, invalidation
- Queue/event design: topics, consumers, ordering guarantees
- Error handling and retry logic: idempotency, dead letter queues

### 4. Scale and reliability

- **Load estimation**: back-of-envelope calculation for peak traffic
- **Horizontal vs. vertical scaling**: when to scale out vs. up
- **Failover**: what happens when each component fails?
- **Redundancy**: which components need replicas?
- **Monitoring**: what signals indicate the system is healthy?

### 5. Trade-off analysis

Every design decision trades something. Make trade-offs explicit:

| Decision | Trade-off | Why we chose it |
|----------|-----------|----------------|
| SQL vs NoSQL | Flexibility vs. consistency | |
| Sync vs async | Simplicity vs. throughput | |
| Monolith vs microservices | Deployment complexity vs. team autonomy | |

## Output format

```markdown
## System Design: [Name]

### Requirements
**Functional**: [list]
**Non-functional**: [scale/latency/availability targets]
**Out of scope**: [explicit exclusions]

### Architecture
[ASCII diagram or component description]

### Data model
[Key entities and relationships]

### API design
[Key endpoints or contracts]

### Trade-offs
[Table of key decisions and reasoning]

### What I'd revisit at 10x scale
[The assumptions that break first]
```

## What makes a design good

- Solves the stated problem without overengineering
- Explicit about what it trades (consistency for availability, latency for throughput)
- Identifies the weakest point and acknowledges it
- Has a clear path from current state to the proposed state

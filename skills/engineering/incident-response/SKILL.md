---
name: incident-response
description: Structured incident response workflow. Use when production is down, a critical bug has been deployed, a security issue has been discovered, or when coordinating a team response to an ongoing incident.
compatibility: Designed for software engineering and operations workflows.
---
# Incident Response

Coordinated response to production incidents. Fast triage, clear communication, minimal collateral damage.

## Severity levels

| Severity | Definition | Response time |
|----------|------------|---------------|
| **P0** | Production down, data loss, security breach | Immediate |
| **P1** | Major feature broken, significant user impact | < 30 min |
| **P2** | Partial degradation, workaround available | < 2 hours |
| **P3** | Minor issue, low user impact | Next business day |

## Response phases

### 1. Detect and declare (first 5 minutes)

- Confirm the incident is real (not a monitoring false positive)
- Declare: "Incident declared — P[N] — [brief description]"
- Assign an Incident Commander (IC) — one person owns the response
- Open a war room (Slack channel, call, etc.)
- Start a timeline document

### 2. Triage (first 15 minutes)

Determine:
- **Scope**: how many users are affected? What percentage of traffic?
- **Blast radius**: what systems are impacted beyond the primary service?
- **Cause hypothesis**: what changed recently? Deployments, config changes, traffic spikes?

Check in order:
```
1. Recent deployments (last 2 hours)
2. Config/feature flag changes
3. Dependency status (databases, external APIs)
4. Infrastructure metrics (CPU, memory, disk, network)
5. Error rates and logs
```

### 3. Mitigate (as fast as possible)

Mitigation goal: stop user harm NOW. Root cause comes later.

Options in priority order:
1. **Rollback** — revert the last deployment if causally linked
2. **Feature flag off** — disable the affected feature
3. **Traffic shift** — route away from affected instances
4. **Scale up** — if overload-caused, add capacity
5. **Manual intervention** — last resort, highest risk

Document every action taken with timestamp: `HH:MM — [action] — [who]`

### 4. Communicate

During the incident:
- Update stakeholders every 15-30 minutes
- "We are investigating [symptoms]. Impact: [scope]. Next update in [time]."
- Don't speculate publicly on root cause until confirmed

Template:
```
[TIME] INCIDENT UPDATE — [service]
Status: INVESTIGATING / MITIGATING / RESOLVED
Impact: [description of user impact]
Timeline: [what happened and when]
Next steps: [what we're doing now]
Next update: [time]
```

### 5. Resolve and verify

- Confirm metrics returned to baseline (not just "looks better")
- Confirm error rates dropped to pre-incident levels
- Confirm affected users can complete their workflows
- Declare resolution: "Incident resolved at [TIME]"

### 6. Post-incident review (within 48 hours)

Write a post-mortem with:
- **Timeline**: what happened, when, in chronological order
- **Root cause**: the actual technical cause (not "human error")
- **Contributing factors**: what made this worse or harder to detect
- **Impact**: scope and duration
- **What went well**: detection, response, communication
- **Action items**: concrete changes with owners and deadlines

Post-mortem rules:
- Blameless. Systems fail, not people.
- Focus on systemic fixes, not individual blame
- Action items must be specific, assigned, and have due dates
- Follow up in 30 days to verify action items completed

## Communication templates

### Stakeholder update
```
INCIDENT [P0/P1/P2] — [Service] — [Status]
Reported: [TIME]
Impact: [user-facing description]
Status: [what we know, what we're doing]
Next update: [TIME]
IC: [name]
```

### Resolution notice
```
RESOLVED — [Service] — [TIME]
Duration: [X hours Y minutes]
Impact: [what was affected]
Cause: [one sentence]
Post-mortem: [link or "scheduled for DATE"]
```

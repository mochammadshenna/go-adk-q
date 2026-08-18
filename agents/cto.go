package agents

import (
	"context"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model"
)

// CTOInstruction is the system prompt for cto_agent: a fully autonomous
// persona standing in for the project's own CTO on final engineering calls.
// Unlike advisor_agent (a holistic second opinion, no verdict) or judge_agent
// (rubric-scored, but not organization-specific), cto_agent decides — it
// does not escalate further, and it does not ask clarifying questions of its
// own; that posture belongs to the human this agent stands in for, not to
// this agent when called mid-turn by manager_agent.
//
// Seeded from this project's own actual standing engineering preferences
// (this repo's org-level instructions) rather than generic "senior engineer"
// platitudes, so a verdict here reflects a real, specific bar:
//   - Prefer Go, PostgreSQL, and metric units by default.
//   - Prefer high-quality, maintainable, long-term solutions over quick
//     fixes — but do not gold-plate beyond what was actually asked.
//   - PostgreSQL tables partitioned by a tenant/entity key require explicit
//     filtering on that key in every query, or the query silently scans
//     other tenants' partitions.
//   - Dependencies and frameworks should be checked against their current
//     stable release, not assumed current from memory.
//   - A risky, hard-to-reverse action needs a stated undo path before it
//     proceeds.
const CTOInstruction = `You are the CTO agent: the project's final engineering authority, standing ` +
	`in for the actual CTO on decisions manager_agent escalates to you. You are given the proposal, ` +
	`change, or decision under review in the incoming message — inspect it directly, do not ask the ` +
	`caller clarifying questions (that posture belongs to the human you stand in for, not to you here).` + "\n\n" +
	`Judge it against this project's actual standing engineering bar:` + "\n" +
	`- Go, PostgreSQL, and metric units are the defaults unless the codebase already uses something else.` + "\n" +
	`- Prefer high-quality, maintainable, long-term solutions over quick fixes — but reject scope creep ` +
	`and gold-plating beyond what was actually asked; both directions are failures.` + "\n" +
	`- Any PostgreSQL query against a table partitioned by a tenant/entity key MUST filter on that key ` +
	`explicitly, or it silently scans other tenants' partitions.` + "\n" +
	`- Dependencies/frameworks should be current-stable, not assumed current from training data.` + "\n" +
	`- A risky, hard-to-reverse action needs a stated undo path before it proceeds.` + "\n\n" +
	`Respond in this exact format:` + "\n" +
	`DECISION: APPROVED or REJECTED` + "\n" +
	`REASONS: up to 3 short, specific, actionable bullet points.` + "\n" +
	`Do not soften a REJECTED verdict with praise — be direct. Do not hedge with "it depends" — decide.`

// GetCTOAgent constructs cto_agent: the fully-autonomous final-call agent
// manager_agent consults before finalizing anything high-stakes or
// ambiguous. No human-in-the-loop pause — it renders DECISION: APPROVED or
// REJECTED directly, per this project's explicit choice (fully autonomous
// persona, not an escalation-to-human gate).
func GetCTOAgent(_ context.Context, m model.LLM) agent.Agent {
	cto, err := llmagent.New(llmagent.Config{
		Name:        "cto_agent",
		Model:       m,
		Description: "The project's final engineering authority. Judges a proposal/change/decision against this project's actual standing engineering bar and returns APPROVED/REJECTED with reasons. Fully autonomous — no human confirmation step. Use for high-stakes or ambiguous final calls.",
		Instruction: CTOInstruction,
		OutputKey:   "cto_decision",
	})
	if err != nil {
		panic("cto_agent: " + err.Error())
	}
	return cto
}

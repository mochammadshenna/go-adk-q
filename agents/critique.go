package agents

import (
	"context"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model"
)

// CritiqueInstruction is the system prompt for critique_agent.
const CritiqueInstruction = `You are an adversarial critic. You are given a claim, plan, or piece of ` +
	`code in {critique_input}. Your job is to actively try to refute or break it: find the ` +
	`concrete input, edge case, or failure scenario where it goes wrong. Default to skeptical — ` +
	`if you cannot find a real flaw after genuinely trying, say so plainly, but do not manufacture ` +
	`a nitpick to appear thorough. Respond with either a specific failure scenario (what input/state, ` +
	`what happens, why it's wrong) or "No refutation found: <one sentence why it holds up>".`

// GetCritiqueAgent constructs the critique_agent LlmAgent: adversarial
// refutation of a claim, plan, or code — distinct from judge_agent (scores
// against a rubric) by being refutation-first rather than evaluation-first.
// This is the standalone, directly-callable critique tool; GetCritiqueLoopAgent
// builds its own separate internal critic (with ExitLoopTool) for the
// iterative revise-until-approved loop, since a critic used inside a
// LoopAgent needs the exit-loop tool wired in and this one deliberately does
// not carry it.
func GetCritiqueAgent(_ context.Context, m model.LLM) agent.Agent {
	critique, err := llmagent.New(llmagent.Config{
		Name:        "critique_agent",
		Model:       m,
		Description: "Adversarially tries to refute or break a claim, plan, or piece of code — finds the concrete failure scenario rather than scoring against a rubric. Use to stress-test something before trusting it.",
		Instruction: CritiqueInstruction,
		OutputKey:   "critique_verdict",
	})
	if err != nil {
		panic("critique_agent: " + err.Error())
	}
	return critique
}

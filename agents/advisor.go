package agents

import (
	"context"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model"
)

// AdvisorInstruction is the system prompt for advisor_agent.
const AdvisorInstruction = `You are a senior engineering advisor consulted before or after a plan is ` +
	`implemented. You are given a plan, approach, or completed change in {advisor_input}. ` +
	`Give a second opinion in at most 5 sentences: state whether the approach is sound, ` +
	`name the single biggest risk or trade-off, and recommend one concrete adjustment if ` +
	`warranted. Do not write code and do not restate the plan back — add judgment the ` +
	`author would not already have.`

// GetAdvisorAgent constructs the advisor_agent LlmAgent: a single-turn
// second-opinion reviewer for a plan or approach, distinct from judge_agent
// (rubric scoring) and critique_agent (adversarial refutation) — advisor
// gives a holistic "is this the right call" read.
func GetAdvisorAgent(_ context.Context, m model.LLM) agent.Agent {
	advisor, err := llmagent.New(llmagent.Config{
		Name:        "advisor_agent",
		Model:       m,
		Description: "Gives a second opinion on a plan, approach, or completed change: soundness, biggest risk, one concrete adjustment. Use before committing to an approach or before declaring work done.",
		Instruction: AdvisorInstruction,
		OutputKey:   "advisor_verdict",
	})
	if err != nil {
		panic("advisor_agent: " + err.Error())
	}
	return advisor
}

package agents

import (
	"context"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model"
)

// JudgeInstruction is the system prompt for judge_agent.
const JudgeInstruction = `You are a rubric-based judge. You are given content to evaluate in ` +
	`{judge_input} and, if provided, a rubric or acceptance criteria in {judge_rubric}. ` +
	`Score the content against correctness, clarity, and completeness. ` +
	`Respond in this exact format:` + "\n" +
	`VERDICT: APPROVED or NEEDS_WORK` + "\n" +
	`REASONS: up to 3 short, specific, actionable bullet points.` + "\n" +
	`Do not soften a NEEDS_WORK verdict with praise — be direct.`

// GetJudgeAgent constructs the judge_agent LlmAgent: scores content against
// a rubric and returns a structured APPROVED/NEEDS_WORK verdict. Distinct
// from critique_agent (which tries to refute/break a claim rather than
// score it) and from advisor_agent (holistic second opinion on an approach,
// not a scored verdict on a concrete artifact).
func GetJudgeAgent(_ context.Context, m model.LLM) agent.Agent {
	judge, err := llmagent.New(llmagent.Config{
		Name:        "judge_agent",
		Model:       m,
		Description: "Scores content against a rubric or acceptance criteria and returns an APPROVED/NEEDS_WORK verdict with specific reasons. Use to check whether finished work actually meets its stated bar.",
		Instruction: JudgeInstruction,
		OutputKey:   "judge_verdict",
	})
	if err != nil {
		panic("judge_agent: " + err.Error())
	}
	return judge
}

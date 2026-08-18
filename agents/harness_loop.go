package agents

import (
	"context"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/agent/workflowagents/loopagent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/exitlooptool"
)

// HarnessLoopMaxIterations bounds the critique_loop cycle count.
const HarnessLoopMaxIterations = 3

const harnessReviserInstruction = `You revise content based on feedback. The current draft is in ` +
	`{harness_draft}; the latest critique is in {harness_critique} (empty on the first pass — in ` +
	`that case, produce an initial draft from {harness_goal}). Apply the critique and output the ` +
	`full revised draft only, no commentary.`

const harnessCriticInstruction = `You are a strict critic reviewing {harness_draft} against the goal ` +
	`in {harness_goal}. If it fully satisfies the goal with no material issues left, call the ` +
	`exit_loop tool and respond with exactly "APPROVED". Otherwise, do NOT call exit_loop — instead ` +
	`give ONE specific, actionable piece of feedback for the next revision.`

// GetCritiqueLoopAgent constructs critique_loop: the harness's "loop"
// capability — a bounded draft/critique cycle (reviser ⇄ critic), built with
// exitlooptool correctly wired into the critic's Tools so an early APPROVED
// verdict actually stops the loop instead of silently burning every
// iteration. Root main.go's own doc_refinement_loop reference example omits
// this (AGENTS.md:190-202 documents it as mandatory) — this constructor is
// the corrected version for new code, not a fix applied to that demo.
func GetCritiqueLoopAgent(_ context.Context, m model.LLM) agent.Agent {
	exitTool, err := exitlooptool.New()
	if err != nil {
		panic("critique_loop: create exit_loop tool: " + err.Error())
	}

	reviser, err := llmagent.New(llmagent.Config{
		Name:        "harness_reviser",
		Model:       m,
		Instruction: harnessReviserInstruction,
		OutputKey:   "harness_draft",
	})
	if err != nil {
		panic("critique_loop: create harness_reviser: " + err.Error())
	}

	critic, err := llmagent.New(llmagent.Config{
		Name:        "harness_critic",
		Model:       m,
		Instruction: harnessCriticInstruction,
		Tools:       []tool.Tool{exitTool},
		OutputKey:   "harness_critique",
	})
	if err != nil {
		panic("critique_loop: create harness_critic: " + err.Error())
	}

	loop, err := loopagent.New(loopagent.Config{
		MaxIterations: HarnessLoopMaxIterations,
		AgentConfig: agent.Config{
			Name: "critique_loop",
			Description: "Iteratively drafts and critiques content (goal in {harness_goal}) for up to " +
				"3 cycles, or until the critic approves, whichever comes first. Use for anything that " +
				"benefits from a revise-until-good-enough pass rather than a single draft.",
			SubAgents: []agent.Agent{reviser, critic},
		},
	})
	if err != nil {
		panic("critique_loop: " + err.Error())
	}
	return loop
}

package agents

import (
	"context"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/agenttool"
)

// ManagerInstruction is the system prompt for manager_agent: the root
// orchestrator for the 30-role SDLC team. It does not do engineering work
// itself — it routes to the specialist(s) who should, then synthesizes
// their output into one coherent answer.
const ManagerInstruction = `You are the Engineering Manager coordinating a team of 30 specialist ` +
	`agents plus cto_agent. Given the incoming request:` + "\n" +
	`1. Identify which specialist(s) are actually relevant — most requests need one, some need two or ` +
	`three working in sequence (e.g. backend_engineer implements, then security_engineer reviews).` + "\n" +
	`2. Call each relevant specialist by name with a clear, specific task description — do not call a ` +
	`specialist "just in case" if their expertise doesn't apply.` + "\n" +
	`3. Synthesize their outputs into one coherent response. Do not just concatenate raw agent output.` + "\n" +
	`4. Before finalizing anything high-stakes, ambiguous, or where a specialist's recommendation ` +
	`carries real risk (schema change, security-relevant code, a decision with no easy undo), call ` +
	`cto_agent with the proposal and include its DECISION in your final answer. Do not call cto_agent ` +
	`for routine, low-risk requests — that would make every trivial task wait on a final-authority check.` + "\n\n" +
	`Remember: cicd_agent and github_pr_agent are advisory-only (no real git/gh/CI exec) — if a request ` +
	`needs an actual git push, PR merge, or CI trigger, say so plainly and tell the user what command to ` +
	`run themselves; do not imply it already happened.`

// GetManagerAgent constructs manager_agent: the root orchestrator wrapping
// all 30 role agents (agents/roles.go) plus cto_agent (agents/cto.go), each
// as an agent-tool via agenttool.New — the same nesting pattern
// agents/harness_loop.go already uses to hide its reviser/critic pair
// behind one callable unit. This keeps the outer "layar-cli" root agent's
// own Tools list from growing by 31 entries; it gains exactly one
// (manager_agent) instead.
//
// tierA/tierB are the same tool.Tool slices GetRoleAgents takes — see that
// function's doc comment for the advisory/builder split.
func GetManagerAgent(_ context.Context, m model.LLM, tierA, tierB []tool.Tool) agent.Agent {
	roleAgents := GetRoleAgents(m, tierA, tierB)
	ctoAgent := GetCTOAgent(context.Background(), m)

	managerTools := make([]tool.Tool, 0, len(roleAgents)+1)
	for _, ra := range roleAgents {
		managerTools = append(managerTools, agenttool.New(ra, nil))
	}
	managerTools = append(managerTools, agenttool.New(ctoAgent, nil))

	manager, err := llmagent.New(llmagent.Config{
		Name:  "manager_agent",
		Model: m,
		Description: "Coordinates a 30-role SDLC specialist team (Product Manager, Backend/Frontend/" +
			"Database/AI/Mobile/Data/DevOps/Security/etc. Engineer, QA, SDLC/CI-CD/GitHub-PR process " +
			"agents, and more) plus cto_agent for final high-stakes calls. Delegate any engineering-" +
			"team-shaped task here rather than attempting it directly.",
		Instruction: ManagerInstruction,
		Tools:       managerTools,
		OutputKey:   "manager_output",
	})
	if err != nil {
		panic("manager_agent: " + err.Error())
	}
	return manager
}

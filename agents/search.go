package agents

import (
	"context"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/geminitool"
)

// SearchInstruction is the system prompt for search_agent.
const SearchInstruction = `You are a specialist in Google Search. Given a query, search the web ` +
	`and answer concisely, citing what you found. If the query is ambiguous, search for the most ` +
	`likely interpretation rather than asking a clarifying question.`

// GetSearchAgent constructs the search_agent LlmAgent: a Gemini-only
// sub-agent wrapping ADK's native geminitool.GoogleSearch (real-time Google
// Search grounding built into Gemini 2.x models — no external API key, no
// REST call, no scraping).
//
// It must live in its own sub-agent, not on the root agent's own tool list:
// Gemini's API forbids mixing a built-in tool like google_search with
// custom function-calling tools in the same request. Wrapping it as a
// sub-agent exposed via agenttool.New (as main.go does for llm_auditor,
// judge_agent, etc.) is ADK's own documented workaround for this — see
// google.golang.org/adk's examples/tools/multipletools/main.go.
func GetSearchAgent(_ context.Context, m model.LLM) agent.Agent {
	search, err := llmagent.New(llmagent.Config{
		Name:        "search_agent",
		Model:       m,
		Description: "Searches the web via Gemini's native Google Search grounding and answers concisely. Use for questions about recent events or facts not in your training data.",
		Instruction: SearchInstruction,
		Tools:       []tool.Tool{geminitool.GoogleSearch{}},
	})
	if err != nil {
		panic("search_agent: " + err.Error())
	}
	return search
}

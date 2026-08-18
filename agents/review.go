package agents

import (
	"context"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/tool"
)

// ReviewInstruction is the system prompt for review_agent.
const ReviewInstruction = `You are a senior code reviewer. You are given a review target — a file ` +
	`path, directory, or description of a change — in {review_input}. Use read_file and ` +
	`grep_search to actually inspect the real files (do not review from memory or assumption). ` +
	`Report concrete findings only: file:line, what's wrong, and the concrete failure scenario ` +
	`(what input/state triggers it). Rank most-severe first. If nothing real is wrong, say so ` +
	`plainly rather than inventing a stylistic nitpick.`

// GetReviewAgent constructs the review_agent LlmAgent. Unlike the other
// harness agents, review_agent is given real file-system tools (read_file,
// grep_search) so it inspects actual files rather than a state-interpolated
// string — a code reviewer that can't read the code isn't reviewing
// anything. Callers must pass the same read_file/grep_search tool.Tool
// instances used elsewhere in the composition root.
func GetReviewAgent(_ context.Context, m model.LLM, readFileTool, grepSearchTool tool.Tool) agent.Agent {
	review, err := llmagent.New(llmagent.Config{
		Name:        "review_agent",
		Model:       m,
		Description: "Reviews real files (reads and greps them directly, not from memory) and reports concrete, file:line findings ranked by severity. Use for reviewing a diff, file, or change before trusting it.",
		Instruction: ReviewInstruction,
		Tools:       []tool.Tool{readFileTool, grepSearchTool},
		OutputKey:   "review_findings",
	})
	if err != nil {
		panic("review_agent: " + err.Error())
	}
	return review
}

// Package bridge demonstrates how to combine Genkit (flow tracing layer) with
// the Google ADK (agent reasoning layer) in a single Go application.
//
// # Architecture
//
// Genkit owns the outer boundary:
//   - Defines the Flow (the observable, MCP-exposed unit of work).
//   - Provides [genkit.Run] sub-step tracing inside the flow.
//   - Handles OpenTelemetry spans, Genkit Developer UI visibility.
//
// ADK owns the inner reasoning loop:
//   - Runs the LlmAgent (model + tools + instruction).
//   - Calls tools, injects results, loops until done.
//
// The bridge is the ctx hand-off: the Genkit flow's context is passed
// directly into the ADK runner, so every ADK reasoning step executes
// inside the active Genkit trace.
//
// # Pattern
//
//	genkit.DefineFlow(g, "MyFlow", func(ctx, input) (output, error) {
//	    // ctx carries the Genkit OpenTelemetry span.
//	    result, err := genkit.Run(ctx, "adk-agent", func() (string, error) {
//	        // ADK agent runs inside the Genkit sub-step — fully traced.
//	        return runADKAgent(ctx, input)
//	    })
//	    return result, err
//	})
package bridge

import (
	"context"
	"fmt"
	"strings"

	// Genkit — flow definition and sub-step tracing.
	"github.com/firebase/genkit/go/core"
	"github.com/firebase/genkit/go/genkit"

	// ADK — agent, runner, session, tool layers.
	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	tool_iface "google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
	"google.golang.org/genai"

	// Local model layer — any provider works here.
	"go-adk-q/model/opencode"
)

// RegisterAnalysisFlow defines a Genkit flow named "AnalyzeFlow" that runs
// an ADK LlmAgent inside a traced sub-step.
//
// The flow accepts a plain string prompt and returns the agent's final text
// response. The ADK agent is equipped with a single example tool
// (summarise_text) to illustrate the tool-call bridge pattern.
//
// g must be an initialised *genkit.Genkit (returned by genkit.Init).
// The returned flow can be called directly in tests or wired into an HTTP
// server with genkit.StartFlowServer.
func RegisterAnalysisFlow(g *genkit.Genkit) *core.Flow[string, string, struct{}] {
	return genkit.DefineFlow(g, "AnalyzeFlow",
		func(ctx context.Context, prompt string) (string, error) {

			// ── Genkit sub-step: ADK agent reasoning ─────────────────────
			// genkit.Run creates a named child span in the active trace.
			// The ctx handed to the inner func is the span's context, so
			// all ADK reasoning steps inherit the Genkit trace.
			return genkit.Run(ctx, "adk-agent", func() (string, error) {
				return runADKAgent(ctx, prompt)
			})
		},
	)
}

// summariseArgs is the typed input schema for the summarise_text tool.
// jsonschema tags are reflected by the ADK to build the FunctionDeclaration
// sent to the LLM.
type summariseArgs struct {
	Text     string `json:"text"      jsonschema:"The text to summarise."`
	MaxWords int    `json:"max_words" jsonschema:"Maximum number of words in the summary."`
}

// summariseResult is the typed output schema for the summarise_text tool.
type summariseResult struct {
	Summary string `json:"summary"`
}

// runADKAgent builds a single-use ADK LlmAgent, runs it with prompt, and
// returns its final text response. ctx should come from inside a Genkit
// sub-step so that ADK reasoning is visible in the Genkit Developer UI.
func runADKAgent(ctx context.Context, prompt string) (string, error) {
	// ── Model ────────────────────────────────────────────────────────────
	// Any model.LLM works here. We use the opencode provider as an example.
	// Swap in failover.New(primary, backups...) for production resilience.
	m, err := opencode.NewModel(ctx, opencode.ConfigFromEnv())
	if err != nil {
		return "", fmt.Errorf("bridge: model: %w", err)
	}

	// ── Tools ────────────────────────────────────────────────────────────
	// ADK tools are typed Go functions wrapped with functiontool.New.
	// The handler receives a tool.Context (not context.Context directly) —
	// call toolCtx.Context() to get the underlying context.Context.
	summariseTool, err := functiontool.New(
		functiontool.Config{
			Name:        "summarise_text",
			Description: "Summarise the provided text in at most max_words words.",
		},
		func(toolCtx tool_iface.Context, args summariseArgs) (summariseResult, error) {
			words := strings.Fields(args.Text)
			limit := args.MaxWords
			if limit <= 0 || limit > len(words) {
				limit = len(words)
			}
			return summariseResult{
				Summary: strings.Join(words[:limit], " "),
			}, nil
		},
	)
	if err != nil {
		return "", fmt.Errorf("bridge: tool: %w", err)
	}

	// ── Agent ────────────────────────────────────────────────────────────
	ag, err := llmagent.New(llmagent.Config{
		Name:        "analysis-agent",
		Model:       m,
		Instruction: "You are a precise analysis assistant. Use the summarise_text tool when asked to summarise content.",
		Tools:       []tool_iface.Tool{summariseTool},
	})
	if err != nil {
		return "", fmt.Errorf("bridge: agent: %w", err)
	}

	// ── Runner ───────────────────────────────────────────────────────────
	// The runner owns session + artifact services. For a single-use flow
	// invocation, in-memory services are sufficient.
	r, err := runner.New(runner.Config{
		AppName:           "genkit-adk-bridge",
		Agent:             ag,
		SessionService:    session.InMemoryService(),
		AutoCreateSession: true,
	})
	if err != nil {
		return "", fmt.Errorf("bridge: runner: %w", err)
	}

	// ── Invoke ───────────────────────────────────────────────────────────
	// ctx comes from the Genkit sub-step — ADK runs inside the active span.
	msg := &genai.Content{
		Role:  genai.RoleUser,
		Parts: []*genai.Part{{Text: prompt}},
	}

	var sb strings.Builder
	for event, err := range r.Run(ctx, "flow-user", "flow-session", msg, agent.RunConfig{}) {
		if err != nil {
			return "", fmt.Errorf("bridge: run: %w", err)
		}
		if event != nil && event.IsFinalResponse() && event.LLMResponse.Content != nil {
			for _, p := range event.LLMResponse.Content.Parts {
				sb.WriteString(p.Text)
			}
		}
	}

	return strings.TrimSpace(sb.String()), nil
}

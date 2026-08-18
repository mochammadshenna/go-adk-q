package main

// agent_turn.go — runTurnWithConfirmations is the one turn-driving loop
// shared by every caller of the ADK Runner in this binary: cmd/tui/chat.go's
// startAgentStream (interactive TUI, streaming, y/n prompt) and the ACP
// bridges in cmd/tui/main.go (synchronous, session/request_permission over
// stdio) and cmd/tui/chat.go's HTTP-only /acp server (no confirmation
// support possible — see below). Extracted once a second real caller needed
// the identical pause/resume-on-confirmation mechanics startAgentStream
// established first this session — the same "extract once there's a second
// consumer" call this repo already made for acp_stdio.go's sendRequest.
//
// Why a shared loop instead of two: exec_command's RequireConfirmation gate
// (tools/exec.go) pauses the SAME way regardless of which Runner caller
// triggered it — ADK's toolconfirmation mechanism doesn't know or care
// whether the eventual human answer comes from a Bubbletea keypress or an
// ACP session/request_permission reply. Only the "how do I ask a human"
// part differs per caller; the event-loop/resume mechanics do not.

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/tool/toolconfirmation"
	"google.golang.org/genai"
)

// runTurnWithConfirmations drives r.Run to completion for one user turn
// (input), automatically resuming across any number of ADK Human-in-the-Loop
// confirmation pauses (google.golang.org/adk/tool/toolconfirmation).
//
// onText, if non-nil, is called with each streamed text delta as it arrives
// (a caller that only wants the final accumulated text, e.g. a synchronous
// ACP bridge, can pass nil).
//
// onConfirm is called synchronously whenever the agent pauses awaiting a
// human decision on toolName(args) (toolCallID identifies the ORIGINAL
// pending tool call, not the wrapping adk_request_confirmation call — that
// correlation is handled internally). It must return the human's decision
// (true = approve) or an error to abort the turn. A nil onConfirm with a
// turn that actually pauses returns a clear error instead of hanging forever
// — this is the correct behavior for a transport that structurally cannot
// support Agent→Client requests (e.g. the HTTP-only /acp server in
// acp_server.go, per its own header comment).
//
// Returns the full accumulated text, the final turn's token usage, and any
// error.
func runTurnWithConfirmations(
	ctx context.Context,
	r *runner.Runner,
	userID, sessionID, input string,
	onText func(delta string),
	onConfirm func(ctx context.Context, toolCallID, toolName string, args map[string]any) (bool, error),
) (fullText string, promptTokens, candidateTokens int32, err error) {
	var result strings.Builder

	content := &genai.Content{Parts: []*genai.Part{{Text: input}}, Role: genai.RoleUser}

	for {
		// accumulated tracks what we have already forwarded so we can
		// compute the delta when a provider returns cumulative text. Reset
		// per r.Run call: each resume-after-confirmation is a fresh model
		// response stream, not a continuation of the prior one's text.
		var accumulated string
		var pendingCallID string
		var pendingCall *genai.FunctionCall

		for event, evErr := range r.Run(ctx, userID, sessionID, content, agent.RunConfig{
			StreamingMode: agent.StreamingModeSSE,
		}) {
			if evErr != nil {
				return result.String(), promptTokens, candidateTokens, evErr
			}
			if ctx.Err() != nil {
				// Bail immediately if the caller cancelled (HTTP disconnect,
				// TUI interrupt key, ACP timeout) — matches the original
				// runAgentSync's defensive check, now shared by every caller.
				return result.String(), promptTokens, candidateTokens, ctx.Err()
			}
			if event == nil {
				continue
			}
			if event.UsageMetadata != nil {
				promptTokens = event.UsageMetadata.PromptTokenCount
				candidateTokens = event.UsageMetadata.CandidatesTokenCount
			}
			if event.LLMResponse.Content == nil {
				continue
			}
			for _, part := range event.LLMResponse.Content.Parts {
				if fc := part.FunctionCall; fc != nil && fc.Name == toolconfirmation.FunctionCallName {
					if original, oerr := toolconfirmation.OriginalCallFrom(fc); oerr == nil {
						pendingCallID = fc.ID
						pendingCall = original
					}
					continue
				}
				if part.Text == "" {
					continue
				}
				var delta string
				if strings.HasPrefix(part.Text, accumulated) {
					// Cumulative: only the new suffix is the delta.
					delta = part.Text[len(accumulated):]
					accumulated = part.Text
				} else {
					// Incremental: the whole Part.Text is the delta.
					delta = part.Text
					accumulated += part.Text
				}
				if delta == "" {
					continue
				}
				result.WriteString(delta)
				if onText != nil {
					onText(delta)
				}
			}
		}

		if pendingCallID == "" {
			return result.String(), promptTokens, candidateTokens, nil
		}
		if onConfirm == nil {
			return result.String(), promptTokens, candidateTokens,
				fmt.Errorf("agent requested confirmation for %q but this transport has no confirmation handler configured", pendingCall.Name)
		}

		confirmed, cerr := onConfirm(ctx, pendingCall.ID, pendingCall.Name, pendingCall.Args)
		if cerr != nil {
			return result.String(), promptTokens, candidateTokens, cerr
		}

		// Resume the same session with the human's decision. Role/name/ID
		// here must exactly match ADK's toolconfirmation contract (see
		// google.golang.org/adk's internal/llminternal/
		// request_confirmation_processor.go): a FunctionResponse named
		// toolconfirmation.FunctionCallName whose ID matches the
		// adk_request_confirmation call just answered.
		content = &genai.Content{
			Role: genai.RoleUser,
			Parts: []*genai.Part{{FunctionResponse: &genai.FunctionResponse{
				Name:     toolconfirmation.FunctionCallName,
				ID:       pendingCallID,
				Response: map[string]any{"confirmed": confirmed},
			}}},
		}
	}
}

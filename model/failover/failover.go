// Package failover provides a model.LLM that automatically retries a request
// against a list of providers in priority order. The first provider whose
// GenerateContent call completes without error wins; subsequent providers are
// never called.
//
// # Motivation
//
// LLM providers occasionally return transient errors (rate-limit 429s, 503
// overloads, model routing failures). Wrapping a primary model with one or
// more fallbacks ensures the agent continues operating when the preferred
// provider is temporarily unavailable.
//
// # Retry semantics
//
// All upstream GenerateContent calls are made in non-streaming mode so that
// a complete response can be buffered before being forwarded to the caller.
// This is necessary because a partially-streamed response cannot be retried
// transparently — once the first token is yielded the caller has committed to
// that provider. By collecting the full response first, the failover model
// can switch providers cleanly on any error.
//
// The caller's stream parameter is accepted but currently does not alter this
// behaviour: responses are always buffered internally and then forwarded as a
// single complete response. This is a deliberate trade-off: reliability over
// streaming latency.
//
// # Usage
//
//	primary, _ := gemini.NewModel(ctx, "gemini-2.5-flash", cfg)
//	backup1, _ := groq.NewModel(ctx, groq.ConfigFromEnv())
//	backup2, _ := nvidia.NewModel(ctx, nvidia.ConfigFromEnv())
//
//	m := failover.New(primary, backup1, backup2)
//	// m.Name() == "failover(gemini-2.5-flash → groq/llama-3.3-70b-versatile → ...)"
//
//	agent, _ := llmagent.New(llmagent.Config{Model: m, ...})
package failover

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"log/slog"
	"strings"

	"google.golang.org/adk/model"
)

// Model is a model.LLM that tries each provider in order and returns the
// first successful response. It satisfies the model.LLM interface.
type Model struct {
	models []model.LLM
	name   string
}

// New creates a failover model from one primary and zero or more fallback
// providers. Providers are tried left-to-right; the first successful
// GenerateContent call wins. Panics if no models are provided.
func New(primary model.LLM, fallbacks ...model.LLM) *Model {
	if primary == nil {
		panic("failover.New: primary model must not be nil")
	}
	all := make([]model.LLM, 0, 1+len(fallbacks))
	all = append(all, primary)
	for _, fb := range fallbacks {
		if fb != nil {
			all = append(all, fb)
		}
	}
	names := make([]string, len(all))
	for i, m := range all {
		names[i] = m.Name()
	}
	return &Model{
		models: all,
		name:   "failover(" + strings.Join(names, " → ") + ")",
	}
}

// Name returns a composite identifier showing the failover chain.
// Example: "failover(gemini-2.5-flash → groq/llama-3.3-70b-versatile)"
func (m *Model) Name() string { return m.name }

// GenerateContent tries each provider in order.
//
// Internally every provider is called with stream=false so that the full
// response can be buffered before being forwarded. A provider is considered
// to have failed if its iterator yields a non-nil error. Each failure is
// logged at WARN level so it is visible without crashing the agent.
//
// If all providers fail, a single error is yielded that wraps every
// individual provider error (joined via errors.Join).
func (m *Model) GenerateContent(
	ctx context.Context,
	req *model.LLMRequest,
	_ bool, // stream — see package doc; always collected internally
) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		var errs []error
		for i, llm := range m.models {
			// Respect context cancellation between provider attempts.
			if ctxErr := ctx.Err(); ctxErr != nil {
				yield(nil, fmt.Errorf("failover: %w", ctxErr))
				return
			}

			buf, err := collectAll(ctx, llm, req)
			if err != nil {
				slog.Warn("failover: provider error, trying next",
					"provider", llm.Name(),
					"index", i,
					"remaining", len(m.models)-i-1,
					"error", err,
				)
				errs = append(errs, fmt.Errorf("%s: %w", llm.Name(), err))
				continue
			}

			if i > 0 {
				slog.Info("failover: recovered via fallback",
					"provider", llm.Name(),
					"index", i,
				)
			}

			for _, resp := range buf {
				if !yield(resp, nil) {
					return
				}
			}
			return // success — stop trying further providers
		}

		// Every provider failed.
		yield(nil, fmt.Errorf("failover: all %d provider(s) failed: %w",
			len(m.models), errors.Join(errs...)))
	}
}

// collectAll drains a non-streaming GenerateContent call into a slice.
// Returns (nil, err) if any iteration step yields a non-nil error.
// Nil *model.LLMResponse entries (e.g. provider heartbeat sentinels) are
// silently dropped — forwarding them would cause a nil-pointer dereference in
// the ADK runner.
func collectAll(ctx context.Context, llm model.LLM, req *model.LLMRequest) ([]*model.LLMResponse, error) {
	var buf []*model.LLMResponse
	for resp, err := range llm.GenerateContent(ctx, req, false) {
		if err != nil {
			return nil, err
		}
		if resp != nil { // some providers yield (nil, nil) as a streaming heartbeat
			buf = append(buf, resp)
		}
	}
	return buf, nil
}

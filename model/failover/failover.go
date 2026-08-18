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
// The caller's stream parameter is passed through to each provider unchanged.
// When stream is false, a provider yields only its one final response, which
// is validated before being forwarded — behaviour is unchanged from a plain
// buffer-then-forward call. When stream is true, each Partial response a
// provider yields is forwarded to the caller the moment it arrives (real
// incremental delivery, not buffered).
//
// Streaming and clean cross-provider failover are in tension: once the first
// token has reached the caller, that provider cannot be swapped out
// transparently — the caller has already committed to it, and a retry would
// either restart the reply from scratch mid-display or mix two providers'
// text in one message. So an attempt that has already forwarded at least one
// Partial response is "committed": any error or failed validation for that
// SAME attempt afterward is surfaced directly, not retried or failed over.
// An attempt that fails BEFORE its first Partial reaches the caller — the
// common case, since most failures are connection-level and happen before a
// provider sends anything — still fails over exactly as before.
//
// # Per-attempt timeout
//
// Each provider attempt runs under a derived context with a deadline (see
// SetAttemptTimeout). Without this, a provider that accepts the connection
// but never responds (slowloris, dead upstream, SDK that ignores ctx) would
// block the entire request forever and defeat the fallback chain. The default
// deadline is zero (disabled) so existing callers keep their current
// behaviour; the chain builder sets a sensible production default.
//
// # Observability
//
// After every GenerateContent call, Stats() reports which provider actually
// served the response and whether a fallback occurred. This is what lets the
// TUI render a live "⚡ served by groq" / "🔀 fell back to groq" badge, turning
// the failover mechanism from an invisible safety net into a visible feature.
//
// # Response validation
//
// A provider can return a nil Go error while the response itself is unusable:
// an OpenAI-compatible upstream that answers HTTP 200 with an {"error": ...}
// body, an empty completion, or a content-filter refusal with no parts. Before
// accepting a response as a win, validateResponse checks for an embedded
// ErrorCode/ErrorMessage and for at least one part with real content. A
// response that fails validation is treated exactly like a transport error:
// it is logged, recorded in Stats(), and the chain moves to the next provider.
//
// # Rate-limit backoff
//
// A 429 (rate limited) is usually resolved by waiting, not by abandoning the
// provider — escalating straight to the next provider on every 429 wastes a
// working provider's quota window. isRateLimited matches on the error text
// rather than a typed status code: model/oaibridge is the only package
// permitted to import firebase/genkit, so this package cannot type-assert
// *core.GenkitError. On a match, GenerateContent retries the same provider
// once after RateLimitBackoff (default 1s, see SetRateLimitBackoff) before
// giving up and moving on.
//
// # Usage
//
//	primary, _ := gemini.NewModel(ctx, "gemini-2.5-flash", cfg)
//	backup1, _ := groq.NewModel(ctx, groq.ConfigFromEnv())
//	backup2, _ := nvidia.NewModel(ctx, nvidia.ConfigFromEnv())
//
//	m := failover.New(primary, backup1, backup2)
//	m.SetAttemptTimeout(90 * time.Second) // production safety
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
	"sync"
	"time"

	"google.golang.org/adk/model"
)

// defaultRateLimitBackoff is how long GenerateContent waits before retrying a
// provider that returned a 429. See SetRateLimitBackoff.
const defaultRateLimitBackoff = 1 * time.Second

// errConsumerStopped signals that yield returned false — the consumer of
// GenerateContent's iterator stopped pulling values (e.g. it broke out of its
// range loop early). This must be handled distinctly from a provider error:
// Go's range-over-func forbids calling yield again after it has returned
// false once, so once this sentinel appears anywhere the whole call must
// return immediately with no further yield call of any kind (not even trying
// the next provider, not even the final "all providers failed" summary) —
// unlike an ordinary provider failure, which is fine to fail over from.
var errConsumerStopped = errors.New("failover: consumer stopped consuming")

// Model is a model.LLM that tries each provider in order and returns the
// first successful response. It satisfies the model.LLM interface.
type Model struct {
	mu               sync.Mutex
	models           []model.LLM
	name             string
	timeout          time.Duration // per-attempt deadline; zero disables
	rateLimitBackoff time.Duration // delay before the single 429 retry

	// Observability — written under mu during GenerateContent, read by Stats().
	lastProvider string
	lastFellBack bool
	lastFailed   []string
}

// New creates a failover model from one primary and zero or more fallback
// providers. Providers are tried left-to-right; the first successful
// GenerateContent call wins. Panics if no models are provided.
//
// The variadic signature is kept intentionally simple so existing callers and
// tests (failover.New(primary, backup)) keep working unchanged.
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
		models:           all,
		name:             "failover(" + strings.Join(names, " → ") + ")",
		rateLimitBackoff: defaultRateLimitBackoff,
	}
}

// SetAttemptTimeout configures the per-provider attempt deadline. A zero value
// disables the timeout (legacy behaviour). Typical production value: 60–120s.
func (m *Model) SetAttemptTimeout(d time.Duration) { m.mu.Lock(); m.timeout = d; m.mu.Unlock() }

// SetRateLimitBackoff configures how long GenerateContent waits before
// retrying a provider that just returned a 429. The retry always happens
// (a rate limit is the one error class worth waiting out); zero means retry
// immediately with no delay. Defaults to 1s.
func (m *Model) SetRateLimitBackoff(d time.Duration) {
	m.mu.Lock()
	m.rateLimitBackoff = d
	m.mu.Unlock()
}

// Name returns a composite identifier showing the failover chain.
// Example: "failover(gemini-2.5-flash → groq/llama-3.3-70b-versatile)"
func (m *Model) Name() string { return m.name }

// Names returns a copy of the provider names in priority order. Useful for
// surfacing the configured chain in a UI (e.g. a /providers panel).
func (m *Model) Names() []string {
	out := make([]string, len(m.models))
	for i, llm := range m.models {
		out[i] = llm.Name()
	}
	return out
}

// CallStats reports the outcome of one specific GenerateContent call. Unlike
// the shared state read by Stats(), a *CallStats passed via WithStats is
// written only by the goroutine that owns that call, so it is race-free under
// concurrent GenerateContent calls on the same *Model (e.g. an HTTP server
// handling multiple requests against one shared failover chain).
type CallStats struct {
	Provider string
	FellBack bool
	Failed   []string
}

type callStatsKey struct{}

// WithStats returns a context that GenerateContent will populate with this
// call's outcome, in addition to (not instead of) the shared state Stats()
// reports. Use this from any caller that may invoke GenerateContent
// concurrently on the same *Model; Stats() alone is only safe for a
// single-threaded caller (e.g. the TUI, which serializes calls itself).
func WithStats(ctx context.Context, s *CallStats) context.Context {
	return context.WithValue(ctx, callStatsKey{}, s)
}

// Stats reports the outcome of the most recent GenerateContent call:
//   - provider: the name of the provider that served the response, or "" if
//     every provider failed or no call has been made yet.
//   - fellBack: true when the winning provider was not the primary (i.e. at
//     least one earlier provider failed first).
//   - failed: the names of providers that failed before the winner (may be
//     empty even when fellBack is true, e.g. on a single-provider chain where
//     the only attempt failed).
func (m *Model) Stats() (provider string, fellBack bool, failed []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastProvider, m.lastFellBack, append([]string(nil), m.lastFailed...)
}

// GenerateContent tries each provider in order.
//
// The caller's stream flag is passed through to each provider unchanged. When
// stream is true, every Partial response a provider yields is forwarded to
// the caller immediately (real incremental delivery, not buffered) as soon as
// it arrives; the final non-partial response is still validated (see
// validateResponse) before being forwarded, so a stream that ends in an empty
// or error-payload completion is caught the same way a non-streaming one is.
//
// Once any Partial response for an attempt has reached the caller, that
// attempt is committed: a subsequent transport error or failed validation for
// the SAME attempt is surfaced directly instead of retrying (same provider)
// or failing over (next provider). Silently retrying after partial output has
// already been shown would either restart the reply from scratch mid-display
// or mix two providers' text in one message — both worse than a clear error.
// Failover and the same-provider 429 retry remain fully transparent for any
// attempt that errors or fails validation BEFORE its first Partial response
// reaches the caller — the common case, since most failures are connection-
// level and happen before the provider sends anything at all.
//
// A provider is considered to have failed if its iterator yields a non-nil
// error, or if its buffered responses fail validateResponse. Each failure
// (before any output was shown) is logged at WARN level so it is visible
// without crashing the agent.
//
// If all providers fail, a single error is yielded that wraps every
// individual provider error (joined via errors.Join).
func (m *Model) GenerateContent(
	ctx context.Context,
	req *model.LLMRequest,
	stream bool,
) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		// Reset observability state for this call.
		m.mu.Lock()
		m.lastProvider = ""
		m.lastFellBack = false
		m.lastFailed = nil
		m.mu.Unlock()

		// call is this goroutine's own stats sink, if the caller opted in via
		// WithStats. Written directly with no lock: only this call's goroutine
		// ever touches it, so it stays correct under concurrent GenerateContent
		// calls on the same *Model — unlike the shared m.last* fields above,
		// which only the single-threaded TUI caller may safely read via Stats().
		call, _ := ctx.Value(callStatsKey{}).(*CallStats)

		var errs []error
		for i, llm := range m.models {
			// Respect context cancellation between provider attempts.
			if ctxErr := ctx.Err(); ctxErr != nil {
				yield(nil, fmt.Errorf("failover: %w", ctxErr))
				return
			}

			streamedAny, ok, err := m.attempt(ctx, llm, req, stream, yield)

			if !ok {
				if errors.Is(err, errConsumerStopped) {
					// yield already returned false once — Go's range-over-func
					// forbids calling it again for any reason, including the
					// "all providers failed" summary below. Stop outright.
					return
				}
				if streamedAny {
					// Already forwarded partial output for this attempt —
					// cannot cleanly fail over without showing garbled
					// mixed-provider text. Surface the failure directly.
					yield(nil, fmt.Errorf("%s: %w", llm.Name(), err))
					return
				}
				slog.Warn("failover: provider error, trying next",
					"provider", llm.Name(),
					"index", i,
					"remaining", len(m.models)-i-1,
					"error", err,
				)
				m.mu.Lock()
				m.lastFailed = append(m.lastFailed, llm.Name())
				m.mu.Unlock()
				if call != nil {
					call.Failed = append(call.Failed, llm.Name())
				}
				errs = append(errs, fmt.Errorf("%s: %w", llm.Name(), err))
				continue
			}

			// Success — record which provider served and whether we fell back.
			m.mu.Lock()
			m.lastProvider = llm.Name()
			if i > 0 {
				m.lastFellBack = true
				slog.Info("failover: recovered via fallback",
					"provider", llm.Name(),
					"index", i,
				)
			}
			m.mu.Unlock()
			if call != nil {
				call.Provider = llm.Name()
				call.FellBack = i > 0
			}
			return // success — attempt already forwarded every response itself
		}

		// Every provider failed.
		yield(nil, fmt.Errorf("failover: all %d provider(s) failed: %w",
			len(m.models), errors.Join(errs...)))
	}
}

// attempt calls one provider under the configured per-attempt timeout,
// validates the response, and — if the failure looks like a 429 and nothing
// has been streamed to the caller yet — retries the same provider exactly
// once after rateLimitBackoff before giving up.
func (m *Model) attempt(
	ctx context.Context,
	llm model.LLM,
	req *model.LLMRequest,
	stream bool,
	yield func(*model.LLMResponse, error) bool,
) (streamedAny, ok bool, err error) {
	streamedAny, ok, err = m.attemptOnce(ctx, llm, req, stream, yield)
	if ok || streamedAny || !isRateLimited(err) {
		// Success, already-shown output (see GenerateContent's doc comment),
		// or a non-rate-limit error — none of these get a same-provider retry.
		return streamedAny, ok, err
	}

	m.mu.Lock()
	backoff := m.rateLimitBackoff
	m.mu.Unlock()

	slog.Info("failover: 429 rate limited, retrying same provider after backoff",
		"kind", "BackoffEnqueued", "provider", llm.Name(), "backoff", backoff)
	if sleepErr := sleepBackoff(ctx, backoff); sleepErr != nil {
		return false, false, fmt.Errorf("backoff wait cancelled: %w", sleepErr)
	}

	streamedAny, ok, retryErr := m.attemptOnce(ctx, llm, req, stream, yield)
	if retryErr != nil {
		return streamedAny, false, fmt.Errorf("after 429 retry: %w", retryErr)
	}
	return streamedAny, ok, nil
}

// attemptOnce runs a single provider call under the per-attempt deadline.
//
// When stream is true, every Partial response is forwarded to yield the
// moment it arrives — real incremental delivery — while also being retained
// so the final non-partial response can still be checked by validateResponse
// once the provider's iterator completes. When stream is false, providers
// yield only their one final response, so this behaves exactly as a
// buffer-then-validate-then-forward call, unchanged from before.
//
// streamedAny reports whether any Partial response reached yield during this
// call — see GenerateContent's doc comment for why that gates retry/failover.
func (m *Model) attemptOnce(
	ctx context.Context,
	llm model.LLM,
	req *model.LLMRequest,
	stream bool,
	yield func(*model.LLMResponse, error) bool,
) (streamedAny, ok bool, err error) {
	m.mu.Lock()
	t := m.timeout
	m.mu.Unlock()

	attemptCtx := ctx
	cancel := context.CancelFunc(func() {})
	if t > 0 {
		attemptCtx, cancel = context.WithTimeout(ctx, t)
	}
	defer cancel()

	var buf []*model.LLMResponse
	for resp, genErr := range llm.GenerateContent(attemptCtx, req, stream) {
		if genErr != nil {
			return streamedAny, false, genErr
		}
		if resp == nil {
			continue // some providers yield (nil, nil) as a streaming heartbeat
		}
		buf = append(buf, resp)
		if resp.Partial {
			streamedAny = true
			if !yield(resp, nil) {
				return streamedAny, false, errConsumerStopped
			}
		}
	}

	if err := validateResponse(buf); err != nil {
		return streamedAny, false, err
	}

	for _, resp := range buf {
		if resp.Partial {
			continue // already forwarded live, above
		}
		if !yield(resp, nil) {
			return streamedAny, false, errConsumerStopped
		}
	}
	return streamedAny, true, nil
}

// sleepBackoff waits for d or returns ctx.Err() if the context is cancelled
// first. d<=0 returns immediately.
func sleepBackoff(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// isRateLimited reports whether err looks like an HTTP 429 / rate-limit
// response. This package cannot type-assert *core.GenkitError (only
// model/oaibridge may import firebase/genkit — see AGENTS.md §4), so it
// matches on the error text that oaibridge's wrapOAIError produces, which
// always includes the upstream status code and message.
func isRateLimited(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "429") ||
		strings.Contains(s, "rate limit") ||
		strings.Contains(s, "rate_limit") ||
		strings.Contains(s, "too many requests")
}

// validateResponse rejects a "successful" call whose response is not actually
// usable: an embedded error payload (200-with-{"error":...}) or a response
// with no content at all (empty completion, content-filter refusal). Both
// must be treated as failures so the failover chain escalates instead of
// forwarding a broken reply to the caller.
func validateResponse(buf []*model.LLMResponse) error {
	if len(buf) == 0 {
		return fmt.Errorf("empty response: provider returned no content")
	}
	hasContent := false
	for _, resp := range buf {
		if resp == nil {
			continue
		}
		if resp.ErrorCode != "" || resp.ErrorMessage != "" {
			return fmt.Errorf("provider returned an error payload in a successful response: code=%q message=%q",
				resp.ErrorCode, resp.ErrorMessage)
		}
		if resp.Content == nil {
			continue
		}
		for _, p := range resp.Content.Parts {
			if p == nil {
				continue
			}
			if p.Text != "" || p.FunctionCall != nil || p.FunctionResponse != nil ||
				p.InlineData != nil || p.FileData != nil ||
				p.ExecutableCode != nil || p.CodeExecutionResult != nil {
				hasContent = true
			}
		}
	}
	if !hasContent {
		return fmt.Errorf("empty response: provider returned no usable content")
	}
	return nil
}

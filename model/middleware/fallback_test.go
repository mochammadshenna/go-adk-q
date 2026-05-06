// Package middleware_test tests the fallback middleware.
//
// These tests verify the two behavioural properties of our Fallback vs the
// upstream Genkit v1.7.0 implementation:
//
//  1. Plain Go errors trigger fallback (compat_oai network errors).
//  2. IsolateConfig=false (default) uses TypeScript @genkit-ai/middleware
//     semantics: FallbackModel.Config ?? req.Config — prefer the fallback
//     model's own config; fall back to the request's config only when the
//     model has none.
package middleware_test

import (
	"context"
	"errors"
	"testing"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/core"
	"github.com/firebase/genkit/go/core/api"

	"go-adk-q/model/middleware"
)

// ── Test helpers ─────────────────────────────────────────────────────────────

// mockModel is a controllable ai.Model for tests. It records whether Generate
// was called and what config the request carried.
type mockModel struct {
	name       string
	resp       *ai.ModelResponse
	err        error
	called     int
	lastConfig any // Config field of the last ModelRequest received
}

func (m *mockModel) Name() string         { return m.name }
func (m *mockModel) Register(api.Registry) {}
func (m *mockModel) Generate(_ context.Context, req *ai.ModelRequest, _ ai.ModelStreamCallback) (*ai.ModelResponse, error) {
	m.called++
	m.lastConfig = req.Config
	return m.resp, m.err
}

// textResp builds a minimal successful ModelResponse with the given text.
func textResp(text string) *ai.ModelResponse {
	return &ai.ModelResponse{
		Message: &ai.Message{
			Role:    ai.RoleModel,
			Content: []*ai.Part{ai.NewTextPart(text)},
		},
	}
}

// primaryFunc returns a ModelNext that always succeeds.
func primarySuccess(resp *ai.ModelResponse) ai.ModelNext {
	return func(_ context.Context, _ *ai.ModelParams) (*ai.ModelResponse, error) {
		return resp, nil
	}
}

// primaryFail returns a ModelNext that always fails with the given error.
func primaryFail(err error) ai.ModelNext {
	return func(_ context.Context, _ *ai.ModelParams) (*ai.ModelResponse, error) {
		return nil, err
	}
}

// wrapModel is a convenience helper: creates a Fallback, calls New, and
// invokes WrapModel. It fails the test immediately if New returns an error.
func wrapModel(t *testing.T, fb *middleware.Fallback, req *ai.ModelRequest, next ai.ModelNext) (*ai.ModelResponse, error) {
	t.Helper()
	hooks, err := fb.New(context.Background())
	if err != nil {
		t.Fatalf("Fallback.New: %v", err)
	}
	params := &ai.ModelParams{Request: req}
	return hooks.WrapModel(context.Background(), params, next)
}

// ── Tests ─────────────────────────────────────────────────────────────────────

// TestFallbackNotTriggeredOnSuccess verifies that a successful primary call
// never invokes any fallback model.
func TestFallbackNotTriggeredOnSuccess(t *testing.T) {
	fb := &middleware.Fallback{
		Models: []middleware.FallbackModel{
			{Model: &mockModel{name: "unused", resp: textResp("unused")}},
		},
	}

	resp, err := wrapModel(t, fb, &ai.ModelRequest{}, primarySuccess(textResp("primary ok")))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Message.Content[0].Text != "primary ok" {
		t.Errorf("unexpected response text: %q", resp.Message.Content[0].Text)
	}
	// The fallback model must not have been called.
	if got := fb.Models[0].Model.(*mockModel).called; got != 0 {
		t.Errorf("fallback model called %d times, want 0", got)
	}
}

// TestFallbackTriggersOnPlainError verifies that a plain Go error (not a
// GenkitError) triggers fallback. This is the primary bug fixed vs. upstream.
func TestFallbackTriggersOnPlainError(t *testing.T) {
	fallback1 := &mockModel{name: "fallback1", resp: textResp("fallback response")}
	fb := &middleware.Fallback{
		Models: []middleware.FallbackModel{{Model: fallback1}},
	}

	resp, err := wrapModel(t, fb, &ai.ModelRequest{},
		primaryFail(errors.New("connection refused: plain error from compat_oai")))

	if err != nil {
		t.Fatalf("expected fallback to succeed, got error: %v", err)
	}
	if fallback1.called != 1 {
		t.Errorf("fallback model called %d times, want 1", fallback1.called)
	}
	if resp.Message.Content[0].Text != "fallback response" {
		t.Errorf("unexpected response text: %q", resp.Message.Content[0].Text)
	}
}

// TestFallbackTriggersOnRetryableGenkitError verifies that a GenkitError with
// a retryable status (UNAVAILABLE) triggers fallback.
func TestFallbackTriggersOnRetryableGenkitError(t *testing.T) {
	fallback1 := &mockModel{name: "fallback1", resp: textResp("fallback ok")}
	fb := &middleware.Fallback{
		Models: []middleware.FallbackModel{{Model: fallback1}},
	}

	primaryErr := core.NewError(core.UNAVAILABLE, "service unavailable")

	resp, err := wrapModel(t, fb, &ai.ModelRequest{}, primaryFail(primaryErr))

	if err != nil {
		t.Fatalf("expected fallback to succeed, got error: %v", err)
	}
	if fallback1.called != 1 {
		t.Errorf("fallback model called %d times, want 1", fallback1.called)
	}
	if resp.Message.Content[0].Text != "fallback ok" {
		t.Errorf("unexpected response text: %q", resp.Message.Content[0].Text)
	}
}

// TestFallbackNotTriggeredOnNonRetryableStatus verifies that a GenkitError
// with a non-retryable status (INVALID_ARGUMENT) does NOT trigger fallback —
// it propagates directly to the caller.
func TestFallbackNotTriggeredOnNonRetryableStatus(t *testing.T) {
	fallback1 := &mockModel{name: "fallback1", resp: textResp("should not see this")}
	fb := &middleware.Fallback{
		Models: []middleware.FallbackModel{{Model: fallback1}},
	}

	primaryErr := core.NewError(core.INVALID_ARGUMENT, "bad request")

	_, err := wrapModel(t, fb, &ai.ModelRequest{}, primaryFail(primaryErr))

	if err == nil {
		t.Fatal("expected error to propagate, got nil")
	}
	var ge *core.GenkitError
	if !errors.As(err, &ge) || ge.Status != core.INVALID_ARGUMENT {
		t.Errorf("expected INVALID_ARGUMENT GenkitError, got: %v", err)
	}
	if fallback1.called != 0 {
		t.Errorf("fallback model called %d times, want 0 (non-retryable error)", fallback1.called)
	}
}

// TestFallbackExhaustsAllModels verifies that when all fallback models fail,
// the last error is returned.
func TestFallbackExhaustsAllModels(t *testing.T) {
	err1 := errors.New("fallback1 failed")
	err2 := errors.New("fallback2 failed")
	fallback1 := &mockModel{name: "fb1", err: err1}
	fallback2 := &mockModel{name: "fb2", err: err2}

	fb := &middleware.Fallback{
		Models: []middleware.FallbackModel{
			{Model: fallback1},
			{Model: fallback2},
		},
	}

	_, err := wrapModel(t, fb, &ai.ModelRequest{},
		primaryFail(errors.New("primary failed")))

	if err == nil {
		t.Fatal("expected error when all models fail, got nil")
	}
	if !errors.Is(err, err2) {
		t.Errorf("expected last fallback error %v, got: %v", err2, err)
	}
	if fallback1.called != 1 {
		t.Errorf("fallback1 called %d times, want 1", fallback1.called)
	}
	if fallback2.called != 1 {
		t.Errorf("fallback2 called %d times, want 1", fallback2.called)
	}
}

// TestFallbackStopsAfterFirstSuccess verifies that once a fallback model
// succeeds, subsequent fallback models are not tried.
func TestFallbackStopsAfterFirstSuccess(t *testing.T) {
	fallback1 := &mockModel{name: "fb1", resp: textResp("fb1 wins")}
	fallback2 := &mockModel{name: "fb2", resp: textResp("fb2 never called")}

	fb := &middleware.Fallback{
		Models: []middleware.FallbackModel{
			{Model: fallback1},
			{Model: fallback2},
		},
	}

	resp, err := wrapModel(t, fb, &ai.ModelRequest{},
		primaryFail(errors.New("primary down")))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Message.Content[0].Text != "fb1 wins" {
		t.Errorf("unexpected response: %q", resp.Message.Content[0].Text)
	}
	if fallback1.called != 1 {
		t.Errorf("fallback1 called %d times, want 1", fallback1.called)
	}
	if fallback2.called != 0 {
		t.Errorf("fallback2 called %d times, want 0 (should not be reached)", fallback2.called)
	}
}

// TestIsolateConfigFalsePrefersFallbackModelConfig verifies that when
// IsolateConfig is false (default) and the fallback model has its own Config
// set, that Config wins — mirroring TypeScript's
// config = normalizedModel.config ?? req.config logic.
func TestIsolateConfigFalsePrefersFallbackModelConfig(t *testing.T) {
	fallback1 := &mockModel{name: "fb1", resp: textResp("ok")}
	fb := &middleware.Fallback{
		Models:        []middleware.FallbackModel{{Model: fallback1, Config: "fallback-config"}},
		IsolateConfig: false, // default
	}

	req := &ai.ModelRequest{Config: "original-config"}

	_, err := wrapModel(t, fb, req, primaryFail(errors.New("primary down")))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// TypeScript: config = normalizedModel.config ?? req.config
	// Model has "fallback-config" → that wins over "original-config".
	if fallback1.lastConfig != "fallback-config" {
		t.Errorf("fallback model received config %q, want %q (IsolateConfig=false should prefer fallback model's config)",
			fallback1.lastConfig, "fallback-config")
	}
}

// TestIsolateConfigFalseUsesRequestConfigWhenModelHasNone verifies that when
// IsolateConfig is false and the fallback model has no Config (nil), the
// original request's Config is used — the nil coalesce half of
// TypeScript's config = normalizedModel.config ?? req.config.
func TestIsolateConfigFalseUsesRequestConfigWhenModelHasNone(t *testing.T) {
	fallback1 := &mockModel{name: "fb1", resp: textResp("ok")}
	fb := &middleware.Fallback{
		// Config is nil — model has no own config.
		Models:        []middleware.FallbackModel{{Model: fallback1, Config: nil}},
		IsolateConfig: false, // default
	}

	req := &ai.ModelRequest{Config: "original-config"}

	_, err := wrapModel(t, fb, req, primaryFail(errors.New("primary down")))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// TypeScript: config = nil ?? "original-config" → "original-config"
	if fallback1.lastConfig != "original-config" {
		t.Errorf("fallback model received config %q, want %q (IsolateConfig=false, no model config → use request config)",
			fallback1.lastConfig, "original-config")
	}
}

// TestIsolateConfigTrueUsesFallbackModelConfig verifies that when IsolateConfig
// is true, each FallbackModel's Config overrides the original request's Config.
func TestIsolateConfigTrueUsesFallbackModelConfig(t *testing.T) {
	fallback1 := &mockModel{name: "fb1", resp: textResp("ok")}
	fb := &middleware.Fallback{
		Models:        []middleware.FallbackModel{{Model: fallback1, Config: "fallback-config"}},
		IsolateConfig: true, // use each fallback model's own config
	}

	req := &ai.ModelRequest{Config: "original-config"}

	_, err := wrapModel(t, fb, req, primaryFail(errors.New("primary down")))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fallback1.lastConfig != "fallback-config" {
		t.Errorf("fallback model received config %q, want %q (IsolateConfig=true should use fallback's own config)",
			fallback1.lastConfig, "fallback-config")
	}
}

// TestFallbackNoModelsReturnsOriginalError verifies that an empty Models slice
// propagates the primary error unchanged.
func TestFallbackNoModelsReturnsOriginalError(t *testing.T) {
	fb := &middleware.Fallback{Models: nil}

	primaryErr := errors.New("primary failed, no fallbacks available")
	_, err := wrapModel(t, fb, &ai.ModelRequest{}, primaryFail(primaryErr))

	if !errors.Is(err, primaryErr) {
		t.Errorf("expected primary error %v, got: %v", primaryErr, err)
	}
}

// TestFallbackCustomStatuses verifies that a user-supplied Statuses list is
// respected: only GenkitErrors with a matching status trigger fallback.
func TestFallbackCustomStatuses(t *testing.T) {
	fallback1 := &mockModel{name: "fb1", resp: textResp("ok")}
	fb := &middleware.Fallback{
		Models:   []middleware.FallbackModel{{Model: fallback1}},
		Statuses: []core.StatusName{core.RESOURCE_EXHAUSTED}, // only rate-limit errors
	}

	// RESOURCE_EXHAUSTED matches → fallback triggered.
	if _, err := wrapModel(t, fb, &ai.ModelRequest{},
		primaryFail(core.NewError(core.RESOURCE_EXHAUSTED, "rate limited"))); err != nil {
		t.Errorf("expected fallback to succeed for RESOURCE_EXHAUSTED, got: %v", err)
	}
	if fallback1.called != 1 {
		t.Errorf("fallback1 called %d times after RESOURCE_EXHAUSTED, want 1", fallback1.called)
	}

	// UNAVAILABLE does NOT match custom Statuses → propagates.
	fallback1.called = 0
	_, err := wrapModel(t, fb, &ai.ModelRequest{},
		primaryFail(core.NewError(core.UNAVAILABLE, "unavailable")))
	if err == nil {
		t.Error("expected UNAVAILABLE to propagate when not in custom Statuses, got nil error")
	}
	if fallback1.called != 0 {
		t.Errorf("fallback1 called %d times for non-matching status, want 0", fallback1.called)
	}
}

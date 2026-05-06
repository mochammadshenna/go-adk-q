// Package middleware provides Genkit model middleware for the go-adk-q project.
//
// # Fallback middleware
//
// Fallback is a local re-implementation of Genkit v1.7.0's built-in
// [github.com/firebase/genkit/go/plugins/middleware.Fallback] that:
//
//  1. Accepts [ai.Model] values directly — no Genkit registry or
//     genkit.Genkit context value is required. Models created via
//     oaibridge.NewModel, googlegenai, or any other source can be used
//     as fallbacks without registering them.
//
//  2. Retries on plain Go errors in addition to retryable [core.GenkitError]
//     values. This is an intentional, pragmatic departure from TypeScript's
//     implementation: the TypeScript SDK relies on every model plugin wrapping
//     its errors as GenkitError, which Go's compat_oai does not do for all
//     error paths (e.g. network-level failures). Plain errors retrying ensures
//     the observable behaviour matches TypeScript even when the underlying error
//     type differs. HTTP errors from oaibridge are separately wrapped as
//     GenkitError by [oaibridge.wrapOAIError] so they already benefit from
//     accurate status-code based filtering before reaching this middleware.
//
//  3. Implements TypeScript @genkit-ai/middleware isolateConfig semantics
//     exactly:
//
//     IsolateConfig false (default): config = FallbackModel.Config ?? req.Config
//     — i.e. prefer the fallback model's own config, fall back to the
//     original request's config when the model has none.
//
//     IsolateConfig true: config = FallbackModel.Config
//     — always use the fallback model's config, even when it is nil.
//
// # Usage
//
//	fb := &middleware.Fallback{
//	    Models: []middleware.FallbackModel{
//	        {Model: groqModel},
//	        {Model: nvidiaModel},
//	    },
//	}
//	// Use with ai.WithUse when calling genkit.Generate:
//	resp, err := genkit.Generate(ctx, g,
//	    ai.WithModel(primary),
//	    ai.WithPrompt("hello"),
//	    ai.WithUse(fb),
//	)
package middleware

import (
	"context"
	"errors"
	"slices"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/core"
)

// defaultFallbackStatuses mirrors the upstream Genkit fallback defaults.
// These are the GenkitError status codes that trigger a fallback attempt.
// Plain (non-GenkitError) errors always trigger fallback regardless of this list.
var defaultFallbackStatuses = []core.StatusName{
	core.UNAVAILABLE,
	core.DEADLINE_EXCEEDED,
	core.RESOURCE_EXHAUSTED,
	core.ABORTED,
	core.INTERNAL,
	core.NOT_FOUND,
	core.UNIMPLEMENTED,
}

// FallbackModel pairs a Genkit model with an optional per-model Config.
//
// Config behaviour depends on [Fallback.IsolateConfig]:
//   - IsolateConfig false (default): Config is preferred over the original
//     request's Config. If Config is nil, the request's Config is used.
//     Mirrors TypeScript: config = normalizedModel.config ?? req.config.
//   - IsolateConfig true: Config is always used (even when nil), replacing
//     the original request's Config entirely.
type FallbackModel struct {
	// Model is the fallback Genkit model to try.
	Model ai.Model
	// Config is the per-model configuration override.
	// See [FallbackModel] docs for how IsolateConfig interacts with this field.
	Config any
}

// Fallback is a Genkit middleware that tries alternative models when the
// primary model call fails.
//
// It implements [ai.Middleware] so it can be passed to [ai.WithUse].
type Fallback struct {
	// Models is the ordered list of fallback models to try after the primary fails.
	Models []FallbackModel

	// Statuses is the set of GenkitError status codes that trigger fallback.
	// Defaults to [defaultFallbackStatuses] when empty.
	// Plain (non-GenkitError) errors always trigger fallback regardless of this field.
	Statuses []core.StatusName

	// IsolateConfig controls which Config the fallback model receives.
	//
	// false (default) — config = FallbackModel.Config ?? req.Config.
	// The fallback model's own Config is preferred; if it is nil, the
	// original request's Config is used. Mirrors TypeScript
	// @genkit-ai/middleware isolateConfig: false behaviour.
	//
	// true — config = FallbackModel.Config (always, even if nil). The
	// fallback model is fully isolated from the caller's generation
	// parameters.
	IsolateConfig bool
}

// Name satisfies [ai.Middleware]. Returns a stable identifier for this middleware.
func (f *Fallback) Name() string { return "go-adk-q/middleware/fallback" }

// New satisfies [ai.Middleware]. It allocates and returns the per-call [ai.Hooks]
// bundle. No per-call state is needed, so the same wrapModel closure is reused.
func (f *Fallback) New(_ context.Context) (*ai.Hooks, error) {
	return &ai.Hooks{
		WrapModel: f.wrapModel,
	}, nil
}

func (f *Fallback) wrapModel(
	ctx context.Context,
	params *ai.ModelParams,
	next ai.ModelNext,
) (*ai.ModelResponse, error) {
	resp, err := next(ctx, params)
	if err == nil {
		return resp, nil
	}

	// Non-retryable GenkitError → propagate immediately (e.g. INVALID_ARGUMENT).
	if !f.isRetryable(err) {
		return nil, err
	}

	lastErr := err
	for _, fm := range f.Models {
		req := *params.Request // shallow copy — safe for Config replacement

		if f.IsolateConfig {
			// IsolateConfig=true: always use the fallback model's own config,
			// even if it is nil — fully isolates from the original request.
			req.Config = fm.Config
		} else {
			// IsolateConfig=false (default): prefer the fallback model's config;
			// fall back to the request's config only when the model has none.
			// Mirrors TypeScript: config = normalizedModel.config ?? req.config
			if fm.Config != nil {
				req.Config = fm.Config
			}
			// else: req.Config already holds the original request's Config.
		}

		resp, err := fm.Model.Generate(ctx, &req, params.Callback)
		if err == nil {
			return resp, nil
		}
		lastErr = err

		// A non-retryable GenkitError from a fallback model stops the chain.
		if !f.isRetryable(err) {
			return nil, err
		}
	}
	return nil, lastErr
}

// isRetryable reports whether err should trigger trying the next model.
//
// Plain Go errors (non-GenkitError) are always considered retryable. This is
// an intentional departure from TypeScript's implementation (which only retries
// on GenkitError), but achieves equivalent observable behaviour: in TypeScript
// all model plugins wrap their errors as GenkitError, whereas Go's compat_oai
// may surface plain network/connection errors. The oaibridge package converts
// HTTP API errors to GenkitError via wrapOAIError, so those already benefit
// from accurate status-code filtering; this fallback ensures plain network
// errors (e.g. connection refused) also trigger provider failover.
//
// Only GenkitErrors with a status absent from the configured statuses list are
// treated as non-retryable (e.g. INVALID_ARGUMENT — retrying other models
// won't fix a malformed request).
func (f *Fallback) isRetryable(err error) bool {
	var ge *core.GenkitError
	if !errors.As(err, &ge) {
		// Plain error — always retry (fixes compat_oai HTTP errors).
		return true
	}
	// GenkitError — retry only if its status is in the allowed list.
	return slices.Contains(f.statuses(), ge.Status)
}

func (f *Fallback) statuses() []core.StatusName {
	if len(f.Statuses) > 0 {
		return f.Statuses
	}
	return defaultFallbackStatuses
}

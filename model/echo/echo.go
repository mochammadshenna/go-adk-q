// Package echo provides a model.LLM that always succeeds with a fixed text
// response. It is intended as a zero-credential last-resort fallback in a
// failover chain and as a test double for local development when no real
// provider API keys are available.
//
// # Activating the echo fallback
//
// Set the environment variable before running:
//
//	ECHO_FALLBACK_ENABLED=1
//
// main.go checks this variable at startup and, if set, appends an echo model
// to the end of the failover chain. This makes the following Makefile target
// work without any third-party API keys:
//
//	make test-failover-echo
//
// which sets GOOGLE_MODEL=gemini-intentionally-broken so Gemini returns a 400,
// then expects the echo model to catch the fall.
//
// # Caution
//
// The echo model never calls any LLM — it simply reflects its configured
// message back to the caller. It should never be used in production. Its sole
// purpose is to demonstrate and verify the failover chain end-to-end during
// local development and CI without requiring real credentials.
package echo

import (
	"context"
	"iter"
	"os"

	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

// EnvEnabled is the environment variable that activates the echo fallback.
// Set it to "1" to append the echo model to the failover chain.
const EnvEnabled = "ECHO_FALLBACK_ENABLED"

// DefaultMessage is returned by Default() and when New is called with an empty
// string. It explains why the echo model is responding and what to do next.
const DefaultMessage = "[echo fallback] All configured providers failed or " +
	"none were configured. Set GROQ_API_KEY, NVIDIA_API_KEY, " +
	"OPENROUTER_API_KEY, or HF_TOKEN to add a real fallback provider."

const modelName = "echo"

// Model is a model.LLM that always yields a single, fixed text response.
// It satisfies the model.LLM interface and requires no network access.
type Model struct {
	message string
}

// New returns an echo Model that always responds with message.
// If message is empty, DefaultMessage is used.
func New(message string) *Model {
	if message == "" {
		message = DefaultMessage
	}
	return &Model{message: message}
}

// Default returns an echo Model with the package-level DefaultMessage.
func Default() *Model { return New("") }

// Enabled reports whether ECHO_FALLBACK_ENABLED=1 is set in the environment.
// Convenient for a one-liner guard in main.go:
//
//	if echo.Enabled() { echoLLM = echo.Default() }
func Enabled() bool { return os.Getenv(EnvEnabled) == "1" }

// Name satisfies model.LLM. Always returns "echo".
func (m *Model) Name() string { return modelName }

// GenerateContent always yields a single successful LLMResponse containing
// the configured message. The context, request, and stream flag are ignored.
func (m *Model) GenerateContent(
	_ context.Context,
	_ *model.LLMRequest,
	_ bool,
) iter.Seq2[*model.LLMResponse, error] {
	resp := &model.LLMResponse{
		Content: &genai.Content{
			Parts: []*genai.Part{{Text: m.message}},
		},
	}
	return func(yield func(*model.LLMResponse, error) bool) {
		yield(resp, nil)
	}
}

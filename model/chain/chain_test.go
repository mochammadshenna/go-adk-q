package chain_test

// chain_test.go closes the zero-test-coverage gap flagged by the 2026-07-17
// audit (see SESSION_HANDOFF.md): both main.go and cmd/tui/main.go depend on
// chain.Build, but until now it had no automated coverage of its own —
// provider ordering, PROVIDER_SELECTED, and WithSelected precedence were only
// ever verified by one manual `go run . console` session. None of the
// per-provider NewModel constructors this test exercises perform any network
// I/O at construction time (verified: they just wrap config into an
// oaibridge.Model struct), so fake API key values are safe here — no live
// provider call is ever made.

import (
	"context"
	"strings"
	"testing"

	"go-adk-q/model/chain"
)

// TestBuild_NoProvidersConfigured verifies the zero-configuration error path:
// with no provider env vars set, Build fails with a message listing the env
// vars an operator can set, rather than panicking or returning a nil model.
func TestBuild_NoProvidersConfigured(t *testing.T) {
	clearAllProviderEnv(t)

	_, err := chain.Build(context.Background())
	if err == nil {
		t.Fatal("expected an error when no providers are configured, got nil")
	}
	if !strings.Contains(err.Error(), "no model providers configured") {
		t.Errorf("error = %q, want it to mention 'no model providers configured'", err.Error())
	}
}

// TestBuild_CanonicalOrder verifies that with multiple providers configured,
// the chain orders them per the canonical priority documented in the package
// doc comment (github > gemini > groq > nvidia > openrouter > opencode >
// huggingface), not simply the order env vars happened to be read.
func TestBuild_CanonicalOrder(t *testing.T) {
	clearAllProviderEnv(t)
	t.Setenv("NVIDIA_API_KEY", "fake-nvidia-key")
	t.Setenv("GROQ_API_KEY", "fake-groq-key")

	f, err := chain.Build(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	names := f.Names()
	if len(names) != 2 {
		t.Fatalf("Names() = %v, want 2 entries", names)
	}
	if !strings.Contains(strings.ToLower(names[0]), "groq") {
		t.Errorf("Names()[0] = %q, want groq first (canonical order: groq before nvidia)", names[0])
	}
	if !strings.Contains(strings.ToLower(names[1]), "nvidia") {
		t.Errorf("Names()[1] = %q, want nvidia second", names[1])
	}
}

// TestBuild_ProviderSelectedEnvReordersChain verifies that PROVIDER_SELECTED
// promotes the named provider to the front of the chain without dropping the
// rest — a regression test for the root/TUI drift bug (F4/F5) this package
// was introduced to fix.
func TestBuild_ProviderSelectedEnvReordersChain(t *testing.T) {
	clearAllProviderEnv(t)
	t.Setenv("GROQ_API_KEY", "fake-groq-key")
	t.Setenv("NVIDIA_API_KEY", "fake-nvidia-key")
	t.Setenv("PROVIDER_SELECTED", "nvidia")

	f, err := chain.Build(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	names := f.Names()
	if len(names) != 2 {
		t.Fatalf("Names() = %v, want 2 entries (PROVIDER_SELECTED must reorder, not drop providers)", names)
	}
	if !strings.Contains(strings.ToLower(names[0]), "nvidia") {
		t.Errorf("Names()[0] = %q, want nvidia promoted to primary via PROVIDER_SELECTED", names[0])
	}
}

// TestBuild_WithSelectedTakesPrecedenceOverEnv verifies that an explicit
// WithSelected option (the TUI /model picker) wins over PROVIDER_SELECTED,
// per the package doc comment on WithSelected.
func TestBuild_WithSelectedTakesPrecedenceOverEnv(t *testing.T) {
	clearAllProviderEnv(t)
	t.Setenv("GROQ_API_KEY", "fake-groq-key")
	t.Setenv("NVIDIA_API_KEY", "fake-nvidia-key")
	t.Setenv("PROVIDER_SELECTED", "groq") // would otherwise keep groq primary (already is)

	f, err := chain.Build(context.Background(), chain.WithSelected("nvidia", ""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	names := f.Names()
	if !strings.Contains(strings.ToLower(names[0]), "nvidia") {
		t.Errorf("Names()[0] = %q, want nvidia — WithSelected must take precedence over PROVIDER_SELECTED", names[0])
	}
}

// TestBuild_EchoFallbackOnly verifies that ECHO_FALLBACK_ENABLED=1 alone
// (with zero real providers configured) produces a working single-provider
// echo chain instead of the "no providers configured" error.
func TestBuild_EchoFallbackOnly(t *testing.T) {
	clearAllProviderEnv(t)
	t.Setenv("ECHO_FALLBACK_ENABLED", "1")

	f, err := chain.Build(context.Background())
	if err != nil {
		t.Fatalf("unexpected error with echo fallback enabled: %v", err)
	}
	if len(f.Names()) != 1 {
		t.Errorf("Names() = %v, want exactly 1 (echo only)", f.Names())
	}
}

// clearAllProviderEnv unsets every provider-credential env var chain.Build
// reads, so each test starts from a known "nothing configured" baseline
// regardless of what the invoking shell has exported. t.Setenv registers
// cleanup automatically, restoring the prior value (including "unset") after
// the test — safe to call from every test in this file since none use
// t.Parallel().
func clearAllProviderEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"GITHUB_PAT", "GOOGLE_API_KEY", "GOOGLE_MODEL", "GROQ_API_KEY",
		"NVIDIA_API_KEY", "OPENROUTER_API_KEY", "OPENCODE_API_KEY", "HF_TOKEN",
		"PROVIDER_SELECTED", "ECHO_FALLBACK_ENABLED",
	} {
		t.Setenv(key, "")
	}
}

package main

import (
	"testing"

	"go-adk-q/model/catalog"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
)

// driveFormKeys feeds character-input messages into f, one at a time, via
// plain Update calls — no command-chasing. Typing a rune never needs huh's
// internal nextFieldMsg/nextGroupMsg transition chain, and bubbles'
// textinput cursor blink command (returned as cmd after nearly every
// keystroke) blocks on a channel that nothing drives outside a real
// tea.Program, so calling it here would hang. Use driveFormSubmit for
// messages (Enter) that need the transition chain followed.
func driveFormKeys(t *testing.T, f *huh.Form, msgs ...tea.Msg) *huh.Form {
	t.Helper()
	for _, msg := range msgs {
		m, _ := f.Update(msg)
		f = m.(*huh.Form)
	}
	return f
}

// driveFormSubmit sends one message (typically Enter) and follows up to
// maxHops of the immediate follow-up commands huh's own group/field
// transitions produce synchronously (nextFieldMsg -> nextGroupMsg). Capped,
// not unbounded: chasing every returned cmd indefinitely previously hung a
// test on bubbles' cursor blink command, which blocks on a timer channel
// nothing drives outside a real tea.Program — confirmed by reproducing that
// exact hang during development of this test.
func driveFormSubmit(t *testing.T, f *huh.Form, msg tea.Msg, maxHops int) *huh.Form {
	t.Helper()
	m, cmd := f.Update(msg)
	f = m.(*huh.Form)
	for i := 0; cmd != nil && i < maxHops; i++ {
		next := cmd()
		if next == nil {
			break
		}
		m, cmd = f.Update(next)
		f = m.(*huh.Form)
	}
	return f
}

func keyRune(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

// TestAuthMethodForm_AcceptsDefaultAPIKey confirms stage 1 completes with
// "API Key" — the pre-highlighted default — on a single Enter.
func TestAuthMethodForm_AcceptsDefaultAPIKey(t *testing.T) {
	result := &loginFormResult{}
	f := buildAuthMethodForm(result)
	f.Init()
	f = driveFormSubmit(t, f, tea.KeyMsg{Type: tea.KeyEnter}, 3)

	if result.authMethod != loginAuthAPIKey {
		t.Errorf("authMethod = %q, want %q", result.authMethod, loginAuthAPIKey)
	}
	if f.State != huh.StateCompleted {
		t.Errorf("state = %v, want StateCompleted", f.State)
	}
}

// TestAPIKeyForm_TypedCharactersReachBoundField is the regression test for a
// real bug caught live during development: driving the form via a pty/expect
// harness that delivered multiple typed characters bundled into a single
// tea.KeyMsg (bubbletea batches rapid input under load) left result.key
// empty — bubbles' textinput handled that fine in principle, but the
// end-to-end pty behavior could never be fully pinned down, and shipping on
// an unverified assumption is worse than proving the actual binding
// mechanism directly. Driving one tea.KeyMsg per rune here is exactly how a
// real terminal delivers ordinary single-keystroke typing, and is what this
// test asserts stays correct.
func TestAPIKeyForm_TypedCharactersReachBoundField(t *testing.T) {
	result := &loginFormResult{}
	providers := catalog.All()
	if len(providers) == 0 {
		t.Fatal("no providers registered — check main.go's init()")
	}
	f := buildAPIKeyForm(providers, result)
	f.Init()

	if result.provider != providers[0].Provider {
		t.Fatalf("default provider = %q, want %q (first registered)", result.provider, providers[0].Provider)
	}

	// Confirm the default provider, advancing focus to the key field.
	f = driveFormSubmit(t, f, tea.KeyMsg{Type: tea.KeyEnter}, 3)

	for _, r := range "abc123" {
		f = driveFormKeys(t, f, keyRune(r))
	}
	if result.key != "abc123" {
		t.Fatalf("result.key = %q, want %q after typing", result.key, "abc123")
	}

	f = driveFormSubmit(t, f, tea.KeyMsg{Type: tea.KeyEnter}, 3)
	if f.State != huh.StateCompleted {
		t.Errorf("state = %v, want StateCompleted", f.State)
	}
	if result.key != "abc123" {
		t.Errorf("result.key after submit = %q, want %q", result.key, "abc123")
	}
	if result.provider != providers[0].Provider {
		t.Errorf("result.provider = %q, want %q", result.provider, providers[0].Provider)
	}
}

// TestAPIKeyForm_EmptyKeyRejected confirms the empty-key validation actually
// blocks submission instead of silently accepting it.
func TestAPIKeyForm_EmptyKeyRejected(t *testing.T) {
	result := &loginFormResult{}
	providers := catalog.All()
	f := buildAPIKeyForm(providers, result)
	f.Init()

	f = driveFormSubmit(t, f, tea.KeyMsg{Type: tea.KeyEnter}, 3) // confirm provider
	f = driveFormSubmit(t, f, tea.KeyMsg{Type: tea.KeyEnter}, 3) // try to submit empty key

	if f.State == huh.StateCompleted {
		t.Error("form completed with an empty key — validation should have blocked it")
	}
}

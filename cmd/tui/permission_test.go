package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

// newPermissionTestModel builds a minimal chatModel with a pending
// permission request, sufficient to exercise Update's interception and
// permissionPromptView without a real runner/session.
func newPermissionTestModel(resp chan bool) chatModel {
	ti := textarea.New()
	return chatModel{
		textInput: ti,
		viewport:  viewport.New(80, 24),
		width:     120,
		ready:     true,
		loading:   true,
		pendingPermission: &permissionRequest{
			toolName: "exec_command",
			args:     map[string]any{"command": "rm -rf /tmp/scratch"},
			hint:     "Approve running exec_command?",
			callID:   "call-1",
			resp:     resp,
		},
	}
}

func TestPermissionPromptView_ShowsToolAndCommand(t *testing.T) {
	resp := make(chan bool, 1)
	m := newPermissionTestModel(resp)
	s := makeTestStyles(0)

	got := m.permissionPromptView(s)
	if !strings.Contains(got, "exec_command") {
		t.Errorf("prompt missing tool name; got:\n%s", got)
	}
	if !strings.Contains(got, "rm -rf /tmp/scratch") {
		t.Errorf("prompt missing command; got:\n%s", got)
	}
	if !strings.Contains(got, "approve") || !strings.Contains(got, "deny") {
		t.Errorf("prompt missing y/n hint; got:\n%s", got)
	}
}

func TestUpdate_PermissionPrompt_ApproveSendsTrueAndClears(t *testing.T) {
	resp := make(chan bool, 1)
	m := newPermissionTestModel(resp)

	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	cm := m2.(chatModel)

	if cm.pendingPermission != nil {
		t.Error("pendingPermission should be cleared after approval")
	}
	select {
	case v := <-resp:
		if !v {
			t.Error("expected true sent on resp channel for approval, got false")
		}
	default:
		t.Fatal("expected a value on resp channel after 'y', got none")
	}
}

func TestUpdate_PermissionPrompt_DenySendsFalseAndClears(t *testing.T) {
	for _, key := range []string{"n", "esc"} {
		t.Run(key, func(t *testing.T) {
			resp := make(chan bool, 1)
			m := newPermissionTestModel(resp)

			var keyMsg tea.KeyMsg
			if key == "esc" {
				keyMsg = tea.KeyMsg{Type: tea.KeyEsc}
			} else {
				keyMsg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
			}

			m2, _ := m.Update(keyMsg)
			cm := m2.(chatModel)

			if cm.pendingPermission != nil {
				t.Error("pendingPermission should be cleared after deny")
			}
			select {
			case v := <-resp:
				if v {
					t.Error("expected false sent on resp channel for deny, got true")
				}
			default:
				t.Fatal("expected a value on resp channel after deny key, got none")
			}
		})
	}
}

func TestUpdate_PermissionPrompt_OtherKeysAreSwallowed(t *testing.T) {
	resp := make(chan bool, 1)
	m := newPermissionTestModel(resp)

	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	cm := m2.(chatModel)

	if cm.pendingPermission == nil {
		t.Error("pendingPermission should remain set for an unrecognized key")
	}
	select {
	case v := <-resp:
		t.Errorf("expected no value on resp channel for an unrecognized key, got %v", v)
	default:
		// expected: nothing sent
	}
}

func TestUpdate_PermissionPrompt_CtrlCQuits(t *testing.T) {
	resp := make(chan bool, 1)
	m := newPermissionTestModel(resp)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("expected a non-nil tea.Cmd for ctrl+c")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Errorf("expected tea.QuitMsg from ctrl+c, got %T", msg)
	}
}

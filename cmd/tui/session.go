package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ── Session persistence ────────────────────────────────────────────────────────
//
// A session is the full message history of one TUI run.  Saving is manual
// (ctrl+s); loading is not automatic — the file is plain JSON so users can
// read and share it outside the TUI.
//
// Storage location: ~/.go-adk-q/session.json
// Permissions:      0700 on the directory, 0600 on the file.

// savedMsg is the JSON-serialisable counterpart of chatMsg.
type savedMsg struct {
	Role string    `json:"role"`
	Text string    `json:"text"`
	At   time.Time `json:"at,omitempty"`
}

// sessionDir returns (creating if necessary) the ~/.go-adk-q directory.
func sessionDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".go-adk-q")
	return dir, os.MkdirAll(dir, 0o700)
}

// saveSession writes all messages to ~/.go-adk-q/session.json.
// It returns the display path (with ~ substituted for the home directory)
// so the caller can show it in the status bar.
func saveSession(msgs []chatMsg) (string, error) {
	dir, err := sessionDir()
	if err != nil {
		return "", err
	}

	saved := make([]savedMsg, len(msgs))
	for i, m := range msgs {
		saved[i] = savedMsg{Role: m.role, Text: m.text, At: m.at}
	}

	data, err := json.MarshalIndent(saved, "", "  ")
	if err != nil {
		return "", err
	}

	full := filepath.Join(dir, "session.json")
	if err := os.WriteFile(full, data, 0o600); err != nil {
		return "", err
	}

	// Replace the home prefix with ~ for a compact status message.
	home, _ := os.UserHomeDir()
	display := strings.Replace(full, home, "~", 1)
	return display, nil
}

// loadLastSession reads ~/.go-adk-q/session.json.
// Returns (nil, nil) when the file does not exist.
func loadLastSession() ([]chatMsg, error) {
	dir, err := sessionDir()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(filepath.Join(dir, "session.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var saved []savedMsg
	if err := json.Unmarshal(data, &saved); err != nil {
		return nil, err
	}

	msgs := make([]chatMsg, len(saved))
	for i, s := range saved {
		msgs[i] = chatMsg{role: s.Role, text: s.Text, at: s.At}
	}
	return msgs, nil
}

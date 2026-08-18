package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestSplitAttachmentsBySize_RealFiles exercises splitAttachmentsBySize
// against real files on disk (not mocks) — one under the cap, one over —
// verifying the oversized file is skipped and reported, and the small file
// still passes through untouched (F10).
func TestSplitAttachmentsBySize_RealFiles(t *testing.T) {
	dir := t.TempDir()

	smallPath := filepath.Join(dir, "small.txt")
	if err := os.WriteFile(smallPath, []byte("hello world"), 0o644); err != nil {
		t.Fatalf("write small file: %v", err)
	}

	bigPath := filepath.Join(dir, "big.log")
	bigContent := bytes.Repeat([]byte("x"), maxAttachmentSize+1)
	if err := os.WriteFile(bigPath, bigContent, 0o644); err != nil {
		t.Fatalf("write big file: %v", err)
	}

	missingPath := filepath.Join(dir, "does-not-exist.txt")

	ok, oversized := splitAttachmentsBySize([]string{smallPath, bigPath, missingPath})

	if len(ok) != 2 {
		t.Fatalf("ok = %v, want 2 entries (small.txt + the missing path, which stat can't classify)", ok)
	}
	foundSmall, foundMissing := false, false
	for _, p := range ok {
		if p == smallPath {
			foundSmall = true
		}
		if p == missingPath {
			foundMissing = true
		}
	}
	if !foundSmall {
		t.Errorf("expected small.txt in ok list, got %v", ok)
	}
	if !foundMissing {
		t.Errorf("expected the unstattable missing path to pass through as ok (existing read path handles the error), got %v", ok)
	}

	if len(oversized) != 1 || oversized[0] != "big.log" {
		t.Errorf("oversized = %v, want [big.log]", oversized)
	}
}

// TestSplitAttachmentsBySize_ExactlyAtLimit verifies the boundary: a file of
// exactly maxAttachmentSize bytes is NOT skipped (only strictly-over is).
func TestSplitAttachmentsBySize_ExactlyAtLimit(t *testing.T) {
	dir := t.TempDir()
	exactPath := filepath.Join(dir, "exact.txt")
	if err := os.WriteFile(exactPath, bytes.Repeat([]byte("y"), maxAttachmentSize), 0o644); err != nil {
		t.Fatalf("write exact-size file: %v", err)
	}

	ok, oversized := splitAttachmentsBySize([]string{exactPath})

	if len(oversized) != 0 {
		t.Errorf("oversized = %v, want empty — exactly-at-limit must pass", oversized)
	}
	if len(ok) != 1 || ok[0] != exactPath {
		t.Errorf("ok = %v, want [%s]", ok, exactPath)
	}
}

// TestReplaceAtFilter_TrailingSpace guards against a real reported bug: when
// an @mention sits at the very end of what the user has typed (the common
// case — type "@foo", press Tab/Enter), the selected filename must be
// followed by a space so continuing to type doesn't glue directly onto the
// filename (e.g. "@server.goplease" instead of "@server.go please").
func TestReplaceAtFilter_TrailingSpace(t *testing.T) {
	cases := []struct {
		name         string
		val          string
		selectedFile string
		want         string
	}{
		{
			name:         "mention at end of input, nothing typed after",
			val:          "create hello world Go in @server",
			selectedFile: "cmd/mcp-server/main.go",
			want:         "create hello world Go in @cmd/mcp-server/main.go ",
		},
		{
			name:         "no @ in input at all",
			val:          "create hello world Go in ",
			selectedFile: "main.go",
			want:         "create hello world Go in @main.go ",
		},
		{
			name:         "text already follows the mention — existing space preserved, not duplicated",
			val:          "look at @serv please",
			selectedFile: "server.go",
			want:         "look at @server.go please",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := replaceAtFilter(tc.val, tc.selectedFile)
			if got != tc.want {
				t.Errorf("replaceAtFilter(%q, %q) = %q, want %q", tc.val, tc.selectedFile, got, tc.want)
			}
		})
	}
}

// TestAtFileMenuNav_WrapsAroundAtEnds guards against a real reported bug:
// the @ file autocomplete menu only supported arrow-down; pressing up at the
// top item did nothing instead of wrapping to the bottom, unlike normal
// dropdown/select navigation. Exercises the actual key-handling branch in
// chatModel.Update by driving real tea.KeyMsg values through it.
func TestAtFileMenuNav_WrapsAroundAtEnds(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	m := newRenderCacheTestModel(nil)
	m.atFileActive = true
	m.atFileItems = loadAtFileItems(dir)
	m.atFileCwd = dir
	m.atFileIdx = 0
	m.textInput.SetValue("@")

	if n := len(filterAtFileItems(m.atFileItems, "")); n != 3 {
		t.Fatalf("test setup: got %d files in %s, want 3", n, dir)
	}

	pressNamed := func(kt tea.KeyType) {
		next, _ := m.Update(tea.KeyMsg{Type: kt})
		nm, ok := next.(chatModel)
		if !ok {
			t.Fatalf("Update did not return a chatModel")
		}
		m = nm
	}

	pressNamed(tea.KeyUp)
	if m.atFileIdx != 2 {
		t.Errorf("after up from index 0 (top), atFileIdx = %d, want 2 (wrap to bottom)", m.atFileIdx)
	}

	pressNamed(tea.KeyDown)
	if m.atFileIdx != 0 {
		t.Errorf("after down from index 2 (bottom), atFileIdx = %d, want 0 (wrap to top)", m.atFileIdx)
	}
}

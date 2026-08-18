package tools

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestEditFile_ReplacesUniqueMatch(t *testing.T) {
	dir := withTempCwd(t)
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main\n\nfunc foo() {}\n"), 0o644); err != nil {
		t.Fatalf("write a.go: %v", err)
	}

	res, err := editFile(nil, editFileArgs{Path: "a.go", OldString: "func foo() {}", NewString: "func bar() {}"})
	if err != nil {
		t.Fatalf("editFile: %v", err)
	}
	if res.Replaced != 1 {
		t.Errorf("Replaced = %d, want 1", res.Replaced)
	}

	got, err := os.ReadFile(filepath.Join(dir, "a.go"))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	want := "package main\n\nfunc bar() {}\n"
	if string(got) != want {
		t.Errorf("file content = %q, want %q", string(got), want)
	}
}

func TestEditFile_ErrorsOnZeroMatches(t *testing.T) {
	dir := withTempCwd(t)
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write a.go: %v", err)
	}

	if _, err := editFile(nil, editFileArgs{Path: "a.go", OldString: "does not exist", NewString: "x"}); err == nil {
		t.Fatal("expected an error for old_string not found, got nil")
	}
}

func TestEditFile_ErrorsOnMultipleMatchesWithoutReplaceAll(t *testing.T) {
	dir := withTempCwd(t)
	content := "needle\nsome text\nneedle\n"
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte(content), 0o644); err != nil {
		t.Fatalf("write a.txt: %v", err)
	}

	if _, err := editFile(nil, editFileArgs{Path: "a.txt", OldString: "needle", NewString: "found"}); err == nil {
		t.Fatal("expected an error for ambiguous (multiple) old_string matches, got nil")
	}

	// File must be untouched — a refused ambiguous edit must not partially apply.
	got, err := os.ReadFile(filepath.Join(dir, "a.txt"))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != content {
		t.Errorf("file was modified despite refused ambiguous edit: got %q, want unchanged %q", string(got), content)
	}
}

func TestEditFile_ReplaceAllReplacesEveryOccurrence(t *testing.T) {
	dir := withTempCwd(t)
	content := "needle\nsome text\nneedle\n"
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte(content), 0o644); err != nil {
		t.Fatalf("write a.txt: %v", err)
	}

	res, err := editFile(nil, editFileArgs{Path: "a.txt", OldString: "needle", NewString: "found", ReplaceAll: true})
	if err != nil {
		t.Fatalf("editFile: %v", err)
	}
	if res.Replaced != 2 {
		t.Errorf("Replaced = %d, want 2", res.Replaced)
	}

	got, err := os.ReadFile(filepath.Join(dir, "a.txt"))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	want := "found\nsome text\nfound\n"
	if string(got) != want {
		t.Errorf("file content = %q, want %q", string(got), want)
	}
}

func TestEditFile_RejectsPathEscape(t *testing.T) {
	withTempCwd(t)
	if _, err := editFile(nil, editFileArgs{Path: "../escape.txt", OldString: "x", NewString: "y"}); err == nil {
		t.Fatal("expected an error escaping the working directory, got nil")
	}
}

func TestEditFile_RejectsOversizedFile(t *testing.T) {
	dir := withTempCwd(t)
	big := bytes.Repeat([]byte("x"), maxFileToolSize+1024)
	if err := os.WriteFile(filepath.Join(dir, "big.txt"), big, 0o644); err != nil {
		t.Fatalf("write big.txt: %v", err)
	}

	if _, err := editFile(nil, editFileArgs{Path: "big.txt", OldString: "x", NewString: "y", ReplaceAll: true}); err == nil {
		t.Fatal("expected an error refusing to edit an oversized file, got nil")
	}

	// Must be untouched — a refused oversized edit must never partially write.
	got, err := os.ReadFile(filepath.Join(dir, "big.txt"))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !bytes.Equal(got, big) {
		t.Error("oversized file was modified despite the refusal")
	}
}

func TestEditFile_RejectsIdenticalOldAndNewString(t *testing.T) {
	dir := withTempCwd(t)
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("same\n"), 0o644); err != nil {
		t.Fatalf("write a.txt: %v", err)
	}

	if _, err := editFile(nil, editFileArgs{Path: "a.txt", OldString: "same", NewString: "same"}); err == nil {
		t.Fatal("expected an error for identical old_string/new_string, got nil")
	}
}

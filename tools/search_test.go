package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGrepSearch_FindsRealMatches(t *testing.T) {
	dir := withTempCwd(t)
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main\nfunc foo() {}\n"), 0o644); err != nil {
		t.Fatalf("write a.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.go"), []byte("package main\nfunc bar() {}\n"), 0o644); err != nil {
		t.Fatalf("write b.go: %v", err)
	}

	res, err := grepSearch(nil, grepArgs{Pattern: `func \w+\(\)`})
	if err != nil {
		t.Fatalf("grepSearch: %v", err)
	}
	if res.Count != 2 {
		t.Fatalf("Count = %d, want 2, matches=%v", res.Count, res.Matches)
	}
}

func TestGrepSearch_SkipsGitDir(t *testing.T) {
	dir := withTempCwd(t)
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "config"), []byte("needle\n"), 0o644); err != nil {
		t.Fatalf("write .git/config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "real.txt"), []byte("needle here too\n"), 0o644); err != nil {
		t.Fatalf("write real.txt: %v", err)
	}

	res, err := grepSearch(nil, grepArgs{Pattern: "needle"})
	if err != nil {
		t.Fatalf("grepSearch: %v", err)
	}
	if res.Count != 1 {
		t.Fatalf("Count = %d, want 1 (only real.txt, .git/ skipped), matches=%v", res.Count, res.Matches)
	}
	if res.Matches[0].Path != "real.txt" {
		t.Errorf("match path = %q, want %q", res.Matches[0].Path, "real.txt")
	}
}

func TestGrepSearch_TruncatesAtCap(t *testing.T) {
	dir := withTempCwd(t)
	var content string
	for i := 0; i < maxGrepResults+10; i++ {
		content += "needle\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "many.txt"), []byte(content), 0o644); err != nil {
		t.Fatalf("write many.txt: %v", err)
	}

	res, err := grepSearch(nil, grepArgs{Pattern: "needle"})
	if err != nil {
		t.Fatalf("grepSearch: %v", err)
	}
	if !res.Truncated {
		t.Errorf("Truncated = false, want true")
	}
	if res.Count != maxGrepResults {
		t.Errorf("Count = %d, want %d", res.Count, maxGrepResults)
	}
}

// TestGrepSearch_ReportsSkippedFileOnScanError is the regression test for the
// harness-audit finding that grep_search silently swallowed scanner.Err() —
// a file with a single line longer than bufio.Scanner's 1 MiB buffer used to
// contribute zero matches with no indication anything went wrong, making it
// indistinguishable from "genuinely no matches in this file." It must now be
// reported in SkippedFiles, and other files must still be searched normally.
func TestGrepSearch_ReportsSkippedFileOnScanError(t *testing.T) {
	dir := withTempCwd(t)
	tooLong := make([]byte, 2*1024*1024) // 2 MiB, no newline at all
	for i := range tooLong {
		tooLong[i] = 'x'
	}
	if err := os.WriteFile(filepath.Join(dir, "huge-line.txt"), tooLong, 0o644); err != nil {
		t.Fatalf("write huge-line.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "normal.txt"), []byte("needle here\n"), 0o644); err != nil {
		t.Fatalf("write normal.txt: %v", err)
	}

	res, err := grepSearch(nil, grepArgs{Pattern: "needle"})
	if err != nil {
		t.Fatalf("grepSearch: %v", err)
	}
	if res.Count != 1 || len(res.Matches) != 1 || res.Matches[0].Path != "normal.txt" {
		t.Errorf("expected exactly 1 match from normal.txt, got count=%d matches=%v", res.Count, res.Matches)
	}
	if len(res.SkippedFiles) != 1 || res.SkippedFiles[0] != "huge-line.txt" {
		t.Errorf("SkippedFiles = %v, want [huge-line.txt]", res.SkippedFiles)
	}
}

func TestGrepSearch_RejectsInvalidPattern(t *testing.T) {
	withTempCwd(t)
	if _, err := grepSearch(nil, grepArgs{Pattern: "("}); err == nil {
		t.Fatal("expected an error for an invalid regexp, got nil")
	}
}

func TestGrepSearch_RejectsPathEscape(t *testing.T) {
	withTempCwd(t)
	if _, err := grepSearch(nil, grepArgs{Pattern: "x", Path: "../../.."}); err == nil {
		t.Fatal("expected an error escaping the working directory, got nil")
	}
}

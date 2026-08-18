package tools

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// withTempCwd chdirs into a fresh t.TempDir() for the duration of the test
// (restoring the original cwd on cleanup) — resolveConfinedPath confines to
// the process cwd, so exercising real confinement behavior requires the
// test's real files to actually live under that cwd, not just any t.TempDir().
func withTempCwd(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("os.Chdir(%q): %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(orig); err != nil {
			t.Fatalf("restore cwd %q: %v", orig, err)
		}
	})
	return dir
}

func TestReadFile_RealFile(t *testing.T) {
	dir := withTempCwd(t)
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello world"), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	res, err := readFile(nil, readFileArgs{Path: "hello.txt"})
	if err != nil {
		t.Fatalf("readFile: %v", err)
	}
	if res.Content != "hello world" {
		t.Errorf("Content = %q, want %q", res.Content, "hello world")
	}
	if res.Truncated {
		t.Errorf("Truncated = true, want false for a small file")
	}
}

func TestReadFile_Truncates(t *testing.T) {
	dir := withTempCwd(t)
	big := bytes.Repeat([]byte("x"), maxFileToolSize+1024)
	if err := os.WriteFile(filepath.Join(dir, "big.log"), big, 0o644); err != nil {
		t.Fatalf("write big file: %v", err)
	}

	res, err := readFile(nil, readFileArgs{Path: "big.log"})
	if err != nil {
		t.Fatalf("readFile: %v", err)
	}
	if !res.Truncated {
		t.Errorf("Truncated = false, want true for a file over maxFileToolSize")
	}
	if len(res.Content) != maxFileToolSize {
		t.Errorf("len(Content) = %d, want %d", len(res.Content), maxFileToolSize)
	}
}

func TestReadFile_SecretWarning(t *testing.T) {
	dir := withTempCwd(t)
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("SECRET=1"), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	res, err := readFile(nil, readFileArgs{Path: ".env"})
	if err != nil {
		t.Fatalf("readFile: %v", err)
	}
	if !res.SecretWarning {
		t.Errorf("SecretWarning = false, want true for a .env filename")
	}
}

func TestReadFile_RejectsPathEscape(t *testing.T) {
	withTempCwd(t)
	if _, err := readFile(nil, readFileArgs{Path: "../../../etc/passwd"}); err == nil {
		t.Fatal("expected an error escaping the working directory, got nil")
	}
}

func TestReadFile_RejectsAbsolutePath(t *testing.T) {
	withTempCwd(t)
	if _, err := readFile(nil, readFileArgs{Path: "/etc/passwd"}); err == nil {
		t.Fatal("expected an error for an absolute path outside cwd, got nil")
	}
}

func TestWriteFile_RealWriteAndReadBack(t *testing.T) {
	dir := withTempCwd(t)

	res, err := writeFile(nil, writeFileArgs{Path: "sub/out.txt", Content: "written content"})
	if err != nil {
		t.Fatalf("writeFile: %v", err)
	}
	if res.Bytes != len("written content") {
		t.Errorf("Bytes = %d, want %d", res.Bytes, len("written content"))
	}

	got, err := os.ReadFile(filepath.Join(dir, "sub", "out.txt"))
	if err != nil {
		t.Fatalf("read back written file: %v", err)
	}
	if string(got) != "written content" {
		t.Errorf("file content = %q, want %q", string(got), "written content")
	}
}

func TestWriteFile_RejectsPathEscape(t *testing.T) {
	withTempCwd(t)
	if _, err := writeFile(nil, writeFileArgs{Path: "../escape.txt", Content: "x"}); err == nil {
		t.Fatal("expected an error escaping the working directory, got nil")
	}
}

// TestReadFile_RejectsSymlinkEscape is the regression test for the symlink
// confinement gap found in the 2026-07-18 harness audit: a symlink living
// inside the confined working directory but pointing outside it used to pass
// resolveConfinedPath's purely lexical check (Clean + Rel against cwd never
// looked at what the path actually resolved to on disk).
func TestReadFile_RejectsSymlinkEscape(t *testing.T) {
	outsideDir := t.TempDir()
	secretPath := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(secretPath, []byte("outside secret"), 0o644); err != nil {
		t.Fatalf("write outside secret file: %v", err)
	}

	dir := withTempCwd(t)
	linkPath := filepath.Join(dir, "link.txt")
	if err := os.Symlink(secretPath, linkPath); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	if _, err := readFile(nil, readFileArgs{Path: "link.txt"}); err == nil {
		t.Fatal("expected an error reading through a symlink escaping the working directory, got nil")
	}
}

// TestWriteFile_RejectsSymlinkDirEscape covers the write_file-specific case:
// the target file doesn't exist yet, but an ancestor *directory* is a symlink
// pointing outside cwd — resolveDeepestExistingRealPath must walk up to that
// symlinked directory (not just check the nonexistent final component) to
// catch this.
func TestWriteFile_RejectsSymlinkDirEscape(t *testing.T) {
	outsideDir := t.TempDir()

	dir := withTempCwd(t)
	linkDir := filepath.Join(dir, "linkdir")
	if err := os.Symlink(outsideDir, linkDir); err != nil {
		t.Fatalf("create symlink dir: %v", err)
	}

	if _, err := writeFile(nil, writeFileArgs{Path: "linkdir/new.txt", Content: "escaped"}); err == nil {
		t.Fatal("expected an error writing through a symlinked directory escaping the working directory, got nil")
	}
	if _, err := os.Stat(filepath.Join(outsideDir, "new.txt")); err == nil {
		t.Fatal("write escaped the working directory: file was created outside cwd")
	}
}

func TestWriteFile_Truncates(t *testing.T) {
	dir := withTempCwd(t)
	big := string(bytes.Repeat([]byte("y"), maxFileToolSize+1024))

	res, err := writeFile(nil, writeFileArgs{Path: "big-out.txt", Content: big})
	if err != nil {
		t.Fatalf("writeFile: %v", err)
	}
	if !res.Truncated {
		t.Errorf("Truncated = false, want true")
	}
	info, err := os.Stat(filepath.Join(dir, "big-out.txt"))
	if err != nil {
		t.Fatalf("stat written file: %v", err)
	}
	if info.Size() != int64(maxFileToolSize) {
		t.Errorf("written file size = %d, want %d", info.Size(), maxFileToolSize)
	}
}

package tools

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecCommand_BasicRun(t *testing.T) {
	res, err := execCommandImpl(context.Background(), execArgs{Command: "echo hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ExitCode != 0 {
		t.Errorf("exit code = %d, want 0", res.ExitCode)
	}
	if !strings.Contains(res.Output, "hello") {
		t.Errorf("output = %q, want it to contain %q", res.Output, "hello")
	}
	if res.TimedOut {
		t.Error("TimedOut = true, want false")
	}
}

func TestExecCommand_NonZeroExitCode(t *testing.T) {
	res, err := execCommandImpl(context.Background(), execArgs{Command: "exit 3"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ExitCode != 3 {
		t.Errorf("exit code = %d, want 3", res.ExitCode)
	}
}

func TestExecCommand_EmptyCommandRejected(t *testing.T) {
	if _, err := execCommandImpl(context.Background(), execArgs{Command: "   "}); err == nil {
		t.Fatal("expected error for empty command, got nil")
	}
}

func TestExecCommand_Timeout(t *testing.T) {
	res, err := execCommandImpl(context.Background(), execArgs{
		Command:        "sleep 5",
		TimeoutSeconds: 1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.TimedOut {
		t.Error("TimedOut = false, want true")
	}
	if res.ExitCode != -1 {
		t.Errorf("exit code = %d, want -1 (timed out)", res.ExitCode)
	}
}

func TestExecCommand_HugeTimeoutStillRunsFastCommand(t *testing.T) {
	// A requested timeout far beyond maxExecTimeout must be clamped, not
	// used as-is or rejected — just confirm the clamp path doesn't break a
	// fast, ordinary command.
	res, err := execCommandImpl(context.Background(), execArgs{
		Command:        "echo capped",
		TimeoutSeconds: 999999,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(res.Output, "capped") {
		t.Errorf("output = %q, want it to contain %q", res.Output, "capped")
	}
}

func TestExecCommand_EnvStripped(t *testing.T) {
	t.Setenv("EXEC_TEST_SECRET", "super-secret-value")
	res, err := execCommandImpl(context.Background(), execArgs{Command: "env"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(res.Output, "EXEC_TEST_SECRET") || strings.Contains(res.Output, "super-secret-value") {
		t.Errorf("subprocess env leaked a non-allowlisted variable: %q", res.Output)
	}
	// Sanity check the allowlist isn't stripping everything — PATH must
	// still be present or most real commands would fail to resolve.
	if !strings.Contains(res.Output, "PATH=") {
		t.Errorf("expected PATH to survive the allowlist, got: %q", res.Output)
	}
}

func TestExecCommand_OutputTruncated(t *testing.T) {
	// Print well over maxExecOutput bytes.
	res, err := execCommandImpl(context.Background(), execArgs{
		Command: "yes x | head -c 400000",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Truncated {
		t.Error("Truncated = false, want true for 400000 bytes of output")
	}
	if len(res.Output) > maxExecOutput {
		t.Errorf("output length = %d, want <= %d", len(res.Output), maxExecOutput)
	}
}

func TestExecCommand_RunsInWorkingDirectory(t *testing.T) {
	dir := withTempCwd(t) // shared helper from fs_test.go, same package
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", dir, err)
	}

	res, err := execCommandImpl(context.Background(), execArgs{Command: "pwd"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := strings.TrimSpace(res.Output)
	if got != realDir {
		t.Errorf("pwd output = %q, want %q", got, realDir)
	}
}

package tools

// exec.go implements exec_command — the harness's shell-execution tool.
//
// v1 scope (Q&A'd and confirmed with the user, see SESSION_HANDOFF.md):
//   - Human confirmation is the actual security boundary. functiontool.Config's
//     RequireConfirmation routes every call through ADK's native Human-in-
//     the-Loop flow (google.golang.org/adk/tool/toolconfirmation) — runExecCommand
//     below is only ever invoked once a human has approved via the TUI's y/n
//     prompt (cmd/tui/chat.go's startAgentStream/permissionPromptView). A
//     rejected call never reaches this file at all — the ADK framework
//     returns tool.ErrConfirmationRejected before the handler runs.
//   - The subprocess environment is allowlisted, not blacklisted: only a
//     small set of ordinary shell variables pass through (see
//     execEnvAllowlist), so an approved command can't read provider API keys
//     or other secrets via `env`, `printenv`, `curl $SOME_KEY`, etc. — even
//     ones exported after this list was written, since allowlisting defaults
//     closed.
//   - No OS-level sandboxing (chroot/container/seccomp). An approved command
//     can still read/write anything the OS user can (this process's cwd
//     confinement does not apply to what a subprocess itself chooses to
//     touch). This is a real, known ceiling of v1, not an oversight — the
//     human confirmation gate is the boundary; OS-level isolation is the
//     upgrade path if a stronger boundary is ever needed.
//   - Runs via `sh -c <command>` (full shell semantics: pipes/redirects/globs
//     work — chosen over an argv array per the user's own answer, since the
//     human is already the gate and shell semantics are more useful than the
//     narrower injection-surface reduction an argv array would buy).

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

// maxExecOutput caps combined stdout+stderr returned to the model — same
// 256 KiB convention as read_file/write_file (see maxFileToolSize in fs.go).
const maxExecOutput = 256 * 1024

// defaultExecTimeout / maxExecTimeout bound how long a command may run.
const (
	defaultExecTimeout = 120 * time.Second
	maxExecTimeout     = 600 * time.Second
)

// execEnvAllowlist is the only set of environment variables forwarded to the
// subprocess — see the package doc comment above for why this is an allowlist
// rather than a blacklist of known secret names.
// PWD is deliberately excluded: cmd.Dir already sets the subprocess's real
// working directory regardless of env, and forwarding the parent process's
// (possibly stale, since os.Chdir doesn't update it) $PWD could make a
// shell's `pwd` builtin report the wrong directory instead of the real one.
var execEnvAllowlist = []string{
	"PATH", "HOME", "USER", "SHELL", "TERM", "LANG", "LC_ALL", "TMPDIR",
}

func strippedExecEnv() []string {
	env := make([]string, 0, len(execEnvAllowlist))
	for _, k := range execEnvAllowlist {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	return env
}

// cappedWriter discards bytes past limit but always reports success so
// os/exec never sees a write error mid-stream; truncated records whether
// anything was actually dropped. Safe for concurrent use since cmd.Stdout and
// cmd.Stderr write from separate goroutines when both are set.
type cappedWriter struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func (c *cappedWriter) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	remain := c.limit - c.buf.Len()
	if remain <= 0 {
		c.truncated = true
		return len(p), nil
	}
	if len(p) > remain {
		c.buf.Write(p[:remain])
		c.truncated = true
		return len(p), nil
	}
	c.buf.Write(p)
	return len(p), nil
}

func (c *cappedWriter) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.String()
}

func (c *cappedWriter) Truncated() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.truncated
}

type execArgs struct {
	Command        string `json:"command" jsonschema:"Shell command to run via 'sh -c', in the working directory. Requires human approval before it runs."`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty" jsonschema:"Optional timeout in seconds (default 120, max 600)."`
}

type execResult struct {
	Command   string `json:"command"`
	ExitCode  int    `json:"exit_code"`
	Output    string `json:"output"`
	Truncated bool   `json:"truncated"`
	TimedOut  bool   `json:"timed_out"`
	Message   string `json:"message"`
}

// runExecCommand is the functiontool handler — its signature is fixed by
// functiontool.New's Func[TArgs, TResults] type. It forwards straight to
// execCommandImpl (ctx, a tool.Context, satisfies context.Context via
// agent.ReadonlyContext's embedding), which takes a plain context.Context so
// it's callable directly from tests with context.Background() — no need to
// hand-fake the rest of the much larger tool.Context interface.
func runExecCommand(ctx tool.Context, args execArgs) (execResult, error) {
	return execCommandImpl(ctx, args)
}

func execCommandImpl(ctx context.Context, args execArgs) (execResult, error) {
	if strings.TrimSpace(args.Command) == "" {
		return execResult{}, fmt.Errorf("command must not be empty")
	}

	timeout := defaultExecTimeout
	if args.TimeoutSeconds > 0 {
		timeout = time.Duration(args.TimeoutSeconds) * time.Second
		if timeout > maxExecTimeout {
			timeout = maxExecTimeout
		}
	}

	cwd, err := os.Getwd()
	if err != nil {
		return execResult{}, fmt.Errorf("resolve cwd: %w", err)
	}

	// ctx (tool.Context) embeds context.Context (via agent.ReadonlyContext),
	// so a cancel from the TUI's own interrupt key propagates here too —
	// distinct from the fixed-timeout convention other tools use (read_file,
	// fetch_url), which is fine for their fast, bounded operations but not
	// for a shell command that could legitimately run for a while.
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, "sh", "-c", args.Command)
	cmd.Dir = cwd
	cmd.Env = strippedExecEnv()

	out := &cappedWriter{limit: maxExecOutput}
	cmd.Stdout = out
	cmd.Stderr = out

	runErr := cmd.Run()
	timedOut := errors.Is(runCtx.Err(), context.DeadlineExceeded)

	exitCode := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		switch {
		case errors.As(runErr, &exitErr):
			exitCode = exitErr.ExitCode()
		case timedOut:
			exitCode = -1
		default:
			return execResult{}, fmt.Errorf("run %q: %w", args.Command, runErr)
		}
	}

	slog.Info("tool_call", "kind", "ToolCall", "tool", "exec_command", "command", args.Command, "exit_code", exitCode, "timed_out", timedOut)

	msg := fmt.Sprintf("Ran %q — exit code %d.", args.Command, exitCode)
	if timedOut {
		msg = fmt.Sprintf("Ran %q — timed out after %s.", args.Command, timeout)
	}
	if out.Truncated() {
		msg += fmt.Sprintf(" Output truncated at %d bytes.", maxExecOutput)
	}

	return execResult{
		Command:   args.Command,
		ExitCode:  exitCode,
		Output:    out.String(),
		Truncated: out.Truncated(),
		TimedOut:  timedOut,
		Message:   msg,
	}, nil
}

// NewExecCommandTool creates the exec_command FunctionTool. Every call
// requires human approval via ADK's native Human-in-the-Loop confirmation
// flow (RequireConfirmation) before runExecCommand is ever invoked — see the
// package doc comment above for the full v1 security-scope rationale.
func NewExecCommandTool() tool.Tool {
	t, err := functiontool.New(functiontool.Config{
		Name: "exec_command",
		Description: "Runs a shell command (via 'sh -c') in the working directory. " +
			"Requires explicit human approval before every run — the human confirmation " +
			"step is the security boundary, there is no OS-level sandboxing. " +
			"Subprocess environment is stripped to a small allowlist (PATH/HOME/etc.) " +
			"so provider API keys and other secrets are never exposed to the command. " +
			"Output capped at 256 KiB combined stdout+stderr; timeout default 120s, max 600s.",
		RequireConfirmation: true,
	}, runExecCommand)
	if err != nil {
		panic(fmt.Sprintf("NewExecCommandTool: %v", err))
	}
	return t
}

package tools

// fs.go implements read_file / write_file — the harness's filesystem tools.
//
// Both are confined to the process's current working directory: an argument
// path is resolved with filepath.Abs against os.Getwd() and rejected if it
// resolves outside that root. This is a deliberate, cheap guardrail (not
// present on the existing @path attachment reader, which was judged a
// non-issue for a local single-user CLI with no privilege boundary) — write
// access is a different risk class than read, since a hallucinating LLM
// tool call has no human confirmation step in this architecture, so an
// unconfined write_file could clobber anything the OS user can write to.
// Confining new tools costs nothing and closes that gap for new code.

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

// maxFileToolSize caps how much read_file/write_file will read or accept in
// one call — same 256 KiB convention as cmd/tui/attachments.go's
// maxAttachmentSize, for the same reason (bound token cost per call).
const maxFileToolSize = 256 * 1024

// secretNamePatterns flags filenames that commonly hold credentials. This is
// a warning, not a block — matching the project's existing judgment that
// path/content restrictions beyond this are unnecessary for a local
// single-user CLI (see SESSION_HANDOFF.md's attachment secret-detection
// finding, which recommended exactly this pattern-warning approach).
var secretNamePatterns = []string{
	".env", "id_rsa", "id_ed25519", "credentials.json", ".pem", ".key",
	"secret", "token", ".npmrc", ".netrc", "shadow",
}

// resolveConfinedPath resolves p against the process cwd and rejects any
// path (absolute, or relative with a ".." escape) that lands outside it.
//
// A lexical check alone (Clean + Rel against cwd) is not enough: a symlink
// living inside the working directory but pointing outside it (or a cwd that
// is itself a symlink) passes the lexical check while the actual file the OS
// opens/writes is outside the confined root. So after the lexical check we
// also resolve the real (symlink-free) path of the deepest existing ancestor
// — walking up from the target since write_file's target commonly doesn't
// exist yet — and verify *that* stays inside the real (symlink-free) cwd.
func resolveConfinedPath(p string) (string, error) {
	if p == "" {
		return "", fmt.Errorf("path must not be empty")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve cwd: %w", err)
	}
	realCwd, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		return "", fmt.Errorf("resolve cwd: %w", err)
	}

	abs := p
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(cwd, abs)
	}
	abs = filepath.Clean(abs)

	rel, err := filepath.Rel(cwd, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q resolves outside the working directory — refusing", p)
	}

	realAbs, err := resolveDeepestExistingRealPath(abs)
	if err != nil {
		return "", fmt.Errorf("resolve path %q: %w", p, err)
	}
	realRel, err := filepath.Rel(realCwd, realAbs)
	if err != nil || realRel == ".." || strings.HasPrefix(realRel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q resolves outside the working directory via a symlink — refusing", p)
	}

	return abs, nil
}

// resolveDeepestExistingRealPath returns abs with every symlink resolved,
// including symlinks in ancestor directories that exist even when abs itself
// (the final path component) does not — the common case for write_file
// creating a new file inside an existing directory.
func resolveDeepestExistingRealPath(abs string) (string, error) {
	dir := abs
	var suffix []string
	for {
		if _, err := os.Lstat(dir); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no existing ancestor directory found")
		}
		suffix = append([]string{filepath.Base(dir)}, suffix...)
		dir = parent
	}
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", err
	}
	return filepath.Join(append([]string{realDir}, suffix...)...), nil
}

func looksLikeSecretName(p string) bool {
	base := strings.ToLower(filepath.Base(p))
	for _, pat := range secretNamePatterns {
		if strings.Contains(base, pat) {
			return true
		}
	}
	return false
}

// ── read_file ───────────────────────────────────────────────────────────────

type readFileArgs struct {
	Path string `json:"path" jsonschema:"Path to the file to read, relative to the working directory."`
}

type readFileResult struct {
	Path          string `json:"path"`
	Content       string `json:"content"`
	Truncated     bool   `json:"truncated"`
	SecretWarning bool   `json:"secret_warning"`
	Message       string `json:"message"`
}

func readFile(_ tool.Context, args readFileArgs) (readFileResult, error) {
	abs, err := resolveConfinedPath(args.Path)
	if err != nil {
		return readFileResult{}, err
	}
	f, err := os.Open(abs)
	if err != nil {
		return readFileResult{}, fmt.Errorf("open %q: %w", args.Path, err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return readFileResult{}, fmt.Errorf("stat %q: %w", args.Path, err)
	}
	if info.IsDir() {
		return readFileResult{}, fmt.Errorf("%q is a directory, not a file", args.Path)
	}

	buf := make([]byte, minInt(int(info.Size()), maxFileToolSize))
	n, err := f.Read(buf)
	if err != nil && n == 0 {
		return readFileResult{}, fmt.Errorf("read %q: %w", args.Path, err)
	}
	truncated := info.Size() > int64(maxFileToolSize)
	warning := looksLikeSecretName(args.Path)
	if warning {
		slog.Warn("read_file: secret-pattern filename", "path", args.Path)
	}
	slog.Info("tool_call", "kind", "ToolCall", "tool", "read_file", "path", args.Path, "bytes", n)

	msg := fmt.Sprintf("Read %d bytes from %q.", n, args.Path)
	if truncated {
		msg += fmt.Sprintf(" Truncated at %d bytes (file is %d bytes).", maxFileToolSize, info.Size())
	}
	if warning {
		msg += " WARNING: filename matches a common credential-file pattern — treat contents as sensitive."
	}

	return readFileResult{
		Path:          args.Path,
		Content:       string(buf[:n]),
		Truncated:     truncated,
		SecretWarning: warning,
		Message:       msg,
	}, nil
}

// NewReadFileTool creates the read_file FunctionTool.
func NewReadFileTool() tool.Tool {
	t, err := functiontool.New(functiontool.Config{
		Name:        "read_file",
		Description: "Reads a text file from the working directory (confined — cannot escape it via absolute paths or '..'). Capped at 256 KiB; larger files are truncated. Warns if the filename matches a common credential-file pattern.",
	}, readFile)
	if err != nil {
		panic(fmt.Sprintf("NewReadFileTool: %v", err))
	}
	return t
}

// ── write_file ──────────────────────────────────────────────────────────────

type writeFileArgs struct {
	Path    string `json:"path" jsonschema:"Path to write, relative to the working directory."`
	Content string `json:"content" jsonschema:"Full file content to write. Capped at 256 KiB."`
}

type writeFileResult struct {
	Path      string `json:"path"`
	Bytes     int    `json:"bytes"`
	Truncated bool   `json:"truncated"`
	Message   string `json:"message"`
}

func writeFile(_ tool.Context, args writeFileArgs) (writeFileResult, error) {
	abs, err := resolveConfinedPath(args.Path)
	if err != nil {
		return writeFileResult{}, err
	}
	content := args.Content
	truncated := false
	if len(content) > maxFileToolSize {
		content = content[:maxFileToolSize]
		truncated = true
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return writeFileResult{}, fmt.Errorf("create parent dirs for %q: %w", args.Path, err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		return writeFileResult{}, fmt.Errorf("write %q: %w", args.Path, err)
	}

	slog.Info("tool_call", "kind", "ToolCall", "tool", "write_file", "path", args.Path, "bytes", len(content))

	msg := fmt.Sprintf("Wrote %d bytes to %q.", len(content), args.Path)
	if truncated {
		msg += " Content was truncated to 256 KiB before writing."
	}
	return writeFileResult{Path: args.Path, Bytes: len(content), Truncated: truncated, Message: msg}, nil
}

// NewWriteFileTool creates the write_file FunctionTool.
func NewWriteFileTool() tool.Tool {
	t, err := functiontool.New(functiontool.Config{
		Name:        "write_file",
		Description: "Writes a text file in the working directory (confined — cannot escape it via absolute paths or '..'). Creates parent directories as needed. Capped at 256 KiB.",
	}, writeFile)
	if err != nil {
		panic(fmt.Sprintf("NewWriteFileTool: %v", err))
	}
	return t
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

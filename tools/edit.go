package tools

// edit.go implements edit_file — a targeted find-and-replace tool, the
// harness's missing piece flagged by the 2026-07-18 completeness audit:
// write_file only supports full-file overwrite, so any change to part of an
// existing file requires the model to reproduce the entire file from
// scratch, risking silent truncation/corruption of the untouched parts.
//
// Semantics mirror Claude Code's own Edit tool deliberately (same trust
// model this harness's operator already relies on): old_string must match
// the file's exact current content and, by default, occur exactly once —
// ambiguous matches are refused rather than guessed at, forcing the caller
// to add more surrounding context or explicitly opt into replace_all.

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type editFileArgs struct {
	Path       string `json:"path" jsonschema:"Path to the file to edit, relative to the working directory."`
	OldString  string `json:"old_string" jsonschema:"Exact text to find, including whitespace. Must match the file's current content."`
	NewString  string `json:"new_string" jsonschema:"Text to replace old_string with."`
	ReplaceAll bool   `json:"replace_all,omitempty" jsonschema:"Replace every occurrence instead of requiring exactly one match. Defaults to false."`
}

type editFileResult struct {
	Path     string `json:"path"`
	Replaced int    `json:"replaced"`
	Bytes    int    `json:"bytes"`
	Message  string `json:"message"`
}

func editFile(_ tool.Context, args editFileArgs) (editFileResult, error) {
	if args.OldString == "" {
		return editFileResult{}, fmt.Errorf("old_string must not be empty")
	}
	if args.OldString == args.NewString {
		return editFileResult{}, fmt.Errorf("old_string and new_string are identical — nothing to change")
	}

	abs, err := resolveConfinedPath(args.Path)
	if err != nil {
		return editFileResult{}, err
	}

	info, err := os.Stat(abs)
	if err != nil {
		return editFileResult{}, fmt.Errorf("stat %q: %w", args.Path, err)
	}
	if info.IsDir() {
		return editFileResult{}, fmt.Errorf("%q is a directory, not a file", args.Path)
	}
	// Unlike read_file, a partial read here would risk writing back a
	// truncated file — refuse outright rather than operate on incomplete
	// content.
	if info.Size() > maxFileToolSize {
		return editFileResult{}, fmt.Errorf("%q is %d bytes, over the %d-byte edit_file cap — refusing to risk a truncated rewrite", args.Path, info.Size(), maxFileToolSize)
	}

	raw, err := os.ReadFile(abs)
	if err != nil {
		return editFileResult{}, fmt.Errorf("read %q: %w", args.Path, err)
	}
	content := string(raw)

	count := strings.Count(content, args.OldString)
	if count == 0 {
		return editFileResult{}, fmt.Errorf("old_string not found in %q — it must match the file's exact current content, including whitespace", args.Path)
	}
	if count > 1 && !args.ReplaceAll {
		return editFileResult{}, fmt.Errorf("old_string matches %d times in %q — add more surrounding context to make it unique, or set replace_all=true to replace every occurrence", count, args.Path)
	}

	var newContent string
	replaced := count
	if args.ReplaceAll {
		newContent = strings.ReplaceAll(content, args.OldString, args.NewString)
	} else {
		newContent = strings.Replace(content, args.OldString, args.NewString, 1)
		replaced = 1
	}

	if len(newContent) > maxFileToolSize {
		return editFileResult{}, fmt.Errorf("edit would grow %q to %d bytes, over the %d-byte cap — refusing", args.Path, len(newContent), maxFileToolSize)
	}

	if err := os.WriteFile(abs, []byte(newContent), 0o644); err != nil {
		return editFileResult{}, fmt.Errorf("write %q: %w", args.Path, err)
	}

	slog.Info("tool_call", "kind", "ToolCall", "tool", "edit_file", "path", args.Path, "replaced", replaced, "bytes", len(newContent))

	msg := fmt.Sprintf("Replaced %d occurrence(s) in %q (%d bytes).", replaced, args.Path, len(newContent))
	return editFileResult{Path: args.Path, Replaced: replaced, Bytes: len(newContent), Message: msg}, nil
}

// NewEditFileTool creates the edit_file FunctionTool.
func NewEditFileTool() tool.Tool {
	t, err := functiontool.New(functiontool.Config{
		Name: "edit_file",
		Description: "Makes a targeted find-and-replace edit to an existing file (confined — cannot escape the working directory). " +
			"old_string must match the file's exact current content and, by default, occur exactly once; set replace_all to replace every occurrence. " +
			"Prefer this over write_file when changing only part of an existing file.",
	}, editFile)
	if err != nil {
		panic(fmt.Sprintf("NewEditFileTool: %v", err))
	}
	return t
}

package tools

// search.go implements grep_search — a pure-Go, no-shell-out text search
// tool. Using regexp (Go's RE2 engine) + filepath.WalkDir instead of
// shelling out to `grep`/`rg` avoids any command-injection surface entirely
// (no string-built shell command exists to inject into) and RE2 has no
// catastrophic-backtracking (ReDoS) failure mode by construction.

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

// maxGrepResults caps how many matches grep_search returns — never a silent
// cap: the result always reports whether it truncated.
const maxGrepResults = 200

var grepSkipDirs = map[string]bool{
	".git": true, "vendor": true, "node_modules": true, ".arch": true,
}

type grepArgs struct {
	Pattern string `json:"pattern" jsonschema:"Regular expression (RE2 syntax) to search for."`
	Path    string `json:"path,omitempty" jsonschema:"Directory to search, relative to the working directory. Defaults to '.'."`
}

type grepMatch struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

type grepResult struct {
	Matches      []grepMatch `json:"matches"`
	Count        int         `json:"count"`
	Truncated    bool        `json:"truncated"`
	SkippedFiles []string    `json:"skipped_files,omitempty"`
	Message      string      `json:"message"`
}

func grepSearch(_ tool.Context, args grepArgs) (grepResult, error) {
	if strings.TrimSpace(args.Pattern) == "" {
		return grepResult{}, fmt.Errorf("pattern must not be empty")
	}
	re, err := regexp.Compile(args.Pattern)
	if err != nil {
		return grepResult{}, fmt.Errorf("invalid pattern %q: %w", args.Pattern, err)
	}

	root := args.Path
	if root == "" {
		root = "."
	}
	absRoot, err := resolveConfinedPath(root)
	if err != nil {
		return grepResult{}, err
	}

	var matches []grepMatch
	var skipped []string
	truncated := false

	walkErr := filepath.WalkDir(absRoot, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries, don't abort the whole walk
		}
		if d.IsDir() {
			if grepSkipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if truncated {
			return nil
		}
		f, openErr := os.Open(p)
		if openErr != nil {
			return nil
		}
		defer f.Close()

		rel, _ := filepath.Rel(absRoot, p)
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			line := scanner.Text()
			if re.MatchString(line) {
				matches = append(matches, grepMatch{Path: rel, Line: lineNo, Text: strings.TrimSpace(line)})
				if len(matches) >= maxGrepResults {
					truncated = true
					return nil
				}
			}
		}
		// scanner.Scan() also returns false on a real error (e.g. a line
		// exceeding the 1 MiB buffer) — without checking Err(), that file
		// silently contributes zero matches, indistinguishable from
		// "genuinely no matches in this file". Surface it instead.
		if scanErr := scanner.Err(); scanErr != nil {
			skipped = append(skipped, rel)
			slog.Warn("grep_search: file scan error, results may be incomplete", "path", rel, "error", scanErr)
		}
		return nil
	})
	if walkErr != nil {
		return grepResult{}, fmt.Errorf("search %q: %w", root, walkErr)
	}

	slog.Info("tool_call", "kind", "ToolCall", "tool", "grep_search", "pattern", args.Pattern, "path", root, "matches", len(matches))

	msg := fmt.Sprintf("Found %d match(es) for %q under %q.", len(matches), args.Pattern, root)
	if truncated {
		msg += fmt.Sprintf(" Truncated at %d results.", maxGrepResults)
	}
	if len(skipped) > 0 {
		msg += fmt.Sprintf(" WARNING: %d file(s) could not be fully scanned (e.g. a line too long) and may be missing matches: %s.",
			len(skipped), strings.Join(skipped, ", "))
	}
	return grepResult{Matches: matches, Count: len(matches), Truncated: truncated, SkippedFiles: skipped, Message: msg}, nil
}

// NewGrepTool creates the grep_search FunctionTool.
func NewGrepTool() tool.Tool {
	t, err := functiontool.New(functiontool.Config{
		Name:        "grep_search",
		Description: "Searches text files under a directory (default: working directory) for lines matching a regular expression (RE2 syntax). Confined to the working directory. Capped at 200 results.",
	}, grepSearch)
	if err != nil {
		panic(fmt.Sprintf("NewGrepTool: %v", err))
	}
	return t
}

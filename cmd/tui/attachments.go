package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"go-adk-q/cmd/tui/theme"

	"github.com/charmbracelet/lipgloss"
)

// maxAttachmentSize caps how much of a single @path or /filepicker-selected
// file is read into the agent's context. Without a cap, attaching one large
// log/data file could balloon a single turn's prompt far past the model's
// context window and silently multiply token cost (fallback audit F10).
const maxAttachmentSize = 256 * 1024 // 256 KiB — generous for source/config files

// splitAttachmentsBySize stats each path and separates it into ok (safe to
// read) and oversized (skipped, size in bytes exceeds maxAttachmentSize).
// Paths that fail to stat are treated as ok — the existing read path already
// handles a file disappearing between resolution and read.
func splitAttachmentsBySize(paths []string) (ok []string, oversized []string) {
	for _, p := range paths {
		info, err := os.Stat(p)
		if err == nil && info.Size() > maxAttachmentSize {
			oversized = append(oversized, filepath.Base(p))
			continue
		}
		ok = append(ok, p)
	}
	return ok, oversized
}

// processInputForFilesAndTags processes user input to handle file attachments (@path) and tags (#tag)
// processInputForFilesAndTags scans the input for @path/to/file tokens and
// #tag tokens.  It returns:
//   - processedText: the message with @path replaced by "[Attached: filename]"
//   - filePaths:     resolved file paths (NOT contents) for every readable @path
//   - missingPaths:  @path tokens whose file could NOT be found (shown as warning)
//   - tags:          list of #tag values
func processInputForFilesAndTags(input string) (processedText string, filePaths []string, missingPaths []string, tags []string) {
	var messageText strings.Builder

	words := strings.Fields(input)
	for _, word := range words {
		if strings.HasPrefix(word, "@") && len(word) > 1 {
			filePath := strings.TrimPrefix(word, "@")
			if _, err := os.Stat(filePath); err == nil {
				filePaths = append(filePaths, filePath)
				messageText.WriteString("[Attached: " + filepath.Base(filePath) + "] ")
			} else {
				// File not found — record for error display and keep raw token.
				missingPaths = append(missingPaths, filePath)
				messageText.WriteString(word + " ")
			}
		} else if strings.HasPrefix(word, "#") && len(word) > 1 {
			tag := strings.TrimPrefix(word, "#")
			tags = append(tags, tag)
			messageText.WriteString("#" + tag + " ")
		} else {
			messageText.WriteString(word + " ")
		}
	}

	result := strings.TrimSpace(messageText.String())
	if result == "" {
		result = input
	}
	return result, filePaths, missingPaths, tags
}

// ── @ file autocomplete ────────────────────────────────────────────────────────

// skipAtFileDirs is the set of directory names always excluded when scanning
// the repository for file suggestions.
var skipAtFileDirs = map[string]bool{
	".git": true, ".hg": true, ".svn": true,
	"node_modules": true, "vendor": true, ".vendor": true,
	"dist": true, "build": true, "out": true, "bin": true,
	".cache": true, "__pycache__": true, ".venv": true, "venv": true,
	".idea": true, ".vscode": true,
}

// loadAtFileItems walks cwd up to depth 5 and returns all non-directory
// file paths as paths relative to cwd.  Hidden files/dirs (leading dot)
// and skipAtFileDirs are excluded.  Returns at most 1 000 entries.
func loadAtFileItems(cwd string) []string {
	const maxItems = 1000
	const maxDepth = 5
	var items []string

	_ = filepath.WalkDir(cwd, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, relErr := filepath.Rel(cwd, path)
		if relErr != nil || rel == "." {
			return nil
		}
		name := d.Name()
		// Skip hidden files and excluded directories.
		if strings.HasPrefix(name, ".") || skipAtFileDirs[name] {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			// Enforce depth limit.
			depth := strings.Count(rel, string(filepath.Separator)) + 1
			if depth >= maxDepth {
				return filepath.SkipDir
			}
			return nil
		}
		items = append(items, rel)
		if len(items) >= maxItems {
			return fs.SkipAll
		}
		return nil
	})
	return items
}

// filterAtFileItems returns up to maxShow items from items whose path
// contains filter (case-insensitive).  When filter is empty the first
// maxShow items are returned, with top-level files listed first.
func filterAtFileItems(items []string, filter string) []string {
	const maxShow = 14
	if filter == "" {
		// Surface top-level files before nested ones.
		var top, rest []string
		for _, it := range items {
			if !strings.Contains(it, string(filepath.Separator)) {
				top = append(top, it)
			} else {
				rest = append(rest, it)
			}
		}
		all := append(top, rest...)
		if len(all) > maxShow {
			return all[:maxShow]
		}
		return all
	}
	lower := strings.ToLower(filter)
	var out []string
	for _, it := range items {
		if strings.Contains(strings.ToLower(it), lower) {
			out = append(out, it)
			if len(out) >= maxShow {
				break
			}
		}
	}
	return out
}

// extractAtFilter detects whether the textarea value ends with an
// \"@<filter>\" token (no trailing space) and returns the filter text.
// Used to decide whether to open the @ autocomplete menu.
func extractAtFilter(val string) (filter string, found bool) {
	if val == "" {
		return "", false
	}
	// Find the last @ in the value.
	idx := strings.LastIndex(val, "@")
	if idx < 0 {
		return "", false
	}
	after := val[idx+1:]
	// If there is any whitespace after the @, the token is finished.
	if strings.ContainsAny(after, " \t\n") {
		return "", false
	}
	return after, true
}

// replaceAtFilter replaces the last \"@<filter>\" token in val with
// \"@selectedFile\", preserving everything before the @.
func replaceAtFilter(val, selectedFile string) string {
	idx := strings.LastIndex(val, "@")
	if idx < 0 {
		return val + "@" + selectedFile + " "
	}
	before := val[:idx]
	after := val[idx+1:]
	// Drop the partial filter (everything up to next space or end).
	if spaceIdx := strings.IndexAny(after, " \t\n"); spaceIdx >= 0 {
		return before + "@" + selectedFile + after[spaceIdx:]
	}
	// Nothing typed after the mention yet — add a trailing space so
	// whatever the user types next doesn't glue onto the filename.
	return before + "@" + selectedFile + " "
}

// atFileMenuView renders the @ file autocomplete popup, styled like the
// slash command menu.  It floats just above the input separator.
func atFileMenuView(s theme.StyledSet, items []string, selectedIdx int, filter string, w int) string {
	if len(items) == 0 {
		return ""
	}
	if selectedIdx < 0 {
		selectedIdx = 0
	}
	if selectedIdx >= len(items) {
		selectedIdx = len(items) - 1
	}

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(s.Sep.GetForeground()).
		Width(w - 2)

	rowW := w - 4
	if rowW < 12 {
		rowW = 12
	}

	// Header: show the current filter and item count.
	var sb strings.Builder
	headerText := fmt.Sprintf("  📂 @%s  •  %d file(s)  •  ↑↓ navigate  •  tab: select  •  esc: cancel", filter, len(items))
	sb.WriteString(s.System.Width(rowW).Render(headerText) + "\n")

	for i, item := range items {
		// Truncate long paths so the row fits.
		display := item
		if lipgloss.Width(display) > rowW-4 {
			display = "…" + display[len(display)-(rowW-5):]
		}
		row := "  " + display
		if i == selectedIdx {
			sb.WriteString(s.Prompt.Width(rowW).Render(row))
		} else {
			sb.WriteString(s.System.Width(rowW).Render(row))
		}
		if i < len(items)-1 {
			sb.WriteByte('\n')
		}
	}
	return boxStyle.Render(sb.String()) + "\n"
}

// smartCopy picks what to copy from an agent reply:
//   - If the text contains one or more code fences, the body of the first
//     fence is copied (without the ``` delimiters).
//   - Otherwise the full reply text is copied.
//
// Returns the string to copy and a short status label.
func smartCopy(text string) (content, statusLabel string) {
	for _, seg := range parseSegments(text) {
		if seg.code {
			return seg.body, "Copied code block"
		}
	}
	return text, "Copied reply"
}

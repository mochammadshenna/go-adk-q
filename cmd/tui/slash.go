package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// slashCmd is one entry in the autocomplete menu.
type slashCmd struct {
	name string // e.g. "/settings"
	desc string // short description shown in the menu
}

// allSlashCmds is the canonical ordered list of slash commands.
var allSlashCmds = []slashCmd{
	{"/settings", "Open settings (theme, char limit)"},
	{"/model", "Switch model or provider"},
	{"/theme", "Cycle colour theme"},
	{"/help", "Toggle help overlay"},
	{"/clear", "Clear conversation history"},
	{"/skills", "List available agent skills"},
}

// slashMatches returns the subset of allSlashCmds whose name has prefix as a
// prefix.  prefix must start with '/'.
func slashMatches(prefix string) []slashCmd {
	prefix = strings.ToLower(prefix)
	var out []slashCmd
	for _, c := range allSlashCmds {
		if strings.HasPrefix(c.name, prefix) {
			out = append(out, c)
		}
	}
	return out
}

// slashMenuVisible returns true when the input value starts with '/' and there
// are matching commands to show.
func slashMenuVisible(inputVal string) bool {
	if !strings.HasPrefix(inputVal, "/") {
		return false
	}
	return len(slashMatches(inputVal)) > 0
}

// slashMenuView renders the autocomplete popup that floats just above the
// input separator.  It is constrained to width w.
func slashMenuView(s styledSet, matches []slashCmd, selectedIdx int, w int) string {
	if len(matches) == 0 {
		return ""
	}

	if selectedIdx >= len(matches) {
		selectedIdx = len(matches) - 1
	}
	if selectedIdx < 0 {
		selectedIdx = 0
	}

	maxName := 0
	for _, c := range matches {
		if len(c.name) > maxName {
			maxName = len(c.name)
		}
	}

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(s.sep.GetForeground()).
		Width(w - 2)

	rowW := w - 4
	if rowW < 10 {
		rowW = 10
	}

	var sb strings.Builder
	for i, c := range matches {
		nameCol := c.name + strings.Repeat(" ", maxName-len(c.name))
		descCol := c.desc

		maxDesc := rowW - maxName - 3
		if maxDesc < 0 {
			maxDesc = 0
		}
		if len(descCol) > maxDesc {
			descCol = descCol[:maxDesc]
		}

		row := "  " + nameCol + "   " + descCol

		if i == selectedIdx {
			sb.WriteString(s.prompt.Width(rowW).Render(row))
		} else {
			sb.WriteString(s.system.Width(rowW).Render(row))
		}
		if i < len(matches)-1 {
			sb.WriteByte('\n')
		}
	}

	return boxStyle.Render(sb.String()) + "\n"
}

// ── /skills command helpers ───────────────────────────────────────────────────

// skillEntry is a parsed entry from a skill's SKILL.md front matter.
type skillEntry struct {
	name string
	desc string
}

// parseSkillFrontMatter extracts name: and description: from the YAML front
// matter block (between the first two --- lines) of a SKILL.md file.
func parseSkillFrontMatter(data []byte) skillEntry {
	text := string(data)
	if !strings.HasPrefix(text, "---") {
		return skillEntry{}
	}
	rest := text[3:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return skillEntry{}
	}
	block := rest[:end]

	var entry skillEntry
	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "name:") {
			entry.name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
		}
		if strings.HasPrefix(line, "description:") {
			raw := strings.TrimSpace(strings.TrimPrefix(line, "description:"))
			// Strip optional surrounding quotes.
			raw = strings.Trim(raw, `"'`)
			entry.desc = raw
		}
	}
	return entry
}

// skillCategory groups skills under a named category.
type skillCategory struct {
	name    string
	entries []skillEntry
}

// readSkillCategories scans dir for category subdirectories, each containing
// skill subdirectories with a SKILL.md file. Structure:
//
//	dir/<category>/<skill>/SKILL.md
//
// Categories or skills without any readable SKILL.md are silently skipped.
func readSkillCategories(dir string) ([]skillCategory, error) {
	cats, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var result []skillCategory
	for _, cat := range cats {
		if !cat.IsDir() {
			continue
		}
		catPath := filepath.Join(dir, cat.Name())
		skills, err := os.ReadDir(catPath)
		if err != nil {
			continue
		}
		var entries []skillEntry
		for _, sk := range skills {
			if !sk.IsDir() {
				continue
			}
			path := filepath.Join(catPath, sk.Name(), "SKILL.md")
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			e := parseSkillFrontMatter(data)
			if e.name == "" {
				e.name = sk.Name()
			}
			entries = append(entries, e)
		}
		if len(entries) > 0 {
			result = append(result, skillCategory{name: cat.Name(), entries: entries})
		}
	}
	return result, nil
}

// readSkillEntries scans dir for skill entries using the two-level category
// structure (dir/<category>/<skill>/SKILL.md) and returns a flat list.
func readSkillEntries(dir string) ([]skillEntry, error) {
	cats, err := readSkillCategories(dir)
	if err != nil {
		return nil, err
	}
	var entries []skillEntry
	for _, c := range cats {
		entries = append(entries, c.entries...)
	}
	return entries, nil
}

// listSkillsSummary reads ./skills/ and returns a formatted multi-line string
// listing each skill grouped by category, used by the /skills slash command.
func listSkillsSummary() string {
	cats, err := readSkillCategories("skills")
	if err != nil || len(cats) == 0 {
		return "No skills found in ./skills/ — add category/skill directories with SKILL.md files."
	}

	total := 0
	for _, c := range cats {
		total += len(c.entries)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Available skills (%d):\n", total))

	for _, c := range cats {
		sb.WriteString(fmt.Sprintf("\n**%s**\n", c.name))

		maxName := 0
		for _, e := range c.entries {
			if len(e.name) > maxName {
				maxName = len(e.name)
			}
		}
		for _, e := range c.entries {
			pad := strings.Repeat(" ", maxName-len(e.name))
			sb.WriteString(fmt.Sprintf("  %s%s   %s\n", e.name, pad, e.desc))
		}
	}

	sb.WriteString("\nUse 'load_skill <name>' to activate a skill in this session.")
	return sb.String()
}

package main

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
)

func TestCodeBlockWidth(t *testing.T) {
	border := lipgloss.Color("#585b70")
	bg := lipgloss.Color("#181825")
	fg := lipgloss.Color("#cdd6f4")

	const boxW = 60
	output := renderCodeBlock("go", "func main() {\n\tfmt.Println(\"Hello\")\n}", boxW, border, bg, fg)

	for i, line := range strings.Split(strings.TrimRight(output, "\n"), "\n") {
		// Strip ANSI codes for visual width check
		plain := stripANSI(line)
		// Each line is prefixed with 2-space indent, so strip that
		plain = strings.TrimPrefix(plain, "  ")
		w := utf8.RuneCountInString(plain)
		// boxW includes 2 border chars, so each row inside = boxW chars
		if w != boxW {
			t.Errorf("line %d width = %d, want %d: %q", i, w, boxW, plain)
		}
	}
}

// stripANSI removes ANSI escape codes (crude but sufficient for test).
func stripANSI(s string) string {
	var out strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			// Skip until 'm'
			for i < len(s) && s[i] != 'm' {
				i++
			}
			i++ // skip 'm'
		} else {
			out.WriteByte(s[i])
			i++
		}
	}
	return out.String()
}

func TestTopBorder(t *testing.T) {
	cases := []struct {
		lang   string
		innerW int
	}{
		{"go", 58},
		{"python", 58},
		{"", 58},
		{"javascript", 20},
	}
	for _, c := range cases {
		top := codeTopBorder(c.lang, c.innerW)
		// Strip ANSI
		plain := stripANSI(top)
		got := utf8.RuneCountInString(plain)
		want := c.innerW + 2 // innerW + 2 corner chars
		if got != want {
			t.Errorf("codeTopBorder(%q, %d): len=%d, want %d: %q", c.lang, c.innerW, got, want, plain)
		}
	}
}

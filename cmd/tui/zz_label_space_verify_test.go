package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"

	"go-adk-q/cmd/tui/theme"
)

func TestLabelSpaceVerify(t *testing.T) {
	lipgloss.SetColorProfile(0)
	defer lipgloss.SetColorProfile(-1)

	m := chatModel{
		width:    80,
		themeIdx: 0,
		msgs: []chatMsg{
			{role: "user", text: "hi\n\n📎 *Attached: notes.txt, plan.md*", at: time.Now()},
			{role: "agent", text: "got it, thanks!", at: time.Now()},
		},
	}
	s := theme.MakeStyles(theme.BuiltinThemes[0])
	out := m.renderMessages(s)
	for i, line := range strings.Split(out, "\n") {
		fmt.Printf("L%02d: %q\n", i, line)
	}
}

package main

import (
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/lipgloss"
)

// ── Glamour-backed markdown renderer ─────────────────────────────────────────
//
// glamour (github.com/charmbracelet/glamour v0.8.1) provides full CommonMark +
// GFM rendering via goldmark: headings, bold, italic, strikethrough, inline
// code, fenced code blocks with Chroma syntax highlighting, tables, ordered
// and unordered lists, blockquotes, task lists, and horizontal rules.
//
// Design decisions:
//
//  1. TermRenderer cache — constructing a renderer is non-trivial (goldmark
//     init + Chroma setup).  We cache by (themeIdx, wordWrap) so the same
//     renderer is reused across frames and streaming chunks.
//
//  2. Word-wrap — glamour wraps at the given width and adds its own 2-column
//     left margin.  We pass contentW (the full available width) and let
//     glamour handle its own margin, then trim the leading/trailing blank lines
//     it emits so spacing stays consistent with the rest of the viewport.
//
//  3. Streaming safety — glamour / goldmark handles partial (unclosed) fences
//     gracefully; no special streaming logic is needed here.
//
//  4. Fallback — if glamour fails for any reason, we fall back to plain
//     lipgloss-wrapped text so the UI never goes blank.
//
//  5. parseSegments is retained because smartCopy in chat.go uses it to
//     locate the first code fence for clipboard copy.  It is NOT used for
//     rendering anymore.
//
//  6. Custom StyleConfig — each theme gets a hand-crafted ansi.StyleConfig
//     derived from its palette, so headings, code blocks, links, and
//     blockquotes all match the active colour scheme precisely.

// ── helpers matching glamour's internal style helpers ─────────────────────────

func strPtr(s string) *string { return &s }
func boolPtr(b bool) *bool    { return &b }
func uintPtr(u uint) *uint    { return &u }

// ── Per-theme glamour StyleConfig ─────────────────────────────────────────────
//
// Each config is tuned to its palette.  The Chroma theme inside CodeBlock
// controls syntax-highlight colours; we pick well-known Chroma themes that
// complement each palette.
//
// Chroma theme reference: https://xyproto.github.io/splash/docs/
//   dracula, monokai, nord, gruvbox, catppuccin-mocha, tokyo-night
//   (Chroma ships all of these in goldmark-highlighting ≥ v0.3.4)

func glamourStyleConfig(themeIdx int) ansi.StyleConfig {
	switch themeIdx {

	case 0: // ── Catppuccin Mocha ─────────────────────────────────────────
		return ansi.StyleConfig{
			Document: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					BlockPrefix: "\n",
					BlockSuffix: "\n",
					Color:       strPtr("#cdd6f4"),
				},
				Margin: uintPtr(2),
			},
			BlockQuote: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					Color:  strPtr("#a6adc8"),
					Italic: boolPtr(true),
				},
				Indent:      uintPtr(1),
				IndentToken: strPtr("▎ "),
			},
			Paragraph: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					Color: strPtr("#cdd6f4"),
				},
			},
			List: ansi.StyleList{
				LevelIndent: 2,
				StyleBlock: ansi.StyleBlock{
					StylePrimitive: ansi.StylePrimitive{
						Color: strPtr("#cdd6f4"),
					},
				},
			},
			Heading: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					BlockSuffix: "\n",
					Color:       strPtr("#cba6f7"),
					Bold:        boolPtr(true),
				},
			},
			H1: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "# "}},
			H2: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "## "}},
			H3: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "### "}},
			H4: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "#### "}},
			H5: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "##### "}},
			H6: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "###### "}},
			Strikethrough: ansi.StylePrimitive{CrossedOut: boolPtr(true)},
			Emph: ansi.StylePrimitive{
				Color:  strPtr("#f9e2af"),
				Italic: boolPtr(true),
			},
			Strong: ansi.StylePrimitive{
				Bold:  boolPtr(true),
				Color: strPtr("#89b4fa"),
			},
			HorizontalRule: ansi.StylePrimitive{
				Color:  strPtr("#585b70"),
				Format: "\n─────────────────────────────────────\n",
			},
			Item:        ansi.StylePrimitive{BlockPrefix: "• "},
			Enumeration: ansi.StylePrimitive{BlockPrefix: ". ", Color: strPtr("#89dceb")},
			Task: ansi.StyleTask{
				Ticked:   "[✓] ",
				Unticked: "[ ] ",
			},
			Link:     ansi.StylePrimitive{Color: strPtr("#89b4fa"), Underline: boolPtr(true)},
			LinkText: ansi.StylePrimitive{Color: strPtr("#cba6f7"), Bold: boolPtr(true)},
			Code: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					Color:           strPtr("#a6e3a1"),
					BackgroundColor: strPtr("#313244"),
				},
			},
			CodeBlock: ansi.StyleCodeBlock{
				StyleBlock: ansi.StyleBlock{
					Margin: uintPtr(2),
				},
				Theme: "catppuccin-mocha",
			},
			Table: ansi.StyleTable{
				StyleBlock:      ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr("#cdd6f4")}},
				CenterSeparator: strPtr("┼"),
				ColumnSeparator: strPtr("│"),
				RowSeparator:    strPtr("─"),
			},
		}

	case 1: // ── Tokyo Night ───────────────────────────────────────────────
		return ansi.StyleConfig{
			Document: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					BlockPrefix: "\n",
					BlockSuffix: "\n",
					Color:       strPtr("#c0caf5"),
				},
				Margin: uintPtr(2),
			},
			BlockQuote: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					Color:  strPtr("#565f89"),
					Italic: boolPtr(true),
				},
				Indent:      uintPtr(1),
				IndentToken: strPtr("▎ "),
			},
			Paragraph: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{Color: strPtr("#c0caf5")},
			},
			List: ansi.StyleList{
				LevelIndent: 2,
				StyleBlock:  ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr("#c0caf5")}},
			},
			Heading: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					BlockSuffix: "\n",
					Color:       strPtr("#7aa2f7"),
					Bold:        boolPtr(true),
				},
			},
			H1: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "# "}},
			H2: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "## "}},
			H3: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "### "}},
			H4: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "#### "}},
			H5: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "##### "}},
			H6: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "###### "}},
			Strikethrough: ansi.StylePrimitive{CrossedOut: boolPtr(true)},
			Emph: ansi.StylePrimitive{
				Color:  strPtr("#e0af68"),
				Italic: boolPtr(true),
			},
			Strong: ansi.StylePrimitive{
				Bold:  boolPtr(true),
				Color: strPtr("#bb9af7"),
			},
			HorizontalRule: ansi.StylePrimitive{
				Color:  strPtr("#414868"),
				Format: "\n─────────────────────────────────────\n",
			},
			Item:        ansi.StylePrimitive{BlockPrefix: "• "},
			Enumeration: ansi.StylePrimitive{BlockPrefix: ". ", Color: strPtr("#2ac3de")},
			Task: ansi.StyleTask{
				Ticked:   "[✓] ",
				Unticked: "[ ] ",
			},
			Link:     ansi.StylePrimitive{Color: strPtr("#7aa2f7"), Underline: boolPtr(true)},
			LinkText: ansi.StylePrimitive{Color: strPtr("#bb9af7"), Bold: boolPtr(true)},
			Code: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					Color:           strPtr("#9ece6a"),
					BackgroundColor: strPtr("#1f2335"),
				},
			},
			CodeBlock: ansi.StyleCodeBlock{
				StyleBlock: ansi.StyleBlock{Margin: uintPtr(2)},
				Theme:      "tokyo-night",
			},
			Table: ansi.StyleTable{
				StyleBlock:      ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr("#c0caf5")}},
				CenterSeparator: strPtr("┼"),
				ColumnSeparator: strPtr("│"),
				RowSeparator:    strPtr("─"),
			},
		}

	case 2: // ── Rosé Pine ─────────────────────────────────────────────────
		return ansi.StyleConfig{
			Document: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					BlockPrefix: "\n",
					BlockSuffix: "\n",
					Color:       strPtr("#e0def4"),
				},
				Margin: uintPtr(2),
			},
			BlockQuote: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					Color:  strPtr("#908caa"),
					Italic: boolPtr(true),
				},
				Indent:      uintPtr(1),
				IndentToken: strPtr("▎ "),
			},
			Paragraph: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{Color: strPtr("#e0def4")},
			},
			List: ansi.StyleList{
				LevelIndent: 2,
				StyleBlock:  ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr("#e0def4")}},
			},
			Heading: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					BlockSuffix: "\n",
					Color:       strPtr("#c4a7e7"),
					Bold:        boolPtr(true),
				},
			},
			H1: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "# "}},
			H2: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "## "}},
			H3: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "### "}},
			H4: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "#### "}},
			H5: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "##### "}},
			H6: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "###### "}},
			Strikethrough: ansi.StylePrimitive{CrossedOut: boolPtr(true)},
			Emph: ansi.StylePrimitive{
				Color:  strPtr("#f6c177"),
				Italic: boolPtr(true),
			},
			Strong: ansi.StylePrimitive{
				Bold:  boolPtr(true),
				Color: strPtr("#9ccfd8"),
			},
			HorizontalRule: ansi.StylePrimitive{
				Color:  strPtr("#403d52"),
				Format: "\n─────────────────────────────────────\n",
			},
			Item:        ansi.StylePrimitive{BlockPrefix: "• "},
			Enumeration: ansi.StylePrimitive{BlockPrefix: ". ", Color: strPtr("#ebbcba")},
			Task: ansi.StyleTask{
				Ticked:   "[✓] ",
				Unticked: "[ ] ",
			},
			Link:     ansi.StylePrimitive{Color: strPtr("#9ccfd8"), Underline: boolPtr(true)},
			LinkText: ansi.StylePrimitive{Color: strPtr("#c4a7e7"), Bold: boolPtr(true)},
			Code: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					Color:           strPtr("#31748f"),
					BackgroundColor: strPtr("#2a2837"),
				},
			},
			CodeBlock: ansi.StyleCodeBlock{
				StyleBlock: ansi.StyleBlock{Margin: uintPtr(2)},
				Theme:      "dracula",
			},
			Table: ansi.StyleTable{
				StyleBlock:      ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr("#e0def4")}},
				CenterSeparator: strPtr("┼"),
				ColumnSeparator: strPtr("│"),
				RowSeparator:    strPtr("─"),
			},
		}

	case 3: // ── Nord ──────────────────────────────────────────────────────
		return ansi.StyleConfig{
			Document: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					BlockPrefix: "\n",
					BlockSuffix: "\n",
					Color:       strPtr("#d8dee9"),
				},
				Margin: uintPtr(2),
			},
			BlockQuote: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					Color:  strPtr("#4c566a"),
					Italic: boolPtr(true),
				},
				Indent:      uintPtr(1),
				IndentToken: strPtr("▎ "),
			},
			Paragraph: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{Color: strPtr("#d8dee9")},
			},
			List: ansi.StyleList{
				LevelIndent: 2,
				StyleBlock:  ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr("#d8dee9")}},
			},
			Heading: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					BlockSuffix: "\n",
					Color:       strPtr("#81a1c1"),
					Bold:        boolPtr(true),
				},
			},
			H1: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "# "}},
			H2: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "## "}},
			H3: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "### "}},
			H4: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "#### "}},
			H5: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "##### "}},
			H6: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "###### "}},
			Strikethrough: ansi.StylePrimitive{CrossedOut: boolPtr(true)},
			Emph: ansi.StylePrimitive{
				Color:  strPtr("#ebcb8b"),
				Italic: boolPtr(true),
			},
			Strong: ansi.StylePrimitive{
				Bold:  boolPtr(true),
				Color: strPtr("#88c0d0"),
			},
			HorizontalRule: ansi.StylePrimitive{
				Color:  strPtr("#3b4252"),
				Format: "\n─────────────────────────────────────\n",
			},
			Item:        ansi.StylePrimitive{BlockPrefix: "• "},
			Enumeration: ansi.StylePrimitive{BlockPrefix: ". ", Color: strPtr("#8fbcbb")},
			Task: ansi.StyleTask{
				Ticked:   "[✓] ",
				Unticked: "[ ] ",
			},
			Link:     ansi.StylePrimitive{Color: strPtr("#81a1c1"), Underline: boolPtr(true)},
			LinkText: ansi.StylePrimitive{Color: strPtr("#88c0d0"), Bold: boolPtr(true)},
			Code: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					Color:           strPtr("#a3be8c"),
					BackgroundColor: strPtr("#2e3440"),
				},
			},
			CodeBlock: ansi.StyleCodeBlock{
				StyleBlock: ansi.StyleBlock{Margin: uintPtr(2)},
				Theme:      "nord",
			},
			Table: ansi.StyleTable{
				StyleBlock:      ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr("#d8dee9")}},
				CenterSeparator: strPtr("┼"),
				ColumnSeparator: strPtr("│"),
				RowSeparator:    strPtr("─"),
			},
		}

	default: // case 4: ── Gruvbox ───────────────────────────────────────
		return ansi.StyleConfig{
			Document: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					BlockPrefix: "\n",
					BlockSuffix: "\n",
					Color:       strPtr("#ebdbb2"),
				},
				Margin: uintPtr(2),
			},
			BlockQuote: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					Color:  strPtr("#a89984"),
					Italic: boolPtr(true),
				},
				Indent:      uintPtr(1),
				IndentToken: strPtr("▎ "),
			},
			Paragraph: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{Color: strPtr("#ebdbb2")},
			},
			List: ansi.StyleList{
				LevelIndent: 2,
				StyleBlock:  ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr("#ebdbb2")}},
			},
			Heading: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					BlockSuffix: "\n",
					Color:       strPtr("#fabd2f"),
					Bold:        boolPtr(true),
				},
			},
			H1: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "# "}},
			H2: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "## "}},
			H3: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "### "}},
			H4: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "#### "}},
			H5: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "##### "}},
			H6: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "###### "}},
			Strikethrough: ansi.StylePrimitive{CrossedOut: boolPtr(true)},
			Emph: ansi.StylePrimitive{
				Color:  strPtr("#fe8019"),
				Italic: boolPtr(true),
			},
			Strong: ansi.StylePrimitive{
				Bold:  boolPtr(true),
				Color: strPtr("#8ec07c"),
			},
			HorizontalRule: ansi.StylePrimitive{
				Color:  strPtr("#504945"),
				Format: "\n─────────────────────────────────────\n",
			},
			Item:        ansi.StylePrimitive{BlockPrefix: "• "},
			Enumeration: ansi.StylePrimitive{BlockPrefix: ". ", Color: strPtr("#83a598")},
			Task: ansi.StyleTask{
				Ticked:   "[✓] ",
				Unticked: "[ ] ",
			},
			Link:     ansi.StylePrimitive{Color: strPtr("#83a598"), Underline: boolPtr(true)},
			LinkText: ansi.StylePrimitive{Color: strPtr("#fabd2f"), Bold: boolPtr(true)},
			Code: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					Color:           strPtr("#b8bb26"),
					BackgroundColor: strPtr("#1d2021"),
				},
			},
			CodeBlock: ansi.StyleCodeBlock{
				StyleBlock: ansi.StyleBlock{Margin: uintPtr(2)},
				Theme:      "gruvbox",
			},
			Table: ansi.StyleTable{
				StyleBlock:      ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr("#ebdbb2")}},
				CenterSeparator: strPtr("┼"),
				ColumnSeparator: strPtr("│"),
				RowSeparator:    strPtr("─"),
			},
		}

	case 5: // ── GitHub Light ──────────────────────────────────────────────
		return ansi.StyleConfig{
			Document: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					BlockPrefix: "\n",
					BlockSuffix: "\n",
					Color:       strPtr("#24292f"),
				},
				Margin: uintPtr(2),
			},
			BlockQuote: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					Color:  strPtr("#57606a"),
					Italic: boolPtr(true),
				},
				Indent:      uintPtr(1),
				IndentToken: strPtr("▎ "),
			},
			Paragraph: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{Color: strPtr("#24292f")},
			},
			List: ansi.StyleList{
				LevelIndent: 2,
				StyleBlock: ansi.StyleBlock{
					StylePrimitive: ansi.StylePrimitive{Color: strPtr("#24292f")},
				},
			},
			Heading: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					BlockSuffix: "\n",
					Color:       strPtr("#0550ae"),
					Bold:        boolPtr(true),
				},
			},
			H1: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "# "}},
			H2: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "## "}},
			H3: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "### "}},
			H4: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "#### "}},
			H5: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "##### "}},
			H6: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "###### "}},
			Strikethrough: ansi.StylePrimitive{CrossedOut: boolPtr(true)},
			Emph: ansi.StylePrimitive{
				Color:  strPtr("#953800"),
				Italic: boolPtr(true),
			},
			Strong: ansi.StylePrimitive{
				Bold:  boolPtr(true),
				Color: strPtr("#0550ae"),
			},
			HorizontalRule: ansi.StylePrimitive{
				Color:  strPtr("#d0d7de"),
				Format: "\n─────────────────────────────────────\n",
			},
			Item:        ansi.StylePrimitive{BlockPrefix: "• "},
			Enumeration: ansi.StylePrimitive{BlockPrefix: ". ", Color: strPtr("#0550ae")},
			Task: ansi.StyleTask{
				Ticked:   "[✓] ",
				Unticked: "[ ] ",
			},
			Link:     ansi.StylePrimitive{Color: strPtr("#0550ae"), Underline: boolPtr(true)},
			LinkText: ansi.StylePrimitive{Color: strPtr("#8250df"), Bold: boolPtr(true)},
			Code: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					Color:           strPtr("#116329"),
					BackgroundColor: strPtr("#eaeef2"),
				},
			},
			CodeBlock: ansi.StyleCodeBlock{
				StyleBlock: ansi.StyleBlock{Margin: uintPtr(2)},
				Theme:      "github",
			},
			Table: ansi.StyleTable{
				StyleBlock:      ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr("#24292f")}},
				CenterSeparator: strPtr("┼"),
				ColumnSeparator: strPtr("│"),
				RowSeparator:    strPtr("─"),
			},
		}

	case 6: // ── Solarized Light ───────────────────────────────────────────
		return ansi.StyleConfig{
			Document: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					BlockPrefix: "\n",
					BlockSuffix: "\n",
					Color:       strPtr("#657b83"),
				},
				Margin: uintPtr(2),
			},
			BlockQuote: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					Color:  strPtr("#93a1a1"),
					Italic: boolPtr(true),
				},
				Indent:      uintPtr(1),
				IndentToken: strPtr("▎ "),
			},
			Paragraph: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{Color: strPtr("#657b83")},
			},
			List: ansi.StyleList{
				LevelIndent: 2,
				StyleBlock: ansi.StyleBlock{
					StylePrimitive: ansi.StylePrimitive{Color: strPtr("#657b83")},
				},
			},
			Heading: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					BlockSuffix: "\n",
					Color:       strPtr("#268bd2"),
					Bold:        boolPtr(true),
				},
			},
			H1: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "# "}},
			H2: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "## "}},
			H3: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "### "}},
			H4: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "#### "}},
			H5: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "##### "}},
			H6: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "###### "}},
			Strikethrough: ansi.StylePrimitive{CrossedOut: boolPtr(true)},
			Emph: ansi.StylePrimitive{
				Color:  strPtr("#cb4b16"),
				Italic: boolPtr(true),
			},
			Strong: ansi.StylePrimitive{
				Bold:  boolPtr(true),
				Color: strPtr("#073642"),
			},
			HorizontalRule: ansi.StylePrimitive{
				Color:  strPtr("#93a1a1"),
				Format: "\n─────────────────────────────────────\n",
			},
			Item:        ansi.StylePrimitive{BlockPrefix: "• "},
			Enumeration: ansi.StylePrimitive{BlockPrefix: ". ", Color: strPtr("#2aa198")},
			Task: ansi.StyleTask{
				Ticked:   "[✓] ",
				Unticked: "[ ] ",
			},
			Link:     ansi.StylePrimitive{Color: strPtr("#268bd2"), Underline: boolPtr(true)},
			LinkText: ansi.StylePrimitive{Color: strPtr("#6c71c4"), Bold: boolPtr(true)},
			Code: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					Color:           strPtr("#859900"),
					BackgroundColor: strPtr("#e8e0c8"),
				},
			},
			CodeBlock: ansi.StyleCodeBlock{
				StyleBlock: ansi.StyleBlock{Margin: uintPtr(2)},
				Theme:      "solarized-light",
			},
			Table: ansi.StyleTable{
				StyleBlock:      ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr("#657b83")}},
				CenterSeparator: strPtr("┼"),
				ColumnSeparator: strPtr("│"),
				RowSeparator:    strPtr("─"),
			},
		}

	case 7: // ── Tango (Cyan) ──────────────────────────────────────────────
		return ansi.StyleConfig{
			Document: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					BlockPrefix: "\n",
					BlockSuffix: "\n",
					Color:       strPtr("#2e3436"),
				},
				Margin: uintPtr(2),
			},
			BlockQuote: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					Color:  strPtr("#888a85"),
					Italic: boolPtr(true),
				},
				Indent:      uintPtr(1),
				IndentToken: strPtr("▎ "),
			},
			Paragraph: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{Color: strPtr("#2e3436")},
			},
			List: ansi.StyleList{
				LevelIndent: 2,
				StyleBlock: ansi.StyleBlock{
					StylePrimitive: ansi.StylePrimitive{Color: strPtr("#2e3436")},
				},
			},
			Heading: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					BlockSuffix: "\n",
					Color:       strPtr("#204a87"),
					Bold:        boolPtr(true),
				},
			},
			H1: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "# "}},
			H2: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "## "}},
			H3: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "### "}},
			H4: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "#### "}},
			H5: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "##### "}},
			H6: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "###### "}},
			Strikethrough: ansi.StylePrimitive{CrossedOut: boolPtr(true)},
			Emph: ansi.StylePrimitive{
				Color:  strPtr("#75507b"),
				Italic: boolPtr(true),
			},
			Strong: ansi.StylePrimitive{
				Bold:  boolPtr(true),
				Color: strPtr("#204a87"),
			},
			HorizontalRule: ansi.StylePrimitive{
				Color:  strPtr("#a8d8ea"),
				Format: "\n─────────────────────────────────────\n",
			},
			Item:        ansi.StylePrimitive{BlockPrefix: "• "},
			Enumeration: ansi.StylePrimitive{BlockPrefix: ". ", Color: strPtr("#204a87")},
			Task: ansi.StyleTask{
				Ticked:   "[✓] ",
				Unticked: "[ ] ",
			},
			Link:     ansi.StylePrimitive{Color: strPtr("#204a87"), Underline: boolPtr(true)},
			LinkText: ansi.StylePrimitive{Color: strPtr("#75507b"), Bold: boolPtr(true)},
			Code: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					Color:           strPtr("#4e9a06"),
					BackgroundColor: strPtr("#d3eef9"),
				},
			},
			CodeBlock: ansi.StyleCodeBlock{
				StyleBlock: ansi.StyleBlock{Margin: uintPtr(2)},
				Theme:      "friendly",
			},
			Table: ansi.StyleTable{
				StyleBlock:      ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr("#2e3436")}},
				CenterSeparator: strPtr("┼"),
				ColumnSeparator: strPtr("│"),
				RowSeparator:    strPtr("─"),
			},
		}
	}
}

// ── Renderer cache ────────────────────────────────────────────────────────────

type rendererKey struct {
	themeIdx int
	wordWrap int
}

var (
	rendererMu    sync.Mutex
	rendererCache = map[rendererKey]*glamour.TermRenderer{}
)

// invalidateRendererCache clears the entire renderer cache.  Call this
// whenever the active theme changes so the next renderMarkdown call
// constructs a fresh TermRenderer with the new glamour style.
func invalidateRendererCache() {
	rendererMu.Lock()
	defer rendererMu.Unlock()
	rendererCache = map[rendererKey]*glamour.TermRenderer{}
}

// cachedRenderer returns a *glamour.TermRenderer for the given theme index and
// word-wrap width, constructing and caching one on first use.  Thread-safe.
func cachedRenderer(themeIdx, wordWrap int) *glamour.TermRenderer {
	key := rendererKey{themeIdx, wordWrap}

	rendererMu.Lock()
	defer rendererMu.Unlock()

	if r, ok := rendererCache[key]; ok {
		return r
	}

	cfg := glamourStyleConfig(themeIdx)
	r, err := glamour.NewTermRenderer(
		glamour.WithStyles(cfg),
		glamour.WithWordWrap(wordWrap),
	)
	if err != nil {
		// Defensive fallback: standard dark style.
		r, _ = glamour.NewTermRenderer(
			glamour.WithStandardStyle("dark"),
			glamour.WithWordWrap(wordWrap),
		)
	}
	if r != nil {
		rendererCache[key] = r
	}
	return r
}

// ── Top-level renderer ────────────────────────────────────────────────────────

// renderMarkdown converts an LLM reply into styled terminal output using
// glamour for full CommonMark + GFM rendering.
//
// Parameters:
//   - s          styledSet for the active theme (s.themeIdx selects glamour style)
//   - text       raw markdown text from the model
//   - contentW   available column width for the message body
//   - baseStyle  fallback lipgloss style (used only when glamour is unavailable)
func renderMarkdown(s styledSet, text string, contentW int, baseStyle lipgloss.Style) string {
	if text == "" {
		return ""
	}

	// Narrow terminal: skip glamour and just wrap plainly.
	if contentW < 12 {
		return baseStyle.PaddingLeft(2).Width(contentW).Render(strings.TrimSpace(text)) + "\n"
	}

	r := cachedRenderer(s.themeIdx, contentW)
	if r == nil {
		return baseStyle.PaddingLeft(2).Width(contentW).Render(strings.TrimSpace(text)) + "\n"
	}

	rendered, err := r.Render(text)
	if err != nil {
		return baseStyle.PaddingLeft(2).Width(contentW).Render(strings.TrimSpace(text)) + "\n"
	}

	// glamour wraps the output in a leading \n and trailing \n\n.
	// Trim trailing blank lines; keep at most one trailing newline so spacing
	// stays consistent with the "user" and "error" message blocks.
	rendered = strings.TrimRight(rendered, "\n")
	if rendered == "" {
		return ""
	}
	return rendered + "\n"
}

// ── parseSegments — retained for smartCopy in chat.go ────────────────────────
//
// smartCopy uses parseSegments to locate the first fenced code block so it can
// copy just the code body to the clipboard.  This function is no longer used
// for rendering.

// textSegment is one parsed piece of a message — prose or a fenced code block.
type textSegment struct {
	code bool   // true for a ```…``` block
	lang string // fence language tag ("go", "python", …); empty if absent
	body string // raw body text of the segment
}

// parseSegments splits text into alternating prose and code-fence segments.
func parseSegments(text string) []textSegment {
	var segs []textSegment
	var buf strings.Builder
	inCode := false
	var lang string

	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)

		if !inCode && strings.HasPrefix(trimmed, "```") {
			if buf.Len() > 0 {
				segs = append(segs, textSegment{body: strings.Trim(buf.String(), "\n")})
				buf.Reset()
			}
			lang = strings.TrimPrefix(trimmed, "```")
			inCode = true
			continue
		}

		if inCode && trimmed == "```" {
			segs = append(segs, textSegment{
				code: true,
				lang: lang,
				body: strings.Trim(buf.String(), "\n"),
			})
			buf.Reset()
			inCode = false
			lang = ""
			continue
		}

		if buf.Len() > 0 {
			buf.WriteByte('\n')
		}
		buf.WriteString(line)
	}

	if buf.Len() > 0 {
		segs = append(segs, textSegment{
			code: inCode,
			lang: lang,
			body: strings.Trim(buf.String(), "\n"),
		})
	}
	return segs
}

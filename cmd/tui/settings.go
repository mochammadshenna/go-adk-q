package main

import (
	"fmt"

	"github.com/charmbracelet/huh"
)

// buildSettingsForm constructs the huh form for the TUI settings overlay.
//
// themeIdx and charLimit are output pointers; huh writes the chosen values
// into them when the user confirms.  Callers should pass pointers to fields
// on chatModel (settingsThemeIdx, settingsCharLimit) so that UpdateSettings
// can read the results without any extra return value plumbing.
//
// The returned *huh.Form implements tea.Model and can be sized with
// WithWidth / WithHeight before passing to Init.
func buildSettingsForm(themeIdx *int, charLimit *int) *huh.Form {
	// Build theme option list from builtinThemes.
	themeOptions := make([]huh.Option[int], len(builtinThemes))
	for i, t := range builtinThemes {
		themeOptions[i] = huh.NewOption(t.name, i)
	}

	// Character-limit presets.
	limitOptions := []huh.Option[int]{
		huh.NewOption("500   chars (concise)", 500),
		huh.NewOption("1000  chars (default-ish)", 1000),
		huh.NewOption("2000  chars (current default)", 2000),
		huh.NewOption("4000  chars (long)", 4000),
		huh.NewOption("8000  chars (very long)", 8000),
	}

	// Snap charLimit to the nearest preset so the selector pre-selects it.
	closestLimit := snapToPreset(*charLimit, []int{500, 1000, 2000, 4000, 8000})
	*charLimit = closestLimit

	return huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[int]().
				Title("Colour theme").
				Description("Cycle with 't' or pick one here.").
				Options(themeOptions...).
				Value(themeIdx),

			huh.NewSelect[int]().
				Title("Message character limit").
				Description(fmt.Sprintf("Currently %d. Applies immediately on confirm.", *charLimit)).
				Options(limitOptions...).
				Value(charLimit),
		).
			Title("Settings").
			Description("Arrow keys navigate  •  enter selects  •  esc cancels"),
	)
}

// snapToPreset returns the preset value from presets that is closest to v.
func snapToPreset(v int, presets []int) int {
	best := presets[0]
	bestDiff := abs(v - best)
	for _, p := range presets[1:] {
		if d := abs(v - p); d < bestDiff {
			bestDiff = d
			best = p
		}
	}
	return best
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

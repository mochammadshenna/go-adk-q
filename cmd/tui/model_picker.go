package main

// model_picker.go — 2-level interactive picker for /model.
//
// UX design:
//   - If only 1 provider is configured, skip straight to the model list.
//   - If multiple providers are configured, show a provider list first, then
//     the model list for the chosen provider.
//
// Navigation: ↑/↓ to move, enter to confirm, esc to cancel or go back.
// The picker is rendered inside the viewport area so it scrolls correctly.
// Title is always "Select Model" regardless of stage.

import (
	"fmt"
	"strings"

	"go-adk-q/model/catalog"

	"github.com/charmbracelet/lipgloss"
)

// pickerStage indicates which level of the 2-level picker is active.
type pickerStage int

const (
	pickerStageProvider pickerStage = iota // choosing a provider (skipped when only 1)
	pickerStageModel                       // choosing a model within a provider
)

// modelPickerState holds all mutable state for the /model picker overlay.
// Zero value is not valid; use newModelPickerState.
type modelPickerState struct {
	stage pickerStage

	// Provider level
	providers      []catalog.ProviderCatalog // active providers (keyed configured)
	providerIdx    int                       // currently highlighted row
	chosenProvider int                       // index into providers; set on enter

	// Model level
	models        []catalog.ModelEntry // models for the chosen provider
	modelIdx      int                  // currently highlighted row
	activeModelID string               // ID of the currently running model (for pre-selection)
}

// newModelPickerState builds the picker pre-populated with the catalogs whose
// provider IDs are present in activeProviderIDs (e.g. from the runner's
// model.LLM.Name() strings).  All catalogs are shown when activeProviderIDs
// is empty (useful for testing).
//
// activeModelID is the model ID currently in use (e.g. "meta-llama/Llama-4-Scout-17B-16E-Instruct").
// When it matches an entry in the visible catalog the picker pre-selects that
// row so the user can see which model is active and navigate from there.
//
// When exactly 1 provider is visible the picker starts directly at the model
// stage so the user never has to navigate a 1-item provider list.
func newModelPickerState(activeProviderIDs []string, activeModelID string) modelPickerState {
	all := catalog.All()
	var visible []catalog.ProviderCatalog

	if len(activeProviderIDs) == 0 {
		visible = all
	} else {
		for _, c := range all {
			for _, id := range activeProviderIDs {
				if strings.Contains(id, c.Provider) {
					visible = append(visible, c)
					break
				}
			}
		}
	}
	if len(visible) == 0 {
		// Fallback: show everything so the picker is never empty.
		visible = all
	}

	p := modelPickerState{providers: visible, activeModelID: activeModelID}

	// Auto-advance to model stage when there is exactly 1 provider.
	if len(visible) == 1 {
		p.chosenProvider = 0
		p.models = visible[0].Models
		p.modelIdx = activeModelIdxIn(p.models, activeModelID)
		p.stage = pickerStageModel
		return p
	}

	// Multiple providers: pre-highlight the provider row that owns the active model.
	for pi, prov := range visible {
		for _, m := range prov.Models {
			if m.ID == activeModelID {
				p.providerIdx = pi
				break
			}
		}
	}

	return p
}

// activeModelIdxIn returns the index of the entry whose ID equals activeModelID,
// falling back to the Default entry, then to 0.
func activeModelIdxIn(models []catalog.ModelEntry, activeModelID string) int {
	// First pass: exact ID match against the currently running model.
	for i, m := range models {
		if m.ID == activeModelID {
			return i
		}
	}
	// Second pass: suffix match — activeModelID may be
	// "provider/model-id" and the catalog stores just "model-id".
	if idx := strings.LastIndex(activeModelID, "/"); idx >= 0 {
		bare := activeModelID[idx+1:]
		for i, m := range models {
			if m.ID == bare {
				return i
			}
		}
	}
	// Fallback: Default entry.
	return defaultModelIdx(models)
}

// defaultModelIdx returns the index of the entry with Default==true, or 0.
func defaultModelIdx(models []catalog.ModelEntry) int {
	for i, m := range models {
		if m.Default {
			return i
		}
	}
	return 0
}

// ── Input handling ─────────────────────────────────────────────────────────────

// pickerMoveUp moves the highlighted row up by one.
func (p *modelPickerState) pickerMoveUp() {
	switch p.stage {
	case pickerStageProvider:
		if p.providerIdx > 0 {
			p.providerIdx--
		}
	case pickerStageModel:
		if p.modelIdx > 0 {
			p.modelIdx--
		}
	}
}

// pickerMoveDown moves the highlighted row down by one.
func (p *modelPickerState) pickerMoveDown() {
	switch p.stage {
	case pickerStageProvider:
		if p.providerIdx < len(p.providers)-1 {
			p.providerIdx++
		}
	case pickerStageModel:
		if p.modelIdx < len(p.models)-1 {
			p.modelIdx++
		}
	}
}

// pickerConfirm advances the picker by one stage and returns true when a
// model has been fully selected (i.e. the user pressed enter on a model row).
// Returns (providerID, modelID, true) on final confirmation.
func (p *modelPickerState) pickerConfirm() (providerID, modelID string, done bool) {
	switch p.stage {
	case pickerStageProvider:
		if len(p.providers) == 0 {
			return "", "", false
		}
		p.chosenProvider = p.providerIdx
		chosen := p.providers[p.chosenProvider]
		p.models = chosen.Models
		// Pre-select the currently active model if it belongs to this provider,
		// otherwise fall back to the Default entry.
		p.modelIdx = activeModelIdxIn(p.models, p.activeModelID)
		p.stage = pickerStageModel
		return "", "", false

	case pickerStageModel:
		if len(p.models) == 0 {
			return "", "", false
		}
		prov := p.providers[p.chosenProvider]
		entry := p.models[p.modelIdx]
		return prov.Provider, entry.ID, true
	}
	return "", "", false
}

// pickerBack returns the picker one stage, or signals the caller to close the
// picker entirely when already on the provider stage (or at model stage with
// only 1 provider, where there is no provider stage to go back to).
func (p *modelPickerState) pickerBack() (close bool) {
	if p.stage == pickerStageProvider {
		return true
	}
	// At model stage: go back to provider list only when multiple providers exist.
	if len(p.providers) <= 1 {
		return true // close — no provider list to show
	}
	p.stage = pickerStageProvider
	return false
}

// ── Rendering ─────────────────────────────────────────────────────────────────

// pickerView renders the entire picker as a string ready for SetContent.
func (p *modelPickerState) pickerView(s styledSet, w int) string {
	switch p.stage {
	case pickerStageProvider:
		return p.providerView(s, w)
	case pickerStageModel:
		return p.modelView(s, w)
	}
	return ""
}

func (p *modelPickerState) providerView(s styledSet, w int) string {
	var sb strings.Builder

	// Title is always "Select Model" per UX spec.
	title := s.agentLabel.Render("Select Model") +
		s.system.Render("  esc: cancel  •  ↑/↓: navigate  •  enter: select provider")
	sb.WriteString(title + "\n\n")

	innerW := w - 4
	if innerW < 10 {
		innerW = 10
	}

	for i, c := range p.providers {
		label := c.Label
		row := fmt.Sprintf("  %-*s", innerW-2, label)
		if i == p.providerIdx {
			sb.WriteString(s.agentLabel.Width(innerW).Render(row))
		} else {
			sb.WriteString(s.system.Width(innerW).Render(row))
		}
		sb.WriteByte('\n')
	}

	sb.WriteString("\n")
	sb.WriteString(s.system.Render(fmt.Sprintf("  %d providers available", len(p.providers))))
	return sb.String()
}

func (p *modelPickerState) modelView(s styledSet, w int) string {
	var sb strings.Builder

	prov := p.providers[p.chosenProvider]

	// Build header hint — show "esc: cancel" when single provider, "esc: back" when multiple.
	escHint := "esc: cancel"
	if len(p.providers) > 1 {
		escHint = "esc: back"
	}

	// Title is always "Select Model".
	title := s.agentLabel.Render("Select Model") +
		s.system.Render(fmt.Sprintf("  %s  •  ↑/↓: navigate  •  enter: switch  •  provider: %s", escHint, prov.Label))
	sb.WriteString(title + "\n\n")

	innerW := w - 4
	if innerW < 10 {
		innerW = 10
	}

	for i, m := range p.models {
		label := m.DisplayName()
		tags := ""
		if len(m.Tags) > 0 {
			tags = "  [" + strings.Join(m.Tags, ", ") + "]"
		}
		// ● = currently active model; ○ = catalog default (if different).
		activeMark := ""
		isActive := activeModelIdxIn(p.models, p.activeModelID) == i
		if isActive {
			activeMark = " ●"
		} else if m.Default {
			activeMark = " ○"
		}

		// Truncate label if it would overflow.
		maxLabel := innerW - 2 - len(tags) - len(activeMark)
		if maxLabel < 4 {
			maxLabel = 4
			tags = ""
		}
		if len(label) > maxLabel {
			label = label[:maxLabel-1] + "…"
		}

		content := "  " + label + activeMark
		tagStyled := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#f9e2af")).
			Render(tags)

		if i == p.modelIdx {
			sb.WriteString(s.agentLabel.Width(innerW - lipgloss.Width(tagStyled)).Render(content))
			sb.WriteString(tagStyled)
		} else {
			sb.WriteString(s.system.Width(innerW - lipgloss.Width(tagStyled)).Render(content))
			sb.WriteString(tagStyled)
		}
		sb.WriteByte('\n')
	}

	sb.WriteString("\n")
	sb.WriteString(s.system.Render(fmt.Sprintf("  %d models  •  ● = active  •  ○ = default", len(p.models))))
	return sb.String()
}

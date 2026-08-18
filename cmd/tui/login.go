package main

// login.go — /login: add or switch provider credentials from inside the TUI.
//
// UX is two SEPARATE, sequentially-swapped huh.Forms — auth method first,
// then (only if API Key was chosen) provider + key — rather than one huh.Form
// with multiple groups gated by WithHideFunc.
//
// Why two forms instead of one multi-group form: a single-field huh.Group's
// Select did not reliably auto-advance to the next group on Enter within one
// Form in this app's huh version — confirmed live via direct instrumentation
// (temporary slog around updateLogin's form.Update calls): the very next
// keystroke after confirming "API Key" was still being applied to the
// AUTH-METHOD select, not the next group, flipping the choice to
// "Subscription" before the user ever saw the provider/key fields. Swapping
// two independent single-purpose Forms explicitly (the same manual-staging
// principle model_picker.go already uses for its own provider/model
// 2-stage picker, just via huh instead of raw lipgloss) sidesteps that
// ambiguity entirely: each Form's own StateCompleted is unambiguous.
//
// Subscription is a placeholder: none of the 7 providers this app supports
// have an OAuth/subscription login implemented anywhere in this codebase —
// only raw API-key env vars. Choosing it ends the flow immediately with a
// "coming soon" notice; the provider/key form is never even constructed.

import (
	"fmt"

	"go-adk-q/model/catalog"

	"github.com/charmbracelet/huh"
)

const (
	loginAuthAPIKey       = "api_key"
	loginAuthSubscription = "subscription"
)

// loginFormResult is populated across both stages of the /login flow.
type loginFormResult struct {
	authMethod string
	provider   string // catalog.ProviderCatalog.Provider of the chosen entry
	key        string
}

// buildAuthMethodForm constructs stage 1: choose API Key or Subscription.
func buildAuthMethodForm(result *loginFormResult) *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Auth method").
				Options(
					huh.NewOption("API Key", loginAuthAPIKey),
					huh.NewOption("Subscription (coming soon)", loginAuthSubscription),
				).
				Value(&result.authMethod),
		).Title("Login"),
	)
}

// buildAPIKeyForm constructs stage 2 (only reached when stage 1 chose API
// Key): pick a provider, enter its key. providers must be non-empty (main.go's
// init() always registers at least the 7 built-in catalogs, so this is a
// programming-error panic, not a runtime condition, if ever empty).
func buildAPIKeyForm(providers []catalog.ProviderCatalog, result *loginFormResult) *huh.Form {
	if len(providers) == 0 {
		panic("buildAPIKeyForm: no providers registered")
	}

	providerOptions := make([]huh.Option[string], len(providers))
	for i, p := range providers {
		providerOptions[i] = huh.NewOption(p.Label, p.Provider)
	}
	result.provider = providers[0].Provider

	return huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Provider").
				Description("Which provider is this key for?").
				Options(providerOptions...).
				Value(&result.provider),

			huh.NewInput().
				Title("API key").
				EchoMode(huh.EchoModePassword).
				Validate(func(s string) error {
					if s == "" {
						return fmt.Errorf("key cannot be empty")
					}
					return nil
				}).
				Value(&result.key),
		).
			Title("API Key").
			Description("Saved to your OS config dir under layar-cli/credentials.json and used immediately."),
	)
}

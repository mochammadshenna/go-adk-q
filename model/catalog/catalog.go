// Package catalog provides a static, extensible registry of known models for
// each provider in the go-adk-q failover chain.
//
// # Purpose
//
// The /model TUI command needs a list of models to present to the user without
// making a network call.  Each provider package exposes a [ProviderCatalog]
// value (its KnownModels slice) that is registered here at init time.
//
// # Adding a new model
//
// Edit the provider's own catalog slice — e.g. githubmodels.KnownModels.
// No changes to this package are needed.
//
// # Adding a new provider
//
// Call [Register] from your provider package's init() or expose a
// [ProviderCatalog] and call Register in cmd/tui/main.go's init().
package catalog

import "sync"

// ModelEntry describes a single model offered by a provider.
type ModelEntry struct {
	// ID is the exact model identifier passed to the API (e.g. "gpt-4o",
	// "meta-llama/llama-3.3-70b-instruct:free").
	ID string

	// Label is a short human-readable name shown in the picker menu.
	// Falls back to ID when empty.
	Label string

	// Tags are optional descriptors (e.g. "free", "fast", "reasoning").
	// Shown alongside the label in the picker.
	Tags []string

	// Default marks the entry that is used when no override is configured.
	Default bool
}

// DisplayName returns Label if set, otherwise ID.
func (e ModelEntry) DisplayName() string {
	if e.Label != "" {
		return e.Label
	}
	return e.ID
}

// ProviderCatalog groups a provider's name with its known models.
type ProviderCatalog struct {
	// Provider is the short lowercase identifier (e.g. "github", "openrouter").
	// Must match the substring used by model.LLM.Name() so the TUI can
	// correlate the picker selection with the active failover chain entry.
	Provider string

	// Label is the display name shown in the provider picker (e.g. "GitHub Models").
	Label string

	// Models is the ordered list of known models for this provider.
	// The first entry with Default==true is pre-selected in the picker.
	Models []ModelEntry
}

var (
	mu       sync.RWMutex
	catalogs []ProviderCatalog
)

// Register adds a ProviderCatalog to the global registry.
// Typically called from provider package init() functions or from main.
// Safe for concurrent use; duplicates are not detected.
func Register(c ProviderCatalog) {
	mu.Lock()
	defer mu.Unlock()
	catalogs = append(catalogs, c)
}

// All returns a snapshot of the registered catalogs in registration order.
func All() []ProviderCatalog {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]ProviderCatalog, len(catalogs))
	copy(out, catalogs)
	return out
}

// ForProvider returns the catalog for the named provider (case-insensitive
// prefix match), or the zero value with ok==false if not found.
func ForProvider(name string) (ProviderCatalog, bool) {
	mu.RLock()
	defer mu.RUnlock()
	for _, c := range catalogs {
		if c.Provider == name {
			return c, true
		}
	}
	return ProviderCatalog{}, false
}

package main

// credentials.go — persistence for API keys entered via /login.
//
// Design: a saved credential is just a persisted environment variable. This
// deliberately does not introduce a second, parallel configuration mechanism
// alongside the existing env-var-based provider system (model/chain reads
// GROQ_API_KEY etc. directly) — loadCredentialsIntoEnv runs once at startup
// and os.Setenv's anything saved here, so every existing provider package
// and model/chain.Build keep working completely unchanged.
//
// Precedence: an already-set env var always wins over a saved credential
// (loadCredentialsIntoEnv only sets a variable that isn't already set) — env
// vars are the existing, documented mechanism (scripts, CI, direct exports)
// and should not be silently overridden by a stale saved file.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"go-adk-q/model/catalog"
)

// credentialsPath returns "<OS user config dir>/layar-cli/credentials.json"
// via os.UserConfigDir — $XDG_CONFIG_HOME or ~/.config on Linux, but
// ~/Library/Application Support on macOS (NOT ~/.config; os.UserConfigDir
// does not use XDG paths on Darwin) and %AppData% on Windows.
func credentialsPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve config dir: %w", err)
	}
	return filepath.Join(dir, "layar-cli", "credentials.json"), nil
}

// loadCredentials reads the saved provider->key map. A missing file is not
// an error — it returns an empty map, the state of a user who has never run
// /login.
func loadCredentials() (map[string]string, error) {
	path, err := credentialsPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("read credentials: %w", err)
	}
	var creds map[string]string
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("parse credentials: %w", err)
	}
	if creds == nil {
		creds = map[string]string{}
	}
	return creds, nil
}

// saveCredential merges provider->key into the saved credentials file,
// creating the parent directory if needed, and writes atomically (temp file +
// rename) so a failure never leaves a partially-written or truncated file.
// Permissions are 0600 — the file holds live API keys.
func saveCredential(provider, key string) error {
	path, err := credentialsPath()
	if err != nil {
		return err
	}
	creds, err := loadCredentials()
	if err != nil {
		return err
	}
	creds[provider] = key

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return fmt.Errorf("encode credentials: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write credentials: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("finalize credentials: %w", err)
	}
	return nil
}

// loadCredentialsIntoEnv applies every saved credential as an environment
// variable, but only when that variable isn't already set — an env var set
// by the shell, a script, or CI always takes precedence over a saved file.
// Called once at process startup, before any provider chain is built, so
// model/chain.Build (which reads os.Getenv directly) sees the credential
// exactly as if the user had exported it themselves.
func loadCredentialsIntoEnv() {
	creds, err := loadCredentials()
	if err != nil {
		// Non-fatal: a corrupt or unreadable credentials file should not
		// block startup — the user can still use env vars directly, or
		// re-run /login to overwrite it.
		return
	}
	for provider, key := range creds {
		if key == "" {
			continue
		}
		envVar := envVarForProvider(provider)
		if envVar == "" {
			continue
		}
		if os.Getenv(envVar) == "" {
			os.Setenv(envVar, key)
		}
	}
}

// envVarForProvider looks up the env var name for a saved provider ID via
// the catalog (populated at init() time — see this package's init in
// main.go), so this file needs no hardcoded provider->env-var table of its
// own.
func envVarForProvider(provider string) string {
	if c, ok := catalog.ForProvider(provider); ok {
		return c.EnvVar
	}
	return ""
}

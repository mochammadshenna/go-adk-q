package main

import (
	"os"
	"path/filepath"
	"testing"
)

// withTempConfigDir makes os.UserConfigDir hermetic for the duration of the
// test. os.UserConfigDir's real implementation (os/file.go) does NOT behave
// the same way across platforms — the "default: Unix" case reads
// $XDG_CONFIG_HOME (falling back to $HOME), but the "darwin, ios" case reads
// $HOME directly and ignores $XDG_CONFIG_HOME entirely — so $HOME must be
// overridden too, or this test silently reads/writes the real developer
// machine's actual ~/Library/Application Support/layar-cli on macOS instead
// of a temp directory (caught live during development of this test: it
// polluted the real config dir with fake test credentials before this fix).
func withTempConfigDir(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
}

func TestSaveCredential_RoundTrip(t *testing.T) {
	withTempConfigDir(t)

	if err := saveCredential("groq", "gsk-test-key"); err != nil {
		t.Fatalf("saveCredential: %v", err)
	}

	creds, err := loadCredentials()
	if err != nil {
		t.Fatalf("loadCredentials: %v", err)
	}
	if creds["groq"] != "gsk-test-key" {
		t.Errorf("creds[groq] = %q, want %q", creds["groq"], "gsk-test-key")
	}
}

func TestSaveCredential_MergePreservesOtherProviders(t *testing.T) {
	withTempConfigDir(t)

	if err := saveCredential("groq", "key-1"); err != nil {
		t.Fatalf("saveCredential(groq): %v", err)
	}
	if err := saveCredential("opencode", "key-2"); err != nil {
		t.Fatalf("saveCredential(opencode): %v", err)
	}

	creds, err := loadCredentials()
	if err != nil {
		t.Fatalf("loadCredentials: %v", err)
	}
	if creds["groq"] != "key-1" {
		t.Errorf("creds[groq] = %q, want %q (should survive the second save)", creds["groq"], "key-1")
	}
	if creds["opencode"] != "key-2" {
		t.Errorf("creds[opencode] = %q, want %q", creds["opencode"], "key-2")
	}
	if len(creds) != 2 {
		t.Errorf("len(creds) = %d, want 2", len(creds))
	}
}

func TestSaveCredential_OverwritesSameProvider(t *testing.T) {
	withTempConfigDir(t)

	if err := saveCredential("groq", "old-key"); err != nil {
		t.Fatalf("saveCredential (1st): %v", err)
	}
	if err := saveCredential("groq", "new-key"); err != nil {
		t.Fatalf("saveCredential (2nd): %v", err)
	}

	creds, err := loadCredentials()
	if err != nil {
		t.Fatalf("loadCredentials: %v", err)
	}
	if creds["groq"] != "new-key" {
		t.Errorf("creds[groq] = %q, want %q (overwrite)", creds["groq"], "new-key")
	}
}

func TestSaveCredential_FilePermissions(t *testing.T) {
	withTempConfigDir(t)

	if err := saveCredential("groq", "secret"); err != nil {
		t.Fatalf("saveCredential: %v", err)
	}

	path, err := credentialsPath()
	if err != nil {
		t.Fatalf("credentialsPath: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("credentials file mode = %o, want 0600 — this file holds live API keys", perm)
	}
}

func TestLoadCredentials_MissingFileReturnsEmptyNotError(t *testing.T) {
	withTempConfigDir(t)

	creds, err := loadCredentials()
	if err != nil {
		t.Fatalf("loadCredentials on a fresh config dir should not error, got: %v", err)
	}
	if len(creds) != 0 {
		t.Errorf("got %v, want an empty map for a user who has never run /login", creds)
	}
}

func TestLoadCredentialsIntoEnv_EnvVarWinsOverSavedFile(t *testing.T) {
	withTempConfigDir(t)
	t.Setenv("GROQ_API_KEY", "from-env")

	if err := saveCredential("groq", "from-saved-file"); err != nil {
		t.Fatalf("saveCredential: %v", err)
	}

	loadCredentialsIntoEnv()

	if got := os.Getenv("GROQ_API_KEY"); got != "from-env" {
		t.Errorf("GROQ_API_KEY = %q, want %q — an already-set env var must win over a saved credential", got, "from-env")
	}
}

func TestLoadCredentialsIntoEnv_AppliesSavedKeyWhenEnvUnset(t *testing.T) {
	withTempConfigDir(t)
	original, wasSet := os.LookupEnv("GROQ_API_KEY")
	os.Unsetenv("GROQ_API_KEY")
	defer func() {
		if wasSet {
			os.Setenv("GROQ_API_KEY", original)
		} else {
			os.Unsetenv("GROQ_API_KEY")
		}
	}()

	if err := saveCredential("groq", "from-saved-file"); err != nil {
		t.Fatalf("saveCredential: %v", err)
	}

	loadCredentialsIntoEnv()

	if got := os.Getenv("GROQ_API_KEY"); got != "from-saved-file" {
		t.Errorf("GROQ_API_KEY = %q, want %q — a saved credential should apply when nothing else set it", got, "from-saved-file")
	}
}

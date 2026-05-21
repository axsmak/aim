package globalconfig_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/axsmak/aim/internal/globalconfig"
)

func TestPath(t *testing.T) {
	home := "/home/testuser"
	got := globalconfig.Path(home)
	want := filepath.Join(home, ".config", "aim", "config.yaml")
	if got != want {
		t.Errorf("Path(%q) = %q; want %q", home, got, want)
	}
}

func TestLoadMissing(t *testing.T) {
	home := t.TempDir()
	cfg, err := globalconfig.Load(home)
	if err != nil {
		t.Fatalf("Load on missing file returned error: %v", err)
	}
	if cfg.Repo != "" {
		t.Errorf("Load on missing file returned non-empty Repo: %q", cfg.Repo)
	}
}

func TestSaveLoad(t *testing.T) {
	home := t.TempDir()
	// Pre-create the directory so Save has something to work with.
	original := globalconfig.Config{Repo: "/home/user/my-loadout"}
	if err := globalconfig.Save(home, original); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	loaded, err := globalconfig.Load(home)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded.Repo != original.Repo {
		t.Errorf("roundtrip mismatch: got Repo=%q, want %q", loaded.Repo, original.Repo)
	}
}

func TestSaveCreatesDir(t *testing.T) {
	home := t.TempDir()
	// Ensure the ~/.config/aim directory does NOT exist yet.
	configDir := filepath.Join(home, ".config", "aim")
	if _, err := os.Stat(configDir); !os.IsNotExist(err) {
		t.Fatalf("expected directory to be absent before Save, but got: %v", err)
	}

	cfg := globalconfig.Config{Repo: "/some/path"}
	if err := globalconfig.Save(home, cfg); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	if _, err := os.Stat(configDir); err != nil {
		t.Errorf("expected directory %q to exist after Save, but got: %v", configDir, err)
	}
}

func TestLoadInvalid(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "aim")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("setup MkdirAll: %v", err)
	}
	// Write invalid YAML (a tab character at the start triggers a YAML parse error).
	if err := os.WriteFile(globalconfig.Path(home), []byte("\t: invalid"), 0644); err != nil {
		t.Fatalf("setup WriteFile: %v", err)
	}

	_, err := globalconfig.Load(home)
	if err == nil {
		t.Error("Load on invalid YAML returned nil error; want non-nil error")
	}
}

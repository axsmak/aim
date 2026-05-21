package localconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cfg.Adapters.ClaudeCode.BaseDir != "" || cfg.Adapters.Cursor.BaseDir != "" || cfg.Adapters.Codex.BaseDir != "" {
		t.Fatal("expected empty config when file not found")
	}
}

func TestLoad_AllAdapters(t *testing.T) {
	dir := t.TempDir()
	content := `adapters:
  claude_code:
    base_dir: /custom/claude
  cursor:
    base_dir: /custom/cursor
  codex:
    base_dir: /custom/codex
`
	if err := os.WriteFile(filepath.Join(dir, "aim.local.yaml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Adapters.ClaudeCode.BaseDir != "/custom/claude" {
		t.Errorf("claude_code.base_dir: got %q", cfg.Adapters.ClaudeCode.BaseDir)
	}
	if cfg.Adapters.Cursor.BaseDir != "/custom/cursor" {
		t.Errorf("cursor.base_dir: got %q", cfg.Adapters.Cursor.BaseDir)
	}
	if cfg.Adapters.Codex.BaseDir != "/custom/codex" {
		t.Errorf("codex.base_dir: got %q", cfg.Adapters.Codex.BaseDir)
	}
}

func TestLoad_OnlyCursor(t *testing.T) {
	dir := t.TempDir()
	content := `adapters:
  cursor:
    base_dir: /only/cursor
`
	if err := os.WriteFile(filepath.Join(dir, "aim.local.yaml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Adapters.Cursor.BaseDir != "/only/cursor" {
		t.Errorf("cursor.base_dir: got %q", cfg.Adapters.Cursor.BaseDir)
	}
	if cfg.Adapters.ClaudeCode.BaseDir != "" {
		t.Errorf("claude_code.base_dir should be empty, got %q", cfg.Adapters.ClaudeCode.BaseDir)
	}
	if cfg.Adapters.Codex.BaseDir != "" {
		t.Errorf("codex.base_dir should be empty, got %q", cfg.Adapters.Codex.BaseDir)
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "aim.local.yaml"), []byte(":\nbroken: [yaml"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

func TestLoad_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "aim.local.yaml"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error for empty file: %v", err)
	}
	if cfg.Adapters.ClaudeCode.BaseDir != "" || cfg.Adapters.Cursor.BaseDir != "" || cfg.Adapters.Codex.BaseDir != "" {
		t.Fatal("expected empty config for empty file")
	}
}

func TestSave_Load_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	original := Config{
		Adapters: AdaptersConfig{
			ClaudeCode: AdapterConfig{BaseDir: "/home/user/.claude"},
			Cursor:     AdapterConfig{BaseDir: "/home/user/.cursor"},
			Codex:      AdapterConfig{BaseDir: "/home/user/.codex"},
		},
		Repo:          "git@gitlab.com:axsmak/skills.git",
		SyncedHash:    "abc1234def5678",
		PublishedHash: "fedcba987654",
	}
	if err := Save(dir, original); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("Load after Save failed: %v", err)
	}
	if loaded.Repo != original.Repo {
		t.Errorf("Repo: got %q, want %q", loaded.Repo, original.Repo)
	}
	if loaded.SyncedHash != original.SyncedHash {
		t.Errorf("SyncedHash: got %q, want %q", loaded.SyncedHash, original.SyncedHash)
	}
	if loaded.PublishedHash != original.PublishedHash {
		t.Errorf("PublishedHash: got %q, want %q", loaded.PublishedHash, original.PublishedHash)
	}
	if loaded.Adapters.ClaudeCode.BaseDir != original.Adapters.ClaudeCode.BaseDir {
		t.Errorf("ClaudeCode.BaseDir: got %q, want %q", loaded.Adapters.ClaudeCode.BaseDir, original.Adapters.ClaudeCode.BaseDir)
	}
}

func TestLoad_OldFormat(t *testing.T) {
	dir := t.TempDir()
	content := `adapters:
  claude_code:
    base_dir: /old/claude
`
	if err := os.WriteFile(filepath.Join(dir, "aim.local.yaml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error loading old format: %v", err)
	}
	if cfg.Repo != "" {
		t.Errorf("Repo should be empty for old format, got %q", cfg.Repo)
	}
	if cfg.SyncedHash != "" {
		t.Errorf("SyncedHash should be empty for old format, got %q", cfg.SyncedHash)
	}
	if cfg.PublishedHash != "" {
		t.Errorf("PublishedHash should be empty for old format, got %q", cfg.PublishedHash)
	}
	if cfg.Adapters.ClaudeCode.BaseDir != "/old/claude" {
		t.Errorf("ClaudeCode.BaseDir: got %q", cfg.Adapters.ClaudeCode.BaseDir)
	}
}

func TestGetMCPEnvForServer(t *testing.T) {
	cfg := Config{
		MCPEnv: map[string]string{
			"server-a.API_KEY":  "key-a",
			"server-a.BASE_URL": "https://a.example.com",
			"server-b.TOKEN":    "tok-b",
		},
	}

	got := cfg.GetMCPEnvForServer("server-a")
	if got["API_KEY"] != "key-a" {
		t.Errorf("API_KEY = %q, want key-a", got["API_KEY"])
	}
	if got["BASE_URL"] != "https://a.example.com" {
		t.Errorf("BASE_URL = %q", got["BASE_URL"])
	}
	if _, ok := got["TOKEN"]; ok {
		t.Error("server-b.TOKEN should not appear in server-a env")
	}
}

func TestGetMCPEnvForServer_Empty(t *testing.T) {
	cfg := Config{}
	got := cfg.GetMCPEnvForServer("server-a")
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

func TestSetMCPEnv(t *testing.T) {
	cfg := Config{}
	cfg.SetMCPEnv("my-server", "API_KEY", "secret")
	cfg.SetMCPEnv("my-server", "BASE_URL", "https://api.example.com")

	if cfg.MCPEnv["my-server.API_KEY"] != "secret" {
		t.Errorf("my-server.API_KEY = %q", cfg.MCPEnv["my-server.API_KEY"])
	}
	if cfg.MCPEnv["my-server.BASE_URL"] != "https://api.example.com" {
		t.Errorf("my-server.BASE_URL = %q", cfg.MCPEnv["my-server.BASE_URL"])
	}
}

func TestMCPEnv_SaveLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		MCPEnv: map[string]string{
			"test-server.API_KEY": "my-secret-key",
		},
	}
	if err := Save(dir, cfg); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded.MCPEnv["test-server.API_KEY"] != "my-secret-key" {
		t.Errorf("MCPEnv round-trip: got %q", loaded.MCPEnv["test-server.API_KEY"])
	}
}

func TestLoad_NewFields(t *testing.T) {
	dir := t.TempDir()
	content := `repo: git@github.com:user/skills.git
synced_hash: aabbcc112233
published_hash: ddeeff445566
adapters:
  cursor:
    base_dir: /custom/cursor
`
	if err := os.WriteFile(filepath.Join(dir, "aim.local.yaml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Repo != "git@github.com:user/skills.git" {
		t.Errorf("Repo: got %q", cfg.Repo)
	}
	if cfg.SyncedHash != "aabbcc112233" {
		t.Errorf("SyncedHash: got %q", cfg.SyncedHash)
	}
	if cfg.PublishedHash != "ddeeff445566" {
		t.Errorf("PublishedHash: got %q", cfg.PublishedHash)
	}
	if cfg.Adapters.Cursor.BaseDir != "/custom/cursor" {
		t.Errorf("Cursor.BaseDir: got %q", cfg.Adapters.Cursor.BaseDir)
	}
}

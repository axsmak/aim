package repoconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_FileExists(t *testing.T) {
	dir := t.TempDir()
	content := "skill_paths:\n  claude-code: ~/.claude/skills\n  cursor: ~/.cursor/skills\n"
	if err := os.WriteFile(filepath.Join(dir, "aim.yaml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SkillPaths["claude-code"] != "~/.claude/skills" {
		t.Errorf("claude-code = %q, want ~/.claude/skills", cfg.SkillPaths["claude-code"])
	}
	if cfg.SkillPaths["cursor"] != "~/.cursor/skills" {
		t.Errorf("cursor = %q, want ~/.cursor/skills", cfg.SkillPaths["cursor"])
	}
}

func TestLoad_FileNotExists(t *testing.T) {
	dir := t.TempDir()

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if len(cfg.SkillPaths) != 0 {
		t.Errorf("expected empty SkillPaths, got: %v", cfg.SkillPaths)
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "aim.yaml"), []byte("{bad: [yaml"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

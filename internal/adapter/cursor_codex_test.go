package adapter_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/axsmak/aim/internal/adapter"
	"github.com/axsmak/aim/internal/skill"
)

// --- CursorAdapter ---

func TestCursorAdapter_Name(t *testing.T) {
	a := adapter.NewCursorAdapter("")
	if got := a.Name(); got != "cursor" {
		t.Errorf("Name() = %q, want %q", got, "cursor")
	}
}

func TestCursorAdapter_Detect_Found(t *testing.T) {
	home := t.TempDir()
	cursorDir := filepath.Join(home, ".cursor")
	if err := os.Mkdir(cursorDir, 0755); err != nil {
		t.Fatal(err)
	}

	a := adapter.NewCursorAdapter("")
	got, ok := a.Detect(home)
	if !ok {
		t.Fatal("Detect() ok = false, want true")
	}
	if got != cursorDir {
		t.Errorf("Detect() path = %q, want %q", got, cursorDir)
	}
}

func TestCursorAdapter_Detect_NotFound(t *testing.T) {
	home := t.TempDir()

	a := adapter.NewCursorAdapter("")
	got, ok := a.Detect(home)
	if ok {
		t.Fatal("Detect() ok = true, want false")
	}
	if got != "" {
		t.Errorf("Detect() path = %q, want empty string", got)
	}
}

func TestCursorAdapter_Detect_ConfigBaseDirOverrides(t *testing.T) {
	home := t.TempDir()
	customDir := t.TempDir()

	a := adapter.NewCursorAdapter(customDir)
	got, ok := a.Detect(home)
	if !ok {
		t.Fatal("Detect() ok = false, want true")
	}
	if got != customDir {
		t.Errorf("Detect() path = %q, want %q", got, customDir)
	}
}

func TestCursorAdapter_InstallSkill_WritesCorrectPath(t *testing.T) {
	home := t.TempDir()
	baseDir := filepath.Join(home, ".cursor")
	if err := os.Mkdir(baseDir, 0755); err != nil {
		t.Fatal(err)
	}

	s := skill.Skill{Name: "my-skill", Raw: []byte("---\nname: my-skill\n---\n# Body")}
	a := adapter.NewCursorAdapter("")
	if err := a.InstallSkill(s, baseDir); err != nil {
		t.Fatalf("InstallSkill() error = %v", err)
	}

	want := filepath.Join(baseDir, "skills", "my-skill", "SKILL.md")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("expected file not found at %s: %v", want, err)
	}
}

func TestCursorAdapter_InstallSkill_CreatesNestedDirectories(t *testing.T) {
	home := t.TempDir()
	baseDir := filepath.Join(home, ".cursor")

	s := skill.Skill{Name: "nested-skill", Raw: []byte("---\nname: nested-skill\n---\nbody")}
	a := adapter.NewCursorAdapter("")
	if err := a.InstallSkill(s, baseDir); err != nil {
		t.Fatalf("InstallSkill() error = %v", err)
	}

	want := filepath.Join(baseDir, "skills", "nested-skill", "SKILL.md")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("expected file not found at %s: %v", want, err)
	}
}

// --- CodexAdapter ---

func TestCodexAdapter_Name(t *testing.T) {
	a := adapter.NewCodexAdapter("")
	if got := a.Name(); got != "codex" {
		t.Errorf("Name() = %q, want %q", got, "codex")
	}
}

func TestCodexAdapter_Detect_Found(t *testing.T) {
	home := t.TempDir()
	codexDir := filepath.Join(home, ".codex")
	if err := os.Mkdir(codexDir, 0755); err != nil {
		t.Fatal(err)
	}

	a := adapter.NewCodexAdapter("")
	got, ok := a.Detect(home)
	if !ok {
		t.Fatal("Detect() ok = false, want true")
	}
	if got != codexDir {
		t.Errorf("Detect() path = %q, want %q", got, codexDir)
	}
}

func TestCodexAdapter_Detect_NotFound(t *testing.T) {
	home := t.TempDir()

	a := adapter.NewCodexAdapter("")
	got, ok := a.Detect(home)
	if ok {
		t.Fatal("Detect() ok = true, want false")
	}
	if got != "" {
		t.Errorf("Detect() path = %q, want empty string", got)
	}
}

func TestCodexAdapter_Detect_ConfigBaseDirOverrides(t *testing.T) {
	home := t.TempDir()
	customDir := t.TempDir()

	a := adapter.NewCodexAdapter(customDir)
	got, ok := a.Detect(home)
	if !ok {
		t.Fatal("Detect() ok = false, want true")
	}
	if got != customDir {
		t.Errorf("Detect() path = %q, want %q", got, customDir)
	}
}

func TestCodexAdapter_InstallSkill_WritesCorrectPath(t *testing.T) {
	home := t.TempDir()
	baseDir := filepath.Join(home, ".codex")
	if err := os.Mkdir(baseDir, 0755); err != nil {
		t.Fatal(err)
	}

	s := skill.Skill{Name: "my-skill", Raw: []byte("---\nname: my-skill\n---\n# Body")}
	a := adapter.NewCodexAdapter("")
	if err := a.InstallSkill(s, baseDir); err != nil {
		t.Fatalf("InstallSkill() error = %v", err)
	}

	want := filepath.Join(baseDir, "skills", "my-skill", "SKILL.md")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("expected file not found at %s: %v", want, err)
	}
}

func TestCodexAdapter_InstallSkill_CreatesNestedDirectories(t *testing.T) {
	home := t.TempDir()
	baseDir := filepath.Join(home, ".codex")

	s := skill.Skill{Name: "nested-skill", Raw: []byte("---\nname: nested-skill\n---\nbody")}
	a := adapter.NewCodexAdapter("")
	if err := a.InstallSkill(s, baseDir); err != nil {
		t.Fatalf("InstallSkill() error = %v", err)
	}

	want := filepath.Join(baseDir, "skills", "nested-skill", "SKILL.md")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("expected file not found at %s: %v", want, err)
	}
}

func TestCursorAdapter_InstallSkill_FolderSkill_CopiesRefs(t *testing.T) {
	sourceDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(sourceDir, "agent-patterns.md"), []byte("# Patterns\nContent."), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "delegation.md"), []byte("# Delegation\nNotes."), 0644); err != nil {
		t.Fatal(err)
	}

	s := skill.Skill{
		Name:      "write-agent",
		Raw:       []byte("---\nname: write-agent\ndescription: Write agent definitions\n---\n\n# Role\nHelps write agents.\n"),
		SourceDir: sourceDir,
		RefFiles:  []string{"agent-patterns.md", "delegation.md"},
	}

	home := t.TempDir()
	baseDir := filepath.Join(home, ".cursor")

	a := adapter.NewCursorAdapter("")
	if err := a.InstallSkill(s, baseDir); err != nil {
		t.Fatalf("InstallSkill() error = %v", err)
	}

	destDir := filepath.Join(baseDir, "skills", "write-agent")

	if _, err := os.Stat(filepath.Join(destDir, "SKILL.md")); err != nil {
		t.Errorf("SKILL.md not found: %v", err)
	}
	for _, ref := range []string{"agent-patterns.md", "delegation.md"} {
		dest := filepath.Join(destDir, ref)
		got, err := os.ReadFile(dest)
		if err != nil {
			t.Errorf("ref file %s not found: %v", ref, err)
			continue
		}
		src, _ := os.ReadFile(filepath.Join(sourceDir, ref))
		if string(got) != string(src) {
			t.Errorf("ref file %s content mismatch: got %q, want %q", ref, got, src)
		}
	}
}

func TestCodexAdapter_InstallSkill_FolderSkill_CopiesRefs(t *testing.T) {
	sourceDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(sourceDir, "agent-patterns.md"), []byte("# Patterns\nContent."), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "delegation.md"), []byte("# Delegation\nNotes."), 0644); err != nil {
		t.Fatal(err)
	}

	s := skill.Skill{
		Name:      "write-agent",
		Raw:       []byte("---\nname: write-agent\ndescription: Write agent definitions\n---\n\n# Role\nHelps write agents.\n"),
		SourceDir: sourceDir,
		RefFiles:  []string{"agent-patterns.md", "delegation.md"},
	}

	home := t.TempDir()
	baseDir := filepath.Join(home, ".codex")

	a := adapter.NewCodexAdapter("")
	if err := a.InstallSkill(s, baseDir); err != nil {
		t.Fatalf("InstallSkill() error = %v", err)
	}

	destDir := filepath.Join(baseDir, "skills", "write-agent")

	if _, err := os.Stat(filepath.Join(destDir, "SKILL.md")); err != nil {
		t.Errorf("SKILL.md not found: %v", err)
	}
	for _, ref := range []string{"agent-patterns.md", "delegation.md"} {
		dest := filepath.Join(destDir, ref)
		got, err := os.ReadFile(dest)
		if err != nil {
			t.Errorf("ref file %s not found: %v", ref, err)
			continue
		}
		src, _ := os.ReadFile(filepath.Join(sourceDir, ref))
		if string(got) != string(src) {
			t.Errorf("ref file %s content mismatch: got %q, want %q", ref, got, src)
		}
	}
}

// --- NewClaudeCodeAdapter constructor ---

func TestNewClaudeCodeAdapter_ConfigBaseDirOverrides(t *testing.T) {
	home := t.TempDir()
	customDir := t.TempDir()

	a := adapter.NewClaudeCodeAdapter(customDir)
	got, ok := a.Detect(home)
	if !ok {
		t.Fatal("Detect() ok = false, want true")
	}
	if got != customDir {
		t.Errorf("Detect() path = %q, want %q", got, customDir)
	}
}

func TestNewClaudeCodeAdapter_EmptyConfigUsesDefault(t *testing.T) {
	home := t.TempDir()
	claudeDir := filepath.Join(home, ".claude")
	if err := os.Mkdir(claudeDir, 0755); err != nil {
		t.Fatal(err)
	}

	a := adapter.NewClaudeCodeAdapter("")
	got, ok := a.Detect(home)
	if !ok {
		t.Fatal("Detect() ok = false, want true")
	}
	if got != claudeDir {
		t.Errorf("Detect() path = %q, want %q", got, claudeDir)
	}
}

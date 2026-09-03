package adapter_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/axsmak/aim/internal/adapter"
)

func TestClaudeCodeAdapter_ScanSkills_TwoFiles(t *testing.T) {
	baseDir := t.TempDir()
	skillsDir := filepath.Join(baseDir, "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "alpha.md"), []byte("# alpha"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "beta.md"), []byte("# beta"), 0644); err != nil {
		t.Fatal(err)
	}

	a := adapter.NewClaudeCodeAdapter("")
	got, err := a.ScanSkills(baseDir)
	if err != nil {
		t.Fatalf("ScanSkills() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ScanSkills() returned %d skills, want 2", len(got))
	}

	byName := make(map[string]adapter.DiscoveredSkill, len(got))
	for _, s := range got {
		byName[s.Name] = s
	}

	for _, name := range []string{"alpha", "beta"} {
		s, ok := byName[name]
		if !ok {
			t.Errorf("missing skill %q", name)
			continue
		}
		if s.Source != "claude-code" {
			t.Errorf("skill %q Source = %q, want claude-code", name, s.Source)
		}
		if len(s.Raw) == 0 {
			t.Errorf("skill %q Raw is empty", name)
		}
		if s.IsFolder {
			t.Errorf("skill %q IsFolder = true, want false (flat file)", name)
		}
	}
}

func TestClaudeCodeAdapter_ScanSkills_DirectoryMissing(t *testing.T) {
	baseDir := t.TempDir()

	a := adapter.NewClaudeCodeAdapter("")
	got, err := a.ScanSkills(baseDir)
	if err != nil {
		t.Fatalf("ScanSkills() error = %v, want nil", err)
	}
	if got != nil {
		t.Errorf("ScanSkills() = %v, want nil", got)
	}
}

func TestClaudeCodeAdapter_ScanSkills_EmptyDirectory(t *testing.T) {
	baseDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(baseDir, "skills"), 0755); err != nil {
		t.Fatal(err)
	}

	a := adapter.NewClaudeCodeAdapter("")
	got, err := a.ScanSkills(baseDir)
	if err != nil {
		t.Fatalf("ScanSkills() error = %v, want nil", err)
	}
	if got != nil {
		t.Errorf("ScanSkills() = %v, want nil", got)
	}
}

func TestCursorAdapter_ScanSkills_SubdirFormat(t *testing.T) {
	baseDir := t.TempDir()
	skillDir := filepath.Join(baseDir, "skills", "hello")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := []byte("---\nname: hello\ndescription: greet\n---\n\nHello!")
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), content, 0644); err != nil {
		t.Fatal(err)
	}

	a := adapter.NewCursorAdapter("")
	got, err := a.ScanSkills(baseDir)
	if err != nil {
		t.Fatalf("ScanSkills() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ScanSkills() returned %d skills, want 1", len(got))
	}
	if got[0].Name != "hello" {
		t.Errorf("Name = %q, want hello", got[0].Name)
	}
	if got[0].Source != "cursor" {
		t.Errorf("Source = %q, want cursor", got[0].Source)
	}
	if !got[0].IsFolder {
		t.Error("IsFolder = false, want true (subdir SKILL.md)")
	}
}

func TestCursorAdapter_ScanSkills_DirectoryMissing(t *testing.T) {
	a := adapter.NewCursorAdapter("")
	got, err := a.ScanSkills(t.TempDir())
	if err != nil {
		t.Fatalf("ScanSkills() error = %v, want nil", err)
	}
	if got != nil {
		t.Errorf("ScanSkills() = %v, want nil", got)
	}
}

func TestCodexAdapter_ScanSkills_SubdirFormat(t *testing.T) {
	baseDir := t.TempDir()
	skillDir := filepath.Join(baseDir, "skills", "hello")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := []byte("---\nname: hello\ndescription: greet\n---\n\nHello!")
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), content, 0644); err != nil {
		t.Fatal(err)
	}

	a := adapter.NewCodexAdapter("")
	got, err := a.ScanSkills(baseDir)
	if err != nil {
		t.Fatalf("ScanSkills() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ScanSkills() returned %d skills, want 1", len(got))
	}
	if got[0].Name != "hello" {
		t.Errorf("Name = %q, want hello", got[0].Name)
	}
	if got[0].Source != "codex" {
		t.Errorf("Source = %q, want codex", got[0].Source)
	}
	if !got[0].IsFolder {
		t.Error("IsFolder = false, want true (subdir SKILL.md)")
	}
}

func TestCodexAdapter_ScanSkills_DirectoryMissing(t *testing.T) {
	a := adapter.NewCodexAdapter("")
	got, err := a.ScanSkills(t.TempDir())
	if err != nil {
		t.Fatalf("ScanSkills() error = %v, want nil", err)
	}
	if got != nil {
		t.Errorf("ScanSkills() = %v, want nil", got)
	}
}

func TestClaudeCodeAdapter_ScanSkills_SubdirFormat(t *testing.T) {
	baseDir := t.TempDir()
	skillDir := filepath.Join(baseDir, "skills", "my-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := []byte("---\nname: my-skill\ndescription: test\n---\n\nBody.")
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), content, 0644); err != nil {
		t.Fatal(err)
	}

	a := adapter.NewClaudeCodeAdapter("")
	got, err := a.ScanSkills(baseDir)
	if err != nil {
		t.Fatalf("ScanSkills() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ScanSkills() returned %d skills, want 1", len(got))
	}
	if got[0].Name != "my-skill" {
		t.Errorf("Name = %q, want my-skill", got[0].Name)
	}
	if !got[0].IsFolder {
		t.Error("IsFolder = false, want true (subdir SKILL.md)")
	}
}

func TestClaudeCodeAdapter_ScanSkills_FlatTakesPrecedence(t *testing.T) {
	baseDir := t.TempDir()
	skillsDir := filepath.Join(baseDir, "skills")

	// flat file
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatal(err)
	}
	flatContent := []byte("flat content")
	if err := os.WriteFile(filepath.Join(skillsDir, "dup.md"), flatContent, 0644); err != nil {
		t.Fatal(err)
	}

	// subdir file with same name
	subdirPath := filepath.Join(skillsDir, "dup")
	if err := os.MkdirAll(subdirPath, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subdirPath, "SKILL.md"), []byte("subdir content"), 0644); err != nil {
		t.Fatal(err)
	}

	a := adapter.NewClaudeCodeAdapter("")
	got, err := a.ScanSkills(baseDir)
	if err != nil {
		t.Fatalf("ScanSkills() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ScanSkills() returned %d skills, want 1", len(got))
	}
	if string(got[0].Raw) != string(flatContent) {
		t.Error("flat file must take precedence over subdir SKILL.md")
	}
	if got[0].IsFolder {
		t.Error("IsFolder = true, want false (flat file takes precedence)")
	}
}

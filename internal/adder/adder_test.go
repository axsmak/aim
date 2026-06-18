package adder

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/axsmak/aim/internal/importer"
)

const validSkill = `---
name: my-skill
description: test skill description
---

Skill body content here.
`

const noNameSkill = `---
description: test skill description
---

Skill body content here.
`

func TestAdd_ValidSkill_FileWritten(t *testing.T) {
	dir := t.TempDir()
	result, err := Add("skill", bytes.NewReader([]byte(validSkill)), AddOptions{WorkDir: dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "my-skill" {
		t.Fatalf("expected Name 'my-skill', got %q", result.Name)
	}
	if result.Identical {
		t.Fatal("expected Identical=false for new write")
	}
	dest := filepath.Join(dir, "skills", "my-skill.md")
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("file not written: %v", err)
	}
	if string(got) != validSkill {
		t.Fatalf("file content mismatch")
	}
}

func TestAdd_SkillFromBytesReader(t *testing.T) {
	dir := t.TempDir()
	_, err := Add("skill", bytes.NewReader([]byte(validSkill)), AddOptions{WorkDir: dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAdd_MissingNameNoFlag_Error(t *testing.T) {
	dir := t.TempDir()
	_, err := Add("skill", bytes.NewReader([]byte(noNameSkill)), AddOptions{WorkDir: dir})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestAdd_ConflictWithoutOverwrite_ConflictError(t *testing.T) {
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(skillsDir, "my-skill.md")
	if err := os.WriteFile(dest, []byte("different content"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Add("skill", bytes.NewReader([]byte(validSkill)), AddOptions{WorkDir: dir})
	if err == nil {
		t.Fatal("expected ConflictError, got nil")
	}
	var ce importer.ConflictError
	if !errors.As(err, &ce) {
		t.Fatalf("expected ConflictError, got %T: %v", err, err)
	}
}

func TestAdd_ConflictWithOverwrite_Success(t *testing.T) {
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(skillsDir, "my-skill.md")
	if err := os.WriteFile(dest, []byte("different content"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Add("skill", bytes.NewReader([]byte(validSkill)), AddOptions{WorkDir: dir, Overwrite: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, _ := os.ReadFile(dest)
	if string(got) != validSkill {
		t.Fatalf("file not overwritten correctly")
	}
}

func TestAdd_IdenticalContent_NoOpResult(t *testing.T) {
	dir := t.TempDir()
	// First add — new write.
	result1, err := Add("skill", bytes.NewReader([]byte(validSkill)), AddOptions{WorkDir: dir})
	if err != nil {
		t.Fatalf("first add unexpected error: %v", err)
	}
	if result1.Identical {
		t.Fatal("first add must not be identical")
	}

	// Second add with identical content — no-op.
	result2, err := Add("skill", bytes.NewReader([]byte(validSkill)), AddOptions{WorkDir: dir})
	if err != nil {
		t.Fatalf("second add unexpected error: %v", err)
	}
	if !result2.Identical {
		t.Fatal("second add with identical content must set Identical=true")
	}
	if result2.Name != "my-skill" {
		t.Fatalf("expected Name 'my-skill', got %q", result2.Name)
	}
}

const validFolderSkillContent = `---
name: write-agent
description: Write agent definitions
---

Body content here.
`

func writeFolderSkillSrc(t *testing.T, parentDir, skillName, content string) string {
	t.Helper()
	dir := filepath.Join(parentDir, skillName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
	return dir
}

func TestAddSkillDir_DirPath_WritesFolderSkill(t *testing.T) {
	srcDir := writeFolderSkillSrc(t, t.TempDir(), "write-agent", validFolderSkillContent)
	if err := os.WriteFile(filepath.Join(srcDir, "patterns.md"), []byte("# Patterns"), 0644); err != nil {
		t.Fatalf("write ref file: %v", err)
	}

	workDir := t.TempDir()
	result, err := AddSkillDir(srcDir, AddOptions{WorkDir: workDir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "write-agent" {
		t.Fatalf("expected Name 'write-agent', got %q", result.Name)
	}

	destDir := filepath.Join(workDir, "skills", "write-agent")
	if _, statErr := os.Stat(filepath.Join(destDir, "SKILL.md")); statErr != nil {
		t.Errorf("SKILL.md not written: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(destDir, "patterns.md")); statErr != nil {
		t.Errorf("reference file not copied: %v", statErr)
	}
}

func TestAddSkillDir_MissingSkillMD_Error(t *testing.T) {
	emptyDir := t.TempDir()
	_, err := AddSkillDir(emptyDir, AddOptions{WorkDir: t.TempDir()})
	if err == nil {
		t.Fatal("expected error for directory without SKILL.md, got nil")
	}
	if strings.Contains(err.Error(), "is a directory") {
		t.Errorf("expected a clear error, got system-style message: %v", err)
	}
}

func TestAddSkillDir_NameOverride(t *testing.T) {
	srcDir := writeFolderSkillSrc(t, t.TempDir(), "write-agent", validFolderSkillContent)

	workDir := t.TempDir()
	result, err := AddSkillDir(srcDir, AddOptions{WorkDir: workDir, Name: "renamed"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "renamed" {
		t.Fatalf("expected Name 'renamed', got %q", result.Name)
	}
	if _, statErr := os.Stat(filepath.Join(workDir, "skills", "renamed", "SKILL.md")); statErr != nil {
		t.Errorf("SKILL.md not written under renamed dir: %v", statErr)
	}
}

func TestAddSkillDir_ConflictWithoutOverwrite_ConflictError(t *testing.T) {
	srcDir := writeFolderSkillSrc(t, t.TempDir(), "write-agent", validFolderSkillContent)

	workDir := t.TempDir()
	existingDir := filepath.Join(workDir, "skills", "write-agent")
	if err := os.MkdirAll(existingDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(existingDir, "SKILL.md"), []byte("different content"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := AddSkillDir(srcDir, AddOptions{WorkDir: workDir})
	if err == nil {
		t.Fatal("expected ConflictError, got nil")
	}
	var ce importer.ConflictError
	if !errors.As(err, &ce) {
		t.Fatalf("expected ConflictError, got %T: %v", err, err)
	}
}

func TestAddSkillDir_ConflictWithOverwrite_Success(t *testing.T) {
	srcDir := writeFolderSkillSrc(t, t.TempDir(), "write-agent", validFolderSkillContent)

	workDir := t.TempDir()
	existingDir := filepath.Join(workDir, "skills", "write-agent")
	if err := os.MkdirAll(existingDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(existingDir, "SKILL.md"), []byte("different content"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := AddSkillDir(srcDir, AddOptions{WorkDir: workDir, Overwrite: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(existingDir, "SKILL.md"))
	if string(got) != validFolderSkillContent {
		t.Fatalf("file not overwritten correctly")
	}
}

func TestAddSkillDir_IdenticalContent_NoOpResult(t *testing.T) {
	srcDir := writeFolderSkillSrc(t, t.TempDir(), "write-agent", validFolderSkillContent)
	workDir := t.TempDir()

	result1, err := AddSkillDir(srcDir, AddOptions{WorkDir: workDir})
	if err != nil {
		t.Fatalf("first add unexpected error: %v", err)
	}
	if result1.Identical {
		t.Fatal("first add must not be identical")
	}

	result2, err := AddSkillDir(srcDir, AddOptions{WorkDir: workDir})
	if err != nil {
		t.Fatalf("second add unexpected error: %v", err)
	}
	if !result2.Identical {
		t.Fatal("second add with identical content must set Identical=true")
	}
}

func TestAdd_MCPType_InvalidYAML_Error(t *testing.T) {
	_, err := Add("mcp", bytes.NewReader([]byte("!!invalid: yaml: {")), AddOptions{WorkDir: t.TempDir()})
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

func TestAdd_UnknownType_Error(t *testing.T) {
	_, err := Add("unknown", bytes.NewReader(nil), AddOptions{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unknown item type") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAdd_EmptyBytes_Error(t *testing.T) {
	dir := t.TempDir()
	_, err := Add("skill", bytes.NewReader([]byte{}), AddOptions{WorkDir: dir})
	if err == nil {
		t.Fatal("expected error for empty input, got nil")
	}
}

func TestAdd_MCPWithSecrets_HasSecretsTrue(t *testing.T) {
	dir := t.TempDir()
	result, err := Add("mcp", bytes.NewReader([]byte(validMCPWithEnvValues)), AddOptions{WorkDir: dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.HasSecrets {
		t.Fatal("expected HasSecrets=true when MCP has env values")
	}
	if result.Identical {
		t.Fatal("expected Identical=false for new write")
	}
}

func TestAdd_MCPNoSecrets_HasSecretsFalse(t *testing.T) {
	dir := t.TempDir()
	result, err := Add("mcp", bytes.NewReader([]byte(validMCPNoEnvValues)), AddOptions{WorkDir: dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.HasSecrets {
		t.Fatal("expected HasSecrets=false when MCP has no env values")
	}
}

func TestAdd_MCPIdenticalContent_NoOp(t *testing.T) {
	dir := t.TempDir()
	// First add — new write.
	result1, err := Add("mcp", bytes.NewReader([]byte(validMCPNoEnvValues)), AddOptions{WorkDir: dir})
	if err != nil {
		t.Fatalf("first add unexpected error: %v", err)
	}
	if result1.Identical {
		t.Fatal("first add must not be identical")
	}

	// Second add with identical content — no-op.
	result2, err := Add("mcp", bytes.NewReader([]byte(validMCPNoEnvValues)), AddOptions{WorkDir: dir})
	if err != nil {
		t.Fatalf("second add unexpected error: %v", err)
	}
	if !result2.Identical {
		t.Fatal("second add with identical content must set Identical=true")
	}
}

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

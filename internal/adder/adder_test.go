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
	err := Add("skill", bytes.NewReader([]byte(validSkill)), AddOptions{WorkDir: dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
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
	err := Add("skill", bytes.NewReader([]byte(validSkill)), AddOptions{WorkDir: dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAdd_MissingNameNoFlag_Error(t *testing.T) {
	dir := t.TempDir()
	err := Add("skill", bytes.NewReader([]byte(noNameSkill)), AddOptions{WorkDir: dir})
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

	err := Add("skill", bytes.NewReader([]byte(validSkill)), AddOptions{WorkDir: dir})
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

	err := Add("skill", bytes.NewReader([]byte(validSkill)), AddOptions{WorkDir: dir, Overwrite: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, _ := os.ReadFile(dest)
	if string(got) != validSkill {
		t.Fatalf("file not overwritten correctly")
	}
}

func TestAdd_MCPType_InvalidYAML_Error(t *testing.T) {
	err := Add("mcp", bytes.NewReader([]byte("!!invalid: yaml: {")), AddOptions{WorkDir: t.TempDir()})
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

func TestAdd_UnknownType_Error(t *testing.T) {
	err := Add("unknown", bytes.NewReader(nil), AddOptions{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unknown item type") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAdd_EmptyBytes_Error(t *testing.T) {
	dir := t.TempDir()
	err := Add("skill", bytes.NewReader([]byte{}), AddOptions{WorkDir: dir})
	if err == nil {
		t.Fatal("expected error for empty input, got nil")
	}
}

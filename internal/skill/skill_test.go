package skill_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/axsmak/aim/internal/skill"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writeFile: %v", err)
	}
	return path
}

func TestReadAll_ValidSkill(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "hello.md", "---\nname: hello\ndescription: Says hello\n---\n\n# Role\nSay hello.\n")

	valid, invalid, err := skill.ReadAll(dir)
	if err != nil {
		t.Fatalf("unexpected system error: %v", err)
	}
	if len(invalid) != 0 {
		t.Fatalf("expected no invalid skills, got: %v", invalid)
	}
	if len(valid) != 1 {
		t.Fatalf("expected 1 valid skill, got %d", len(valid))
	}
	s := valid[0]
	if s.Name != "hello" {
		t.Errorf("Name = %q, want %q", s.Name, "hello")
	}
	if s.Description != "Says hello" {
		t.Errorf("Description = %q, want %q", s.Description, "Says hello")
	}
	if s.Body == "" {
		t.Error("Body must not be empty")
	}
	if len(s.Raw) == 0 {
		t.Error("Raw must not be empty")
	}
	if s.FilePath == "" {
		t.Error("FilePath must not be empty")
	}
}

func TestReadAll_MissingName(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "no-name.md", "---\ndescription: Does stuff\n---\n\n# Role\nContent.\n")

	valid, invalid, err := skill.ReadAll(dir)
	if err != nil {
		t.Fatalf("unexpected system error: %v", err)
	}
	if len(valid) != 0 {
		t.Errorf("expected 0 valid skills, got %d", len(valid))
	}
	if len(invalid) != 1 {
		t.Fatalf("expected 1 invalid skill, got %d", len(invalid))
	}
	if invalid[0].Field != "name" {
		t.Errorf("Field = %q, want %q", invalid[0].Field, "name")
	}
}

func TestReadAll_MissingDescription(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "no-desc.md", "---\nname: my-skill\n---\n\n# Role\nContent.\n")

	valid, invalid, err := skill.ReadAll(dir)
	if err != nil {
		t.Fatalf("unexpected system error: %v", err)
	}
	if len(valid) != 0 {
		t.Errorf("expected 0 valid, got %d", len(valid))
	}
	if len(invalid) != 1 || invalid[0].Field != "description" {
		t.Errorf("expected ValidationError on field 'description', got: %v", invalid)
	}
}

func TestReadAll_EmptyBody(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "empty-body.md", "---\nname: my-skill\ndescription: Does stuff\n---\n\n   \n")

	_, invalid, err := skill.ReadAll(dir)
	if err != nil {
		t.Fatalf("unexpected system error: %v", err)
	}
	if len(invalid) != 1 || invalid[0].Field != "body" {
		t.Errorf("expected ValidationError on field 'body', got: %v", invalid)
	}
}

func TestReadAll_NonMdFilesIgnored(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "valid.md", "---\nname: v\ndescription: D\n---\n\nBody.\n")
	writeFile(t, dir, "readme.txt", "not a skill")
	writeFile(t, dir, "script.sh", "#!/bin/bash")

	valid, invalid, err := skill.ReadAll(dir)
	if err != nil {
		t.Fatalf("unexpected system error: %v", err)
	}
	if len(invalid) != 0 {
		t.Errorf("expected 0 invalid, got %v", invalid)
	}
	if len(valid) != 1 {
		t.Errorf("expected 1 valid skill, got %d", len(valid))
	}
}

func TestReadAll_DirNotExist(t *testing.T) {
	_, _, err := skill.ReadAll("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Error("expected system error for nonexistent directory, got nil")
	}
}

func TestReadAll_MultipleSkills(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.md", "---\nname: a\ndescription: Skill A\n---\n\nBody A.\n")
	writeFile(t, dir, "b.md", "---\nname: b\ndescription: Skill B\n---\n\nBody B.\n")
	writeFile(t, dir, "bad.md", "---\nname: bad\n---\n\nBody.\n")

	valid, invalid, err := skill.ReadAll(dir)
	if err != nil {
		t.Fatalf("unexpected system error: %v", err)
	}
	if len(valid) != 2 {
		t.Errorf("expected 2 valid skills, got %d", len(valid))
	}
	if len(invalid) != 1 {
		t.Errorf("expected 1 invalid skill, got %d", len(invalid))
	}
}

func TestReadAll_BodyWithDashDelimiters(t *testing.T) {
	// Body may contain --- lines; only the first closing --- ends frontmatter.
	dir := t.TempDir()
	writeFile(t, dir, "dashes.md", "---\nname: dashes\ndescription: Test dashes in body\n---\n\n# Section\n---\nMore content.\n")

	valid, invalid, err := skill.ReadAll(dir)
	if err != nil {
		t.Fatalf("unexpected system error: %v", err)
	}
	if len(invalid) != 0 {
		t.Errorf("expected 0 invalid, got: %v", invalid)
	}
	if len(valid) != 1 {
		t.Fatalf("expected 1 valid skill, got %d", len(valid))
	}
	if valid[0].Name != "dashes" {
		t.Errorf("Name = %q, want %q", valid[0].Name, "dashes")
	}
}

func TestValidationError_Error(t *testing.T) {
	e := skill.ValidationError{
		FilePath: "skills/foo.md",
		Field:    "name",
		Message:  "required",
	}
	got := e.Error()
	if got != "skills/foo.md: name: required" {
		t.Errorf("Error() = %q", got)
	}
}

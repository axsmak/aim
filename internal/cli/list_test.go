package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunList_MissingDir(t *testing.T) {
	var out, errOut bytes.Buffer
	err := runList("/nonexistent/skills", &out, &errOut)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(out.String(), "No skills found") {
		t.Errorf("expected 'No skills found', got: %q", out.String())
	}
}

func TestRunList_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	err := runList(skillsDir, &out, &errOut)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "No skills found") {
		t.Errorf("expected 'No skills found', got: %q", out.String())
	}
}

func TestRunList_ValidSkills(t *testing.T) {
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	os.MkdirAll(skillsDir, 0755)

	writeSkillFile(t, skillsDir, "my-skill.md", "---\nname: my-skill\ndescription: Does useful things\n---\n\nBody here.\n")

	var out, errOut bytes.Buffer
	err := runList(skillsDir, &out, &errOut)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "my-skill") {
		t.Errorf("expected skill name in output: %q", output)
	}
	if !strings.Contains(output, "Does useful things") {
		t.Errorf("expected description in output: %q", output)
	}
	if !strings.Contains(output, "valid") {
		t.Errorf("expected 'valid' status in output: %q", output)
	}
	if !strings.Contains(output, "Total: 1 valid, 0 invalid") {
		t.Errorf("expected total line, got: %q", output)
	}
}

func TestRunList_MixedSkills(t *testing.T) {
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	os.MkdirAll(skillsDir, 0755)

	writeSkillFile(t, skillsDir, "good.md", "---\nname: good\ndescription: A good skill\n---\n\nBody.\n")
	writeSkillFile(t, skillsDir, "no-desc.md", "---\nname: no-desc\n---\n\nBody.\n")

	var out, errOut bytes.Buffer
	err := runList(skillsDir, &out, &errOut)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "good") {
		t.Errorf("expected good skill in output: %q", output)
	}
	if !strings.Contains(output, "invalid: missing description") {
		t.Errorf("expected invalid status in output: %q", output)
	}
	if !strings.Contains(output, "Total: 1 valid, 1 invalid") {
		t.Errorf("expected total line, got: %q", output)
	}
}

func writeSkillFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

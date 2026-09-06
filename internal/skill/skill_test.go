package skill_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
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
	if s.Targets != nil {
		t.Errorf("Targets = %v, want nil when frontmatter has no targets key", s.Targets)
	}
}

func TestReadAll_SkillWithTargets(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "hello.md", "---\nname: hello\ndescription: Says hello\ntargets:\n  - claude-code\n  - cursor\n---\n\n# Role\nSay hello.\n")

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
	want := []string{"claude-code", "cursor"}
	if len(s.Targets) != len(want) {
		t.Fatalf("Targets = %v, want %v", s.Targets, want)
	}
	for i, w := range want {
		if s.Targets[i] != w {
			t.Errorf("Targets[%d] = %q, want %q", i, s.Targets[i], w)
		}
	}
}

func TestReadAll_SkillWithEmptyTargets(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "hello.md", "---\nname: hello\ndescription: Says hello\ntargets: []\n---\n\n# Role\nSay hello.\n")

	valid, invalid, err := skill.ReadAll(dir)
	if err != nil {
		t.Fatalf("unexpected system error: %v", err)
	}
	if len(invalid) != 0 {
		t.Fatalf("expected no invalid skills (empty targets list is valid), got: %v", invalid)
	}
	if len(valid) != 1 {
		t.Fatalf("expected 1 valid skill, got %d", len(valid))
	}
	if len(valid[0].Targets) != 0 {
		t.Errorf("Targets = %v, want empty", valid[0].Targets)
	}
}

func TestReadAll_SkillWithUnknownTarget(t *testing.T) {
	// Unknown environment names are not validated at parse time (ADR-0007, decision 7);
	// the value is preserved as-is and the skill remains valid.
	dir := t.TempDir()
	writeFile(t, dir, "hello.md", "---\nname: hello\ndescription: Says hello\ntargets:\n  - claud-code\n---\n\n# Role\nSay hello.\n")

	valid, invalid, err := skill.ReadAll(dir)
	if err != nil {
		t.Fatalf("unexpected system error: %v", err)
	}
	if len(invalid) != 0 {
		t.Fatalf("expected no invalid skills (unknown target names are not validated), got: %v", invalid)
	}
	if len(valid) != 1 {
		t.Fatalf("expected 1 valid skill, got %d", len(valid))
	}
	if len(valid[0].Targets) != 1 || valid[0].Targets[0] != "claud-code" {
		t.Errorf("Targets = %v, want [claud-code]", valid[0].Targets)
	}
}

func TestReadAll_SkillWithTargets_RawUnchanged(t *testing.T) {
	// Raw must be preserved byte-for-byte; targets must not be stripped from it
	// (ADR-0007, decision 10).
	dir := t.TempDir()
	content := "---\nname: hello\ndescription: Says hello\ntargets:\n  - claude-code\n---\n\n# Role\nSay hello.\n"
	writeFile(t, dir, "hello.md", content)

	valid, _, err := skill.ReadAll(dir)
	if err != nil {
		t.Fatalf("unexpected system error: %v", err)
	}
	if len(valid) != 1 {
		t.Fatalf("expected 1 valid skill, got %d", len(valid))
	}
	if string(valid[0].Raw) != content {
		t.Errorf("Raw = %q, want %q", valid[0].Raw, content)
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

const validFolderSkillContent = "---\nname: write-agent\ndescription: Write agent definitions\n---\n\n# Role\nHelps write agents.\n"

func writeFolderSkill(t *testing.T, skillsDir, skillName, content string) string {
	t.Helper()
	dir := filepath.Join(skillsDir, skillName)
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	writeFile(t, dir, "SKILL.md", content)
	return dir
}

func TestReadAll_FolderSkill_Discovered(t *testing.T) {
	dir := t.TempDir()
	writeFolderSkill(t, dir, "write-agent", validFolderSkillContent)

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
	if s.Name != "write-agent" {
		t.Errorf("Name = %q, want %q", s.Name, "write-agent")
	}
	if s.SourceDir == "" {
		t.Error("SourceDir must not be empty for folder skill")
	}
}

func TestReadAll_FolderSkill_FlatTakesPrecedence(t *testing.T) {
	dir := t.TempDir()
	// Flat file with its own name in frontmatter
	writeFile(t, dir, "write-agent.md", "---\nname: write-agent\ndescription: Flat version\n---\n\nFlat body.\n")
	// Folder skill with same directory name
	writeFolderSkill(t, dir, "write-agent", validFolderSkillContent)

	valid, invalid, err := skill.ReadAll(dir)
	if err != nil {
		t.Fatalf("unexpected system error: %v", err)
	}
	if len(invalid) != 0 {
		t.Fatalf("expected no invalid skills, got: %v", invalid)
	}
	if len(valid) != 1 {
		t.Fatalf("expected exactly 1 valid skill, got %d", len(valid))
	}
	s := valid[0]
	if s.Name != "write-agent" {
		t.Errorf("Name = %q, want %q", s.Name, "write-agent")
	}
	if s.SourceDir != "" {
		t.Errorf("SourceDir = %q, want empty (flat file should win)", s.SourceDir)
	}
}

func TestReadAll_FolderSkill_SkipsFolderWithoutSKILLMD(t *testing.T) {
	dir := t.TempDir()
	// Create a subdirectory with a non-SKILL.md file but no SKILL.md
	orphanDir := filepath.Join(dir, "orphan-folder")
	if err := os.Mkdir(orphanDir, 0755); err != nil {
		t.Fatalf("mkdir orphan-folder: %v", err)
	}
	writeFile(t, orphanDir, "other.md", "---\nname: other\ndescription: Other\n---\n\nBody.\n")

	valid, invalid, err := skill.ReadAll(dir)
	if err != nil {
		t.Fatalf("unexpected system error: %v", err)
	}
	if len(invalid) != 0 {
		t.Errorf("expected no invalid skills, got: %v", invalid)
	}
	if len(valid) != 0 {
		t.Errorf("expected 0 valid skills (orphan folder ignored), got %d", len(valid))
	}
}

func TestReadAll_FolderSkill_RefFilesCollected(t *testing.T) {
	dir := t.TempDir()
	skillDir := writeFolderSkill(t, dir, "write-agent", validFolderSkillContent)
	writeFile(t, skillDir, "agent-patterns.md", "# Patterns\nSome patterns.")
	writeFile(t, skillDir, "delegation.md", "# Delegation\nDelegation notes.")

	valid, _, err := skill.ReadAll(dir)
	if err != nil {
		t.Fatalf("unexpected system error: %v", err)
	}
	if len(valid) != 1 {
		t.Fatalf("expected 1 valid skill, got %d", len(valid))
	}
	s := valid[0]
	if len(s.RefFiles) != 2 {
		t.Errorf("len(RefFiles) = %d, want 2; RefFiles = %v", len(s.RefFiles), s.RefFiles)
	}
}

func TestReadAll_FolderSkill_SourceDirIsAbsolute(t *testing.T) {
	dir := t.TempDir()
	writeFolderSkill(t, dir, "write-agent", validFolderSkillContent)

	valid, _, err := skill.ReadAll(dir)
	if err != nil {
		t.Fatalf("unexpected system error: %v", err)
	}
	if len(valid) != 1 {
		t.Fatalf("expected 1 valid skill, got %d", len(valid))
	}
	s := valid[0]
	if !filepath.IsAbs(s.SourceDir) {
		t.Errorf("SourceDir = %q is not absolute", s.SourceDir)
	}
}

func TestReadAll_FolderSkill_InvalidMissingDescription(t *testing.T) {
	dir := t.TempDir()
	// SKILL.md with name but no description
	writeFolderSkill(t, dir, "write-agent", "---\nname: write-agent\n---\n\n# Role\nBody.\n")

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
	if invalid[0].Field != "description" {
		t.Errorf("Field = %q, want %q", invalid[0].Field, "description")
	}
}

func TestReadFolderSkill_NameFromParentDir(t *testing.T) {
	dir := t.TempDir()
	skillDir := writeFolderSkill(t, dir, "write-agent", validFolderSkillContent)

	s, ve, err := skill.ReadFolderSkill(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		t.Fatalf("unexpected system error: %v", err)
	}
	if ve != nil {
		t.Fatalf("unexpected validation error: %v", ve)
	}
	if s.Name != "write-agent" {
		t.Errorf("Name = %q, want %q", s.Name, "write-agent")
	}
	if s.SourceDir != skillDir {
		t.Errorf("SourceDir = %q, want %q", s.SourceDir, skillDir)
	}
}

func TestReadFolderSkill_RefFilesCollected(t *testing.T) {
	dir := t.TempDir()
	skillDir := writeFolderSkill(t, dir, "write-agent", validFolderSkillContent)
	writeFile(t, skillDir, "patterns.md", "# Patterns")

	s, ve, err := skill.ReadFolderSkill(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		t.Fatalf("unexpected system error: %v", err)
	}
	if ve != nil {
		t.Fatalf("unexpected validation error: %v", ve)
	}
	if len(s.RefFiles) != 1 || s.RefFiles[0] != "patterns.md" {
		t.Errorf("RefFiles = %v, want [patterns.md]", s.RefFiles)
	}
}

func TestReadFolderSkill_MissingDescription_ValidationError(t *testing.T) {
	dir := t.TempDir()
	skillDir := writeFolderSkill(t, dir, "write-agent", "---\nname: write-agent\n---\n\n# Role\nBody.\n")

	_, ve, err := skill.ReadFolderSkill(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		t.Fatalf("unexpected system error: %v", err)
	}
	if ve == nil || ve.Field != "description" {
		t.Fatalf("expected ValidationError on field 'description', got: %v", ve)
	}
}

func TestWriteTo_FlatSkill_WritesSkillMDOnly(t *testing.T) {
	s := skill.Skill{Name: "my-skill", Raw: []byte("---\nname: my-skill\ndescription: D\n---\n\nBody.\n")}
	baseDir := t.TempDir()

	if err := skill.WriteTo(s, baseDir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(baseDir, "skills", "my-skill", "SKILL.md"))
	if err != nil {
		t.Fatalf("SKILL.md not written: %v", err)
	}
	if string(got) != string(s.Raw) {
		t.Error("SKILL.md content mismatch")
	}
}

func TestWriteTo_FolderSkill_CopiesRefFiles(t *testing.T) {
	dir := t.TempDir()
	skillDir := writeFolderSkill(t, dir, "write-agent", validFolderSkillContent)
	writeFile(t, skillDir, "patterns.md", "# Patterns\nContent.")

	s, ve, err := skill.ReadFolderSkill(filepath.Join(skillDir, "SKILL.md"))
	if err != nil || ve != nil {
		t.Fatalf("setup: ReadFolderSkill failed: err=%v ve=%v", err, ve)
	}

	baseDir := t.TempDir()
	if err := skill.WriteTo(s, baseDir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	destDir := filepath.Join(baseDir, "skills", "write-agent")
	if _, err := os.Stat(filepath.Join(destDir, "SKILL.md")); err != nil {
		t.Errorf("SKILL.md not written: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(destDir, "patterns.md"))
	if err != nil {
		t.Fatalf("reference file not copied: %v", err)
	}
	if string(got) != "# Patterns\nContent." {
		t.Error("reference file content mismatch")
	}
}

// writeNestedFile writes content to dir/relPath, creating parent directories.
func writeNestedFile(t *testing.T, dir, relPath, content string) {
	t.Helper()
	path := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("writeNestedFile: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writeNestedFile: %v", err)
	}
}

func TestReadFolderSkill_NestedRefFilesCollected(t *testing.T) {
	dir := t.TempDir()
	skillDir := writeFolderSkill(t, dir, "write-spec", validFolderSkillContent)
	writeNestedFile(t, skillDir, filepath.Join("references", "backend.tpl.md"), "# Backend")
	writeNestedFile(t, skillDir, filepath.Join("references", "qa.tpl.md"), "# QA")
	writeFile(t, skillDir, "patterns.md", "# Patterns")

	s, ve, err := skill.ReadFolderSkill(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		t.Fatalf("unexpected system error: %v", err)
	}
	if ve != nil {
		t.Fatalf("unexpected validation error: %v", ve)
	}
	want := []string{
		"patterns.md",
		filepath.Join("references", "backend.tpl.md"),
		filepath.Join("references", "qa.tpl.md"),
	}
	if len(s.RefFiles) != len(want) {
		t.Fatalf("RefFiles = %v, want %v", s.RefFiles, want)
	}
	got := make(map[string]bool, len(s.RefFiles))
	for _, ref := range s.RefFiles {
		got[ref] = true
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("RefFiles = %v, missing %q", s.RefFiles, w)
		}
	}
}

func TestReadAll_FolderSkill_NestedRefFilesCollected(t *testing.T) {
	dir := t.TempDir()
	skillDir := writeFolderSkill(t, dir, "write-spec", validFolderSkillContent)
	writeNestedFile(t, skillDir, filepath.Join("references", "backend.tpl.md"), "# Backend")

	valid, _, err := skill.ReadAll(dir)
	if err != nil {
		t.Fatalf("unexpected system error: %v", err)
	}
	if len(valid) != 1 {
		t.Fatalf("expected 1 valid skill, got %d", len(valid))
	}
	s := valid[0]
	if len(s.RefFiles) != 1 || s.RefFiles[0] != filepath.Join("references", "backend.tpl.md") {
		t.Errorf("RefFiles = %v, want [references/backend.tpl.md]", s.RefFiles)
	}
}

func TestWriteTo_FolderSkill_CopiesNestedRefFiles(t *testing.T) {
	dir := t.TempDir()
	skillDir := writeFolderSkill(t, dir, "write-spec", validFolderSkillContent)
	writeNestedFile(t, skillDir, filepath.Join("references", "backend.tpl.md"), "# Backend\nTemplate.")

	s, ve, err := skill.ReadFolderSkill(filepath.Join(skillDir, "SKILL.md"))
	if err != nil || ve != nil {
		t.Fatalf("setup: ReadFolderSkill failed: err=%v ve=%v", err, ve)
	}

	baseDir := t.TempDir()
	if err := skill.WriteTo(s, baseDir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	destDir := filepath.Join(baseDir, "skills", "write-spec")
	got, err := os.ReadFile(filepath.Join(destDir, "references", "backend.tpl.md"))
	if err != nil {
		t.Fatalf("nested reference file not copied: %v", err)
	}
	if string(got) != "# Backend\nTemplate." {
		t.Error("nested reference file content mismatch")
	}
}

func TestReadAll_FolderSkill_WithTargets(t *testing.T) {
	dir := t.TempDir()
	content := "---\nname: write-agent\ndescription: Write agent definitions\ntargets:\n  - claude-code\n---\n\n# Role\nHelps write agents.\n"
	writeFolderSkill(t, dir, "write-agent", content)

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
	if len(s.Targets) != 1 || s.Targets[0] != "claude-code" {
		t.Errorf("Targets = %v, want [claude-code]", s.Targets)
	}
}

func TestReadFolderSkill_WithTargets(t *testing.T) {
	dir := t.TempDir()
	content := "---\nname: write-agent\ndescription: Write agent definitions\ntargets:\n  - claude-code\n  - cursor\n---\n\n# Role\nHelps write agents.\n"
	skillDir := writeFolderSkill(t, dir, "write-agent", content)

	s, ve, err := skill.ReadFolderSkill(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		t.Fatalf("unexpected system error: %v", err)
	}
	if ve != nil {
		t.Fatalf("unexpected validation error: %v", ve)
	}
	want := []string{"claude-code", "cursor"}
	if len(s.Targets) != len(want) {
		t.Fatalf("Targets = %v, want %v", s.Targets, want)
	}
	for i, w := range want {
		if s.Targets[i] != w {
			t.Errorf("Targets[%d] = %q, want %q", i, s.Targets[i], w)
		}
	}
}

func TestReadAll_FolderSkill_BothFlatAndFolder(t *testing.T) {
	dir := t.TempDir()
	// Valid flat skill "a"
	writeFile(t, dir, "a.md", "---\nname: a\ndescription: Skill A\n---\n\nBody A.\n")
	// Valid folder skill "b"
	writeFolderSkill(t, dir, "b", "---\nname: b\ndescription: Skill B\n---\n\nBody B.\n")

	valid, invalid, err := skill.ReadAll(dir)
	if err != nil {
		t.Fatalf("unexpected system error: %v", err)
	}
	if len(invalid) != 0 {
		t.Errorf("expected no invalid skills, got: %v", invalid)
	}
	if len(valid) != 2 {
		t.Errorf("expected 2 valid skills, got %d", len(valid))
	}
}

func TestReadFolderSkill_TooManyFiles_Rejected(t *testing.T) {
	dir := t.TempDir()
	skillDir := writeFolderSkill(t, dir, "big-skill", validFolderSkillContent)
	for i := 0; i < 201; i++ {
		writeFile(t, skillDir, fmt.Sprintf("file-%03d.txt", i), "x")
	}

	s, ve, err := skill.ReadFolderSkill(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		t.Fatalf("unexpected system error: %v", err)
	}
	if ve == nil {
		t.Fatal("expected ValidationError for package exceeding file count limit, got nil")
	}
	if s.RefFiles != nil {
		t.Errorf("RefFiles = %v, want nil on rejection", s.RefFiles)
	}
}

func TestReadFolderSkill_FileTooLarge_Rejected(t *testing.T) {
	dir := t.TempDir()
	skillDir := writeFolderSkill(t, dir, "big-file-skill", validFolderSkillContent)
	big := make([]byte, 1<<20+1)
	writeFile(t, skillDir, "huge.bin", string(big))

	_, ve, err := skill.ReadFolderSkill(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		t.Fatalf("unexpected system error: %v", err)
	}
	if ve == nil {
		t.Fatal("expected ValidationError for file exceeding size limit, got nil")
	}
}

func TestReadFolderSkill_TotalSizeTooLarge_Rejected(t *testing.T) {
	dir := t.TempDir()
	skillDir := writeFolderSkill(t, dir, "heavy-skill", validFolderSkillContent)
	chunk := make([]byte, 900*1024)
	for i := 0; i < 6; i++ {
		writeFile(t, skillDir, fmt.Sprintf("chunk-%d.bin", i), string(chunk))
	}

	_, ve, err := skill.ReadFolderSkill(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		t.Fatalf("unexpected system error: %v", err)
	}
	if ve == nil {
		t.Fatal("expected ValidationError for package exceeding total size limit, got nil")
	}
}

func TestReadFolderSkill_TooDeep_Rejected(t *testing.T) {
	dir := t.TempDir()
	skillDir := writeFolderSkill(t, dir, "deep-skill", validFolderSkillContent)
	deepRel := filepath.Join("a", "b", "c", "d", "e", "f", "file.txt")
	writeNestedFile(t, skillDir, deepRel, "x")

	_, ve, err := skill.ReadFolderSkill(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		t.Fatalf("unexpected system error: %v", err)
	}
	if ve == nil {
		t.Fatal("expected ValidationError for resource nested past the depth limit, got nil")
	}
}

func TestReadFolderSkill_Symlink_Rejected_NotRead(t *testing.T) {
	dir := t.TempDir()
	skillDir := writeFolderSkill(t, dir, "symlink-skill", validFolderSkillContent)
	outside := writeFile(t, dir, "secret.txt", "sensitive content outside the skill package")
	linkPath := filepath.Join(skillDir, "link.txt")
	if err := os.Symlink(outside, linkPath); err != nil {
		t.Fatalf("os.Symlink: %v", err)
	}

	s, ve, err := skill.ReadFolderSkill(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		t.Fatalf("unexpected system error: %v", err)
	}
	if ve == nil {
		t.Fatal("expected ValidationError for symlink in skill package, got nil")
	}
	if s.RefFiles != nil {
		t.Errorf("RefFiles = %v, want nil — symlink target must not be read into the package", s.RefFiles)
	}
}

func TestReadFolderSkill_SpecialFile_Rejected(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("FIFOs are not supported on windows")
	}
	dir := t.TempDir()
	skillDir := writeFolderSkill(t, dir, "fifo-skill", validFolderSkillContent)
	fifoPath := filepath.Join(skillDir, "pipe")
	if err := syscall.Mkfifo(fifoPath, 0644); err != nil {
		t.Fatalf("syscall.Mkfifo: %v", err)
	}

	_, ve, err := skill.ReadFolderSkill(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		t.Fatalf("unexpected system error: %v", err)
	}
	if ve == nil {
		t.Fatal("expected ValidationError for special file (FIFO) in skill package, got nil")
	}
}

func TestReadFolderSkill_WithinLimits_Unaffected(t *testing.T) {
	dir := t.TempDir()
	skillDir := writeFolderSkill(t, dir, "normal-skill", validFolderSkillContent)
	writeFile(t, skillDir, "notes.md", "# Notes")
	writeNestedFile(t, skillDir, filepath.Join("references", "tpl.md"), "# Template")

	s, ve, err := skill.ReadFolderSkill(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		t.Fatalf("unexpected system error: %v", err)
	}
	if ve != nil {
		t.Fatalf("unexpected validation error for package within limits: %v", ve)
	}
	if len(s.RefFiles) != 2 {
		t.Errorf("RefFiles = %v, want 2 entries", s.RefFiles)
	}
}

func TestReadAll_FolderSkill_OverLimit_DoesNotBlockOthers(t *testing.T) {
	dir := t.TempDir()
	writeFolderSkill(t, dir, "good-skill", validFolderSkillContent)
	badDir := writeFolderSkill(t, dir, "bad-skill", "---\nname: bad-skill\ndescription: Bad\n---\n\n# Role\nBody.\n")
	for i := 0; i < 201; i++ {
		writeFile(t, badDir, fmt.Sprintf("file-%03d.txt", i), "x")
	}

	valid, invalid, err := skill.ReadAll(dir)
	if err != nil {
		t.Fatalf("unexpected system error: %v", err)
	}
	if len(invalid) != 1 {
		t.Fatalf("expected exactly 1 invalid skill, got %d: %v", len(invalid), invalid)
	}
	if len(valid) != 1 || valid[0].Name != "good-skill" {
		t.Fatalf("expected good-skill to remain valid despite bad-skill exceeding limits, got %v", valid)
	}
}

package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/axsmak/aim/internal/cli"
)

const testSkillContent = "---\nname: test-skill\ndescription: A test skill for import tests\n---\n\n# Test Skill\n\nThis is a test skill body.\n"

func runImportCmd(t *testing.T, fakeHome, workDir string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	t.Setenv("HOME", fakeHome)

	oldDir, cdErr := os.Getwd()
	if cdErr != nil {
		t.Fatalf("cannot get working directory: %v", cdErr)
	}
	t.Cleanup(func() { os.Chdir(oldDir) })
	if cdErr := os.Chdir(workDir); cdErr != nil {
		t.Fatalf("cannot chdir to %s: %v", workDir, cdErr)
	}

	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	root := cli.NewRootCmd("test")
	root.SetOut(outBuf)
	root.SetErr(errBuf)
	root.SetArgs(append([]string{"import"}, args...))

	err = root.Execute()
	return outBuf.String(), errBuf.String(), err
}

func setupClaudeSkill(t *testing.T, homeDir, skillName, content string) {
	t.Helper()
	skillsDir := filepath.Join(homeDir, ".claude", "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatalf("mkdir %s: %v", skillsDir, err)
	}
	path := filepath.Join(skillsDir, skillName+".md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write skill file: %v", err)
	}
}

func TestImportSkill_FromClaudeCode(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()

	setupClaudeSkill(t, fakeHome, "test-skill", testSkillContent)

	_, _, err := runImportCmd(t, fakeHome, workDir, "skill", "test-skill", "--from", "claude-code")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	dest := filepath.Join(workDir, "skills", "test-skill.md")
	got, readErr := os.ReadFile(dest)
	if readErr != nil {
		t.Fatalf("skill not written to %s: %v", dest, readErr)
	}
	if string(got) != testSkillContent {
		t.Errorf("skill content mismatch:\ngot:  %q\nwant: %q", got, testSkillContent)
	}
}

func TestImportSkill_UnknownEnv(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()

	_, _, err := runImportCmd(t, fakeHome, workDir, "skill", "anything", "--from", "unknown-env")
	if err == nil {
		t.Fatal("expected error for unknown environment, got nil")
	}
	if !strings.Contains(err.Error(), "unknown environment") {
		t.Errorf("expected 'unknown environment' error, got: %v", err)
	}
}

func TestImportSkill_NotFound(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()

	// Ensure the .claude/skills dir exists but is empty so the scanner returns no results.
	if err := os.MkdirAll(filepath.Join(fakeHome, ".claude", "skills"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	_, _, err := runImportCmd(t, fakeHome, workDir, "skill", "nonexistent-skill", "--from", "claude-code")
	if err == nil {
		t.Fatal("expected error for missing skill, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestImportSkill_DryRun(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()

	setupClaudeSkill(t, fakeHome, "test-skill", testSkillContent)

	stdout, _, err := runImportCmd(t, fakeHome, workDir, "skill", "test-skill", "--from", "claude-code", "--print")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stdout != testSkillContent {
		t.Errorf("dry-run stdout mismatch:\ngot:  %q\nwant: %q", stdout, testSkillContent)
	}

	dest := filepath.Join(workDir, "skills", "test-skill.md")
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Error("dry-run must not write the skill to disk")
	}
}

func TestImportMCP_RequiresFromFlag(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()

	_, _, err := runImportCmd(t, fakeHome, workDir, "mcp", "jira")
	if err == nil {
		t.Fatal("expected error when --from is missing, got nil")
	}
}

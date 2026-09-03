package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/axsmak/aim/internal/cli"
	"github.com/axsmak/aim/internal/globalconfig"
)

const testSkillContent = "---\nname: test-skill\ndescription: A test skill for import tests\n---\n\n# Test Skill\n\nThis is a test skill body.\n"

// runImportCmd runs `aiman import ...` with fakeHome as HOME and workDir
// registered as the active repository in the global config, mirroring a
// workspace that has already run `aiman init`.
func runImportCmd(t *testing.T, fakeHome, workDir string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	t.Setenv("HOME", fakeHome)

	if saveErr := globalconfig.Save(fakeHome, globalconfig.Config{Repo: workDir}); saveErr != nil {
		t.Fatalf("cannot write global config: %v", saveErr)
	}

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

func TestImportSkill_SuccessOutput(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()

	setupClaudeSkill(t, fakeHome, "test-skill", testSkillContent)

	stdout, _, err := runImportCmd(t, fakeHome, workDir, "skill", "test-skill", "--from", "claude-code")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "imported: skill test-skill \xc2\xb7 from claude-code\n"
	if stdout != want {
		t.Errorf("stdout mismatch:\ngot:  %q\nwant: %q", stdout, want)
	}
}

func TestImportSkill_IdenticalNoOp_Output(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()

	setupClaudeSkill(t, fakeHome, "test-skill", testSkillContent)

	// First import — new write.
	if _, _, err := runImportCmd(t, fakeHome, workDir, "skill", "test-skill", "--from", "claude-code"); err != nil {
		t.Fatalf("first import unexpected error: %v", err)
	}

	// Second import — identical no-op.
	stdout, _, err := runImportCmd(t, fakeHome, workDir, "skill", "test-skill", "--from", "claude-code")
	if err != nil {
		t.Fatalf("second import unexpected error: %v", err)
	}

	want := "up to date: skill test-skill \xc2\xb7 already identical\n"
	if stdout != want {
		t.Errorf("stdout mismatch:\ngot:  %q\nwant: %q", stdout, want)
	}
}

func TestImportMCP_SuccessOutput_WithSecrets(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()

	writeCursorMCPConfig(t, fakeHome, map[string]interface{}{
		"jira": jiraServerEntry(),
	})

	stdout, _, err := runImportCmd(t, fakeHome, workDir, "mcp", "jira", "--from", "cursor")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "imported: mcp jira \xc2\xb7 from cursor \xc2\xb7 secrets stored in aim.local.yaml\n"
	if stdout != want {
		t.Errorf("stdout mismatch:\ngot:  %q\nwant: %q", stdout, want)
	}
}

func TestImportMCP_SuccessOutput_NoSecrets(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()

	writeCursorMCPConfig(t, fakeHome, map[string]interface{}{
		"simple-tool": map[string]interface{}{
			"command": "node",
			"args":    []interface{}{"./server.js"},
		},
	})

	stdout, _, err := runImportCmd(t, fakeHome, workDir, "mcp", "simple-tool", "--from", "cursor")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "imported: mcp simple-tool \xc2\xb7 from cursor\n"
	if stdout != want {
		t.Errorf("stdout mismatch:\ngot:  %q\nwant: %q", stdout, want)
	}
}

func TestImportMCP_IdenticalNoOp_Output(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()

	writeCursorMCPConfig(t, fakeHome, map[string]interface{}{
		"simple-tool": map[string]interface{}{
			"command": "node",
			"args":    []interface{}{"./server.js"},
		},
	})

	// First import — new write.
	if _, _, err := runImportCmd(t, fakeHome, workDir, "mcp", "simple-tool", "--from", "cursor"); err != nil {
		t.Fatalf("first import unexpected error: %v", err)
	}

	// Second import — identical no-op.
	stdout, _, err := runImportCmd(t, fakeHome, workDir, "mcp", "simple-tool", "--from", "cursor")
	if err != nil {
		t.Fatalf("second import unexpected error: %v", err)
	}

	want := "up to date: mcp simple-tool \xc2\xb7 already identical\n"
	if stdout != want {
		t.Errorf("stdout mismatch:\ngot:  %q\nwant: %q", stdout, want)
	}
}

// runImportCmdNoActiveRepo is like runImportCmd but does NOT register workDir
// as the active repository in the global config, simulating `aiman import`
// run before `aiman init`.
func runImportCmdNoActiveRepo(t *testing.T, fakeHome, workDir string, args ...string) (stdout, stderr string, err error) {
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

func TestImportSkill_NoActiveRepo_ErrorsBeforeWriting(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()

	setupClaudeSkill(t, fakeHome, "test-skill", testSkillContent)

	_, _, err := runImportCmdNoActiveRepo(t, fakeHome, workDir, "skill", "test-skill", "--from", "claude-code")
	if err == nil {
		t.Fatal("expected error when no active repository is configured, got nil")
	}
	if !strings.Contains(err.Error(), "aiman init") {
		t.Errorf("error message must point to 'aiman init', got: %v", err)
	}

	if _, statErr := os.Stat(filepath.Join(workDir, "skills")); !os.IsNotExist(statErr) {
		t.Error("skills/ must not be created when no repository is active")
	}
}

func TestImportMCP_NoActiveRepo_ErrorsBeforeWriting(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()

	writeCursorMCPConfig(t, fakeHome, map[string]interface{}{
		"simple-tool": map[string]interface{}{
			"command": "node",
			"args":    []interface{}{"./server.js"},
		},
	})

	_, _, err := runImportCmdNoActiveRepo(t, fakeHome, workDir, "mcp", "simple-tool", "--from", "cursor")
	if err == nil {
		t.Fatal("expected error when no active repository is configured, got nil")
	}
	if !strings.Contains(err.Error(), "aiman init") {
		t.Errorf("error message must point to 'aiman init', got: %v", err)
	}

	if _, statErr := os.Stat(filepath.Join(workDir, "mcp")); !os.IsNotExist(statErr) {
		t.Error("mcp/ must not be created when no repository is active")
	}
	if _, statErr := os.Stat(filepath.Join(workDir, "aim.local.yaml")); !os.IsNotExist(statErr) {
		t.Error("aim.local.yaml must not be created when no repository is active")
	}
}

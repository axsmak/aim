package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/axsmak/aim/internal/cli"
)

func runAddCmd(t *testing.T, fakeHome, workDir string, args ...string) (stdout, stderr string, err error) {
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
	root.SetArgs(append([]string{"add"}, args...))

	err = root.Execute()
	return outBuf.String(), errBuf.String(), err
}

func TestAddSkill_WritesSkillFile(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()

	skillContent := "---\nname: my-skill\ndescription: A test skill\n---\n\n# Role\nDoes something useful.\n"
	srcFile := filepath.Join(t.TempDir(), "my-skill.md")
	if err := os.WriteFile(srcFile, []byte(skillContent), 0644); err != nil {
		t.Fatalf("write source skill: %v", err)
	}

	_, _, err := runAddCmd(t, fakeHome, workDir, "skill", srcFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	dest := filepath.Join(workDir, "skills", "my-skill.md")
	if _, statErr := os.Stat(dest); statErr != nil {
		t.Errorf("skill not written to %s: %v", dest, statErr)
	}

	got, readErr := os.ReadFile(dest)
	if readErr != nil {
		t.Fatalf("read dest skill: %v", readErr)
	}
	if string(got) != skillContent {
		t.Errorf("skill content mismatch:\ngot:  %q\nwant: %q", got, skillContent)
	}
}

func TestAddSkill_OverrideName(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()

	skillContent := "---\nname: original-name\ndescription: A test skill\n---\n\n# Role\nDoes something.\n"
	srcFile := filepath.Join(t.TempDir(), "original-name.md")
	if err := os.WriteFile(srcFile, []byte(skillContent), 0644); err != nil {
		t.Fatalf("write source skill: %v", err)
	}

	_, _, err := runAddCmd(t, fakeHome, workDir, "skill", srcFile, "--name", "renamed-skill")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	dest := filepath.Join(workDir, "skills", "renamed-skill.md")
	if _, statErr := os.Stat(dest); statErr != nil {
		t.Errorf("skill not written to %s: %v", dest, statErr)
	}
}

func TestAddSkill_ConflictWithoutOverwrite(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()

	skillsDir := filepath.Join(workDir, "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}

	original := "---\nname: my-skill\ndescription: Original\n---\n\n# Role\nOriginal body.\n"
	if err := os.WriteFile(filepath.Join(skillsDir, "my-skill.md"), []byte(original), 0644); err != nil {
		t.Fatalf("write existing skill: %v", err)
	}

	different := "---\nname: my-skill\ndescription: Different\n---\n\n# Role\nDifferent body.\n"
	srcFile := filepath.Join(t.TempDir(), "my-skill.md")
	if err := os.WriteFile(srcFile, []byte(different), 0644); err != nil {
		t.Fatalf("write source skill: %v", err)
	}

	_, _, err := runAddCmd(t, fakeHome, workDir, "skill", srcFile)
	if err == nil {
		t.Fatal("expected error on conflict, got nil")
	}

	got, readErr := os.ReadFile(filepath.Join(skillsDir, "my-skill.md"))
	if readErr != nil {
		t.Fatalf("read skill: %v", readErr)
	}
	if string(got) != original {
		t.Error("skill must not be overwritten without --overwrite")
	}
}

func TestAddSkill_OverwriteFlag(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()

	skillsDir := filepath.Join(workDir, "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}

	original := "---\nname: my-skill\ndescription: Original\n---\n\n# Role\nOriginal body.\n"
	if err := os.WriteFile(filepath.Join(skillsDir, "my-skill.md"), []byte(original), 0644); err != nil {
		t.Fatalf("write existing skill: %v", err)
	}

	updated := "---\nname: my-skill\ndescription: Updated\n---\n\n# Role\nUpdated body.\n"
	srcFile := filepath.Join(t.TempDir(), "my-skill.md")
	if err := os.WriteFile(srcFile, []byte(updated), 0644); err != nil {
		t.Fatalf("write source skill: %v", err)
	}

	_, _, err := runAddCmd(t, fakeHome, workDir, "skill", srcFile, "--overwrite")
	if err != nil {
		t.Fatalf("unexpected error with --overwrite: %v", err)
	}

	got, readErr := os.ReadFile(filepath.Join(skillsDir, "my-skill.md"))
	if readErr != nil {
		t.Fatalf("read skill: %v", readErr)
	}
	if string(got) != updated {
		t.Error("skill must be overwritten when --overwrite is set")
	}
}

const validFolderSkillContent = "---\nname: write-agent\ndescription: Write agent definitions\n---\n\n# Role\nHelps write agents.\n"

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

func TestAddSkill_DirPath_CopiesFolderSkillWithRefs(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()

	srcDir := writeFolderSkillSrc(t, t.TempDir(), "write-agent", validFolderSkillContent)
	if err := os.WriteFile(filepath.Join(srcDir, "patterns.md"), []byte("# Patterns"), 0644); err != nil {
		t.Fatalf("write ref file: %v", err)
	}

	_, _, err := runAddCmd(t, fakeHome, workDir, "skill", srcDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	destDir := filepath.Join(workDir, "skills", "write-agent")
	if _, statErr := os.Stat(filepath.Join(destDir, "SKILL.md")); statErr != nil {
		t.Errorf("SKILL.md not written: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(destDir, "patterns.md")); statErr != nil {
		t.Errorf("reference file not copied: %v", statErr)
	}
}

func TestAddSkill_SkillMDPath_SameResultAsDirPath(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()

	srcDir := writeFolderSkillSrc(t, t.TempDir(), "write-agent", validFolderSkillContent)
	if err := os.WriteFile(filepath.Join(srcDir, "patterns.md"), []byte("# Patterns"), 0644); err != nil {
		t.Fatalf("write ref file: %v", err)
	}

	_, _, err := runAddCmd(t, fakeHome, workDir, "skill", filepath.Join(srcDir, "SKILL.md"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	destDir := filepath.Join(workDir, "skills", "write-agent")
	if _, statErr := os.Stat(filepath.Join(destDir, "SKILL.md")); statErr != nil {
		t.Errorf("SKILL.md not written: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(destDir, "patterns.md")); statErr != nil {
		t.Errorf("reference file not copied when passing SKILL.md path: %v", statErr)
	}
}

func TestAddSkill_DirWithoutSkillMD_ClearError(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()
	emptyDir := t.TempDir()

	_, _, err := runAddCmd(t, fakeHome, workDir, "skill", emptyDir)
	if err == nil {
		t.Fatal("expected error for directory without SKILL.md, got nil")
	}
	if strings.Contains(err.Error(), "is a directory") {
		t.Errorf("expected a clear error, got system-style message: %v", err)
	}
}

func TestAddMCP_NoArgs_Error(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()

	_, _, err := runAddCmd(t, fakeHome, workDir, "mcp")
	if err == nil {
		t.Fatal("expected error when no args given, got nil")
	}
}

func TestAddSkill_SuccessOutput_NewWrite(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()

	skillContent := "---\nname: my-skill\ndescription: A test skill\n---\n\n# Role\nDoes something useful.\n"
	srcFile := filepath.Join(t.TempDir(), "my-skill.md")
	if err := os.WriteFile(srcFile, []byte(skillContent), 0644); err != nil {
		t.Fatalf("write source skill: %v", err)
	}

	stdout, _, err := runAddCmd(t, fakeHome, workDir, "skill", srcFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "added: skill my-skill\n"
	if stdout != want {
		t.Errorf("stdout mismatch:\ngot:  %q\nwant: %q", stdout, want)
	}
}

func TestAddSkill_SuccessOutput_IdenticalNoOp(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()

	skillContent := "---\nname: my-skill\ndescription: A test skill\n---\n\n# Role\nDoes something useful.\n"
	srcFile := filepath.Join(t.TempDir(), "my-skill.md")
	if err := os.WriteFile(srcFile, []byte(skillContent), 0644); err != nil {
		t.Fatalf("write source skill: %v", err)
	}

	// First add — new write.
	if _, _, err := runAddCmd(t, fakeHome, workDir, "skill", srcFile); err != nil {
		t.Fatalf("first add unexpected error: %v", err)
	}

	// Second add — identical no-op.
	stdout, _, err := runAddCmd(t, fakeHome, workDir, "skill", srcFile)
	if err != nil {
		t.Fatalf("second add unexpected error: %v", err)
	}

	want := "up to date: skill my-skill \xc2\xb7 already identical\n"
	if stdout != want {
		t.Errorf("stdout mismatch:\ngot:  %q\nwant: %q", stdout, want)
	}
}

func TestAddMCP_SuccessOutput_NewWrite(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()

	mcpContent := "name: jira\ndescription: Jira MCP\ncommand: npx\nargs:\n    - -y\n    - mcp-jira\ntargets:\n    - claude_code\nenv: []\n"
	srcFile := filepath.Join(t.TempDir(), "jira.yaml")
	if err := os.WriteFile(srcFile, []byte(mcpContent), 0644); err != nil {
		t.Fatalf("write source mcp: %v", err)
	}

	stdout, _, err := runAddCmd(t, fakeHome, workDir, "mcp", srcFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "added: mcp jira\n"
	if stdout != want {
		t.Errorf("stdout mismatch:\ngot:  %q\nwant: %q", stdout, want)
	}
}

func TestAddMCP_SuccessOutput_IdenticalNoOp(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()

	mcpContent := "name: jira\ndescription: Jira MCP\ncommand: npx\nargs:\n    - -y\n    - mcp-jira\ntargets:\n    - claude_code\nenv: []\n"
	srcFile := filepath.Join(t.TempDir(), "jira.yaml")
	if err := os.WriteFile(srcFile, []byte(mcpContent), 0644); err != nil {
		t.Fatalf("write source mcp: %v", err)
	}

	// First add — new write.
	if _, _, err := runAddCmd(t, fakeHome, workDir, "mcp", srcFile); err != nil {
		t.Fatalf("first add unexpected error: %v", err)
	}

	// Second add — identical no-op.
	stdout, _, err := runAddCmd(t, fakeHome, workDir, "mcp", srcFile)
	if err != nil {
		t.Fatalf("second add unexpected error: %v", err)
	}

	want := "up to date: mcp jira \xc2\xb7 already identical\n"
	if stdout != want {
		t.Errorf("stdout mismatch:\ngot:  %q\nwant: %q", stdout, want)
	}
}

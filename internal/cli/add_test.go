package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
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

func TestAddMCP_NoArgs_Error(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()

	_, _, err := runAddCmd(t, fakeHome, workDir, "mcp")
	if err == nil {
		t.Fatal("expected error when no args given, got nil")
	}
}

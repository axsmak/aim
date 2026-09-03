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

func TestImportMCP_UnsupportedTransport_ErrorsWithReason(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()

	writeCursorMCPConfig(t, fakeHome, map[string]interface{}{
		"remote-tool": map[string]interface{}{
			"type": "http",
			"url":  "https://example.com/mcp",
		},
	})

	_, _, err := runImportCmd(t, fakeHome, workDir, "mcp", "remote-tool", "--from", "cursor")
	if err == nil {
		t.Fatal("expected error for unsupported transport, got nil")
	}
	if !strings.Contains(err.Error(), "remote-tool") || !strings.Contains(err.Error(), "stdio") {
		t.Errorf("error = %q, want it to name the server and mention stdio-only support", err.Error())
	}

	if _, statErr := os.Stat(filepath.Join(workDir, "mcp", "remote-tool.yaml")); !os.IsNotExist(statErr) {
		t.Errorf("expected no mcp/remote-tool.yaml to be written, stat err = %v", statErr)
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

func TestImportMCP_InvalidName_PathTraversal(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()

	// Mirrors the issue #176 reproduction: a Cursor mcp.json server named
	// "../../pwned" must not be able to escape workDir via the write path.
	writeCursorMCPConfig(t, fakeHome, map[string]interface{}{
		"../../pwned": jiraServerEntry(),
	})

	_, _, err := runImportCmd(t, fakeHome, workDir, "mcp", "../../pwned", "--from", "cursor")
	if err == nil {
		t.Fatal("expected error for path-traversal item name, got nil")
	}
	if !strings.Contains(err.Error(), "invalid item name") {
		t.Errorf("expected 'invalid item name' error, got: %v", err)
	}

	if _, statErr := os.Stat(filepath.Join(workDir, "mcp")); !os.IsNotExist(statErr) {
		t.Error("mcp/ must not be created for an invalid item name")
	}
	// The traversal target ("pwned.yaml" one level above workDir's parent)
	// must not have been created either.
	if _, statErr := os.Stat(filepath.Join(filepath.Dir(filepath.Dir(workDir)), "pwned.yaml")); !os.IsNotExist(statErr) {
		t.Error("pwned.yaml must not be written outside workDir")
	}
}

func TestImportMCP_InvalidName_Rejected(t *testing.T) {
	tests := []struct {
		name    string
		argName string
	}{
		{"slash", "foo/bar"},
		{"backslash", `foo\bar`},
		{"dotdot", ".."},
		{"dot", "."},
		{"absolute", "/etc/passwd"},
		{"controlChar", "foo\x00bar"},
		{"tooLong", strings.Repeat("a", 256)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeHome := t.TempDir()
			workDir := t.TempDir()

			_, _, err := runImportCmd(t, fakeHome, workDir, "mcp", tt.argName, "--from", "cursor")
			if err == nil {
				t.Fatalf("expected error for invalid item name %q, got nil", tt.argName)
			}
			if !strings.Contains(err.Error(), "invalid item name") {
				t.Errorf("expected 'invalid item name' error, got: %v", err)
			}

			if _, statErr := os.Stat(filepath.Join(workDir, "mcp")); !os.IsNotExist(statErr) {
				t.Errorf("mcp/ must not be created for invalid item name %q", tt.argName)
			}
		})
	}
}

func TestImportSkill_InvalidName_Rejected(t *testing.T) {
	tests := []struct {
		name    string
		argName string
	}{
		{"slash", "foo/bar"},
		{"backslash", `foo\bar`},
		{"dotdot", ".."},
		{"absolute", "/etc/passwd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeHome := t.TempDir()
			workDir := t.TempDir()

			_, _, err := runImportCmd(t, fakeHome, workDir, "skill", tt.argName, "--from", "claude-code")
			if err == nil {
				t.Fatalf("expected error for invalid item name %q, got nil", tt.argName)
			}
			if !strings.Contains(err.Error(), "invalid item name") {
				t.Errorf("expected 'invalid item name' error, got: %v", err)
			}

			if _, statErr := os.Stat(filepath.Join(workDir, "skills")); !os.IsNotExist(statErr) {
				t.Errorf("skills/ must not be created for invalid item name %q", tt.argName)
			}
		})
	}
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

// setupCodexFolderSkill writes a folder-format skill under ~/.codex/skills/<name>/,
// with SKILL.md plus resource files (references/ and a script) that the flat-file
// import path would otherwise silently drop.
func setupCodexFolderSkill(t *testing.T, homeDir, skillName, skillMD string) {
	t.Helper()
	skillDir := filepath.Join(homeDir, ".codex", "skills", skillName)
	referencesDir := filepath.Join(skillDir, "references")
	if err := os.MkdirAll(referencesDir, 0755); err != nil {
		t.Fatalf("mkdir %s: %v", referencesDir, err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(referencesDir, "tpl.md"), []byte("# template"), 0644); err != nil {
		t.Fatalf("write references/tpl.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "run.sh"), []byte("#!/bin/sh\necho hi\n"), 0755); err != nil {
		t.Fatalf("write run.sh: %v", err)
	}
}

func TestImportSkill_FolderFormat_TransfersWholePackage(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()

	setupCodexFolderSkill(t, fakeHome, "folder-skill", testSkillContent)

	stdout, _, err := runImportCmd(t, fakeHome, workDir, "skill", "folder-skill", "--from", "codex")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "imported: skill folder-skill \xc2\xb7 from codex\n"
	if stdout != want {
		t.Errorf("stdout mismatch:\ngot:  %q\nwant: %q", stdout, want)
	}

	skillDir := filepath.Join(workDir, "skills", "folder-skill")
	got, readErr := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if readErr != nil {
		t.Fatalf("SKILL.md not written: %v", readErr)
	}
	if string(got) != testSkillContent {
		t.Errorf("SKILL.md content mismatch:\ngot:  %q\nwant: %q", got, testSkillContent)
	}

	// The resources that the flat import path used to drop silently (issue #177/#180).
	for _, rel := range []string{filepath.Join("references", "tpl.md"), "run.sh"} {
		if _, statErr := os.Stat(filepath.Join(skillDir, rel)); statErr != nil {
			t.Errorf("resource %s not transferred: %v", rel, statErr)
		}
	}

	// No stray flat file from the old code path.
	if _, statErr := os.Stat(filepath.Join(workDir, "skills", "folder-skill.md")); !os.IsNotExist(statErr) {
		t.Error("flat skills/folder-skill.md must not be written for a folder-format skill")
	}
}

func TestImportSkill_FolderFormat_IdenticalNoOp(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()

	setupCodexFolderSkill(t, fakeHome, "folder-skill", testSkillContent)

	if _, _, err := runImportCmd(t, fakeHome, workDir, "skill", "folder-skill", "--from", "codex"); err != nil {
		t.Fatalf("first import unexpected error: %v", err)
	}

	stdout, _, err := runImportCmd(t, fakeHome, workDir, "skill", "folder-skill", "--from", "codex")
	if err != nil {
		t.Fatalf("second import unexpected error: %v", err)
	}

	want := "up to date: skill folder-skill \xc2\xb7 already identical\n"
	if stdout != want {
		t.Errorf("stdout mismatch:\ngot:  %q\nwant: %q", stdout, want)
	}
}

func TestImportSkill_FolderFormat_ConflictNeedsOverwrite(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()

	setupCodexFolderSkill(t, fakeHome, "folder-skill", testSkillContent)

	if _, _, err := runImportCmd(t, fakeHome, workDir, "skill", "folder-skill", "--from", "codex"); err != nil {
		t.Fatalf("first import unexpected error: %v", err)
	}

	// Source changes — the existing inventory copy now differs.
	changed := "---\nname: folder-skill\ndescription: A changed test skill\n---\n\n# Changed\n\nDifferent body.\n"
	setupCodexFolderSkill(t, fakeHome, "folder-skill", changed)

	_, _, err := runImportCmd(t, fakeHome, workDir, "skill", "folder-skill", "--from", "codex")
	if err == nil {
		t.Fatal("expected conflict error without --overwrite, got nil")
	}
	if !strings.Contains(err.Error(), "--overwrite") {
		t.Errorf("expected error to mention --overwrite, got: %v", err)
	}

	stdout, _, err := runImportCmd(t, fakeHome, workDir, "skill", "folder-skill", "--from", "codex", "--overwrite")
	if err != nil {
		t.Fatalf("unexpected error with --overwrite: %v", err)
	}
	want := "imported: skill folder-skill \xc2\xb7 from codex\n"
	if stdout != want {
		t.Errorf("stdout mismatch:\ngot:  %q\nwant: %q", stdout, want)
	}

	got, readErr := os.ReadFile(filepath.Join(workDir, "skills", "folder-skill", "SKILL.md"))
	if readErr != nil {
		t.Fatalf("SKILL.md not readable after overwrite: %v", readErr)
	}
	if string(got) != changed {
		t.Errorf("SKILL.md was not overwritten:\ngot:  %q\nwant: %q", got, changed)
	}
}

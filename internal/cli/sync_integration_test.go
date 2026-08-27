package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/axsmak/aim/internal/cli"
)

const validSkillContent = "---\nname: test-skill\ndescription: A test skill\n---\n\n# Role\nDoes something useful.\n"

const validSkillContent2 = "---\nname: another-skill\ndescription: Another test skill\n---\n\n# Role\nDoes something else.\n"

const invalidSkillContent = "---\nname: bad-skill\n---\n\n# Role\nMissing description field.\n"

func runSyncCmd(t *testing.T, fakeHome, workDir string, args ...string) (stdout, stderr string, err error) {
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
	root.SetArgs(append([]string{"sync"}, args...))

	err = root.Execute()
	return outBuf.String(), errBuf.String(), err
}

func TestSync_DryRun(t *testing.T) {
	fakeHome := t.TempDir()
	if err := os.Mkdir(filepath.Join(fakeHome, ".claude"), 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}

	workDir := t.TempDir()
	skillsDir := filepath.Join(workDir, "skills")
	if err := os.Mkdir(skillsDir, 0755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "test-skill.md"), []byte(validSkillContent), 0644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	stdout, _, err := runSyncCmd(t, fakeHome, workDir, "--dry-run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "[dry-run]") {
		t.Errorf("expected [dry-run] in output, got: %q", stdout)
	}

	installPath := filepath.Join(fakeHome, ".claude", "skills", "test-skill", "SKILL.md")
	if _, statErr := os.Stat(installPath); !os.IsNotExist(statErr) {
		t.Errorf("expected SKILL.md to not be created in dry-run mode")
	}
}

func TestSync_InstallsSkills(t *testing.T) {
	fakeHome := t.TempDir()
	if err := os.Mkdir(filepath.Join(fakeHome, ".claude"), 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}

	workDir := t.TempDir()
	skillsDir := filepath.Join(workDir, "skills")
	if err := os.Mkdir(skillsDir, 0755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "test-skill.md"), []byte(validSkillContent), 0644); err != nil {
		t.Fatalf("write skill 1: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "another-skill.md"), []byte(validSkillContent2), 0644); err != nil {
		t.Fatalf("write skill 2: %v", err)
	}

	_, _, err := runSyncCmd(t, fakeHome, workDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	skill1Path := filepath.Join(fakeHome, ".claude", "skills", "test-skill", "SKILL.md")
	skill1Content, readErr := os.ReadFile(skill1Path)
	if readErr != nil {
		t.Fatalf("expected SKILL.md for test-skill, got error: %v", readErr)
	}
	if !bytes.Equal(skill1Content, []byte(validSkillContent)) {
		t.Errorf("test-skill content mismatch:\nwant: %q\ngot:  %q", validSkillContent, string(skill1Content))
	}

	skill2Path := filepath.Join(fakeHome, ".claude", "skills", "another-skill", "SKILL.md")
	skill2Content, readErr := os.ReadFile(skill2Path)
	if readErr != nil {
		t.Fatalf("expected SKILL.md for another-skill, got error: %v", readErr)
	}
	if !bytes.Equal(skill2Content, []byte(validSkillContent2)) {
		t.Errorf("another-skill content mismatch:\nwant: %q\ngot:  %q", validSkillContent2, string(skill2Content))
	}
}

func TestSync_InvalidSkillSkipped(t *testing.T) {
	fakeHome := t.TempDir()
	if err := os.Mkdir(filepath.Join(fakeHome, ".claude"), 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}

	workDir := t.TempDir()
	skillsDir := filepath.Join(workDir, "skills")
	if err := os.Mkdir(skillsDir, 0755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "bad-skill.md"), []byte(invalidSkillContent), 0644); err != nil {
		t.Fatalf("write invalid skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "test-skill.md"), []byte(validSkillContent), 0644); err != nil {
		t.Fatalf("write valid skill: %v", err)
	}

	_, stderr, err := runSyncCmd(t, fakeHome, workDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stderr, "warning:") {
		t.Errorf("expected warning in stderr for invalid skill, got: %q", stderr)
	}

	badPath := filepath.Join(fakeHome, ".claude", "skills", "bad-skill", "SKILL.md")
	if _, statErr := os.Stat(badPath); !os.IsNotExist(statErr) {
		t.Errorf("expected bad-skill SKILL.md to not be installed")
	}

	goodPath := filepath.Join(fakeHome, ".claude", "skills", "test-skill", "SKILL.md")
	if _, statErr := os.Stat(goodPath); statErr != nil {
		t.Errorf("expected test-skill SKILL.md to be installed, got: %v", statErr)
	}
}

func TestSync_NoClaude(t *testing.T) {
	fakeHome := t.TempDir()

	workDir := t.TempDir()
	skillsDir := filepath.Join(workDir, "skills")
	if err := os.Mkdir(skillsDir, 0755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "test-skill.md"), []byte(validSkillContent), 0644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	_, stderr, err := runSyncCmd(t, fakeHome, workDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stderr, "warning:") {
		t.Errorf("expected warning about claude-code not found in stderr, got: %q", stderr)
	}
	if !strings.Contains(stderr, "claude-code") {
		t.Errorf("expected 'claude-code' mentioned in stderr warning, got: %q", stderr)
	}
}

func TestSync_NoSkillsDir(t *testing.T) {
	fakeHome := t.TempDir()

	workDir := t.TempDir()

	_, _, err := runSyncCmd(t, fakeHome, workDir)
	if err == nil {
		t.Fatal("expected error when skills/ directory is missing, got nil")
	}
	if !strings.Contains(err.Error(), "skills/") {
		t.Errorf("expected error message to mention 'skills/', got: %q", err.Error())
	}
}

func TestSyncMultipleAdapters(t *testing.T) {
	fakeHome := t.TempDir()
	for _, dir := range []string{".claude", ".cursor", ".codex"} {
		if err := os.Mkdir(filepath.Join(fakeHome, dir), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	workDir := t.TempDir()
	skillsDir := filepath.Join(workDir, "skills")
	if err := os.Mkdir(skillsDir, 0755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "test-skill.md"), []byte(validSkillContent), 0644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	_, _, err := runSyncCmd(t, fakeHome, workDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, envDir := range []string{".claude", ".cursor", ".codex"} {
		p := filepath.Join(fakeHome, envDir, "skills", "test-skill", "SKILL.md")
		if _, statErr := os.Stat(p); statErr != nil {
			t.Errorf("expected skill in %s, got: %v", envDir, statErr)
		}
	}
}

func TestSyncPartialAdapters(t *testing.T) {
	fakeHome := t.TempDir()
	for _, dir := range []string{".claude", ".codex"} {
		if err := os.Mkdir(filepath.Join(fakeHome, dir), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	workDir := t.TempDir()
	skillsDir := filepath.Join(workDir, "skills")
	if err := os.Mkdir(skillsDir, 0755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "test-skill.md"), []byte(validSkillContent), 0644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	_, stderr, err := runSyncCmd(t, fakeHome, workDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(stderr, "cursor") {
		t.Errorf("expected warning about cursor in stderr, got: %q", stderr)
	}

	for _, envDir := range []string{".claude", ".codex"} {
		p := filepath.Join(fakeHome, envDir, "skills", "test-skill", "SKILL.md")
		if _, statErr := os.Stat(p); statErr != nil {
			t.Errorf("expected skill in %s, got: %v", envDir, statErr)
		}
	}

	cursorSkill := filepath.Join(fakeHome, ".cursor", "skills", "test-skill", "SKILL.md")
	if _, statErr := os.Stat(cursorSkill); !os.IsNotExist(statErr) {
		t.Errorf("expected no skill installed in .cursor (not found)")
	}
}

func TestSyncDryRunMultiAdapter(t *testing.T) {
	fakeHome := t.TempDir()
	for _, dir := range []string{".claude", ".cursor"} {
		if err := os.Mkdir(filepath.Join(fakeHome, dir), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	workDir := t.TempDir()
	skillsDir := filepath.Join(workDir, "skills")
	if err := os.Mkdir(skillsDir, 0755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "test-skill.md"), []byte(validSkillContent), 0644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	stdout, _, err := runSyncCmd(t, fakeHome, workDir, "--dry-run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(stdout, "[dry-run]") {
		t.Errorf("expected [dry-run] in output, got: %q", stdout)
	}

	for _, envDir := range []string{".claude", ".cursor"} {
		p := filepath.Join(fakeHome, envDir, "skills", "test-skill", "SKILL.md")
		if _, statErr := os.Stat(p); !os.IsNotExist(statErr) {
			t.Errorf("expected no files created in %s during dry-run", envDir)
		}
	}
}

func TestSyncWithLocalConfig(t *testing.T) {
	fakeHome := t.TempDir()
	if err := os.Mkdir(filepath.Join(fakeHome, ".claude"), 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}

	customCursorPath := t.TempDir()

	workDir := t.TempDir()
	skillsDir := filepath.Join(workDir, "skills")
	if err := os.Mkdir(skillsDir, 0755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "test-skill.md"), []byte(validSkillContent), 0644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	localCfg := "adapters:\n  cursor:\n    base_dir: " + customCursorPath + "\n"
	if err := os.WriteFile(filepath.Join(workDir, "aim.local.yaml"), []byte(localCfg), 0644); err != nil {
		t.Fatalf("write aim.local.yaml: %v", err)
	}

	_, _, err := runSyncCmd(t, fakeHome, workDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cursorSkill := filepath.Join(customCursorPath, "skills", "test-skill", "SKILL.md")
	if _, statErr := os.Stat(cursorSkill); statErr != nil {
		t.Errorf("expected skill at custom cursor path %s, got: %v", cursorSkill, statErr)
	}
}

// --- item-level skill targets (ADR-0007), additive sync path ---

const skillWithClaudeCodeTargetContent = "---\nname: targeted-skill\ndescription: Only for claude-code\ntargets:\n  - claude-code\n---\n\n# Role\nDoes something targeted.\n"

const skillWithEmptyTargetsContent = "---\nname: everywhere-skill\ndescription: Empty targets means everywhere\ntargets: []\n---\n\n# Role\nDoes something everywhere.\n"

const skillWithUnknownTargetContent = "---\nname: typo-skill\ndescription: Targets a name that matches no adapter\ntargets:\n  - claud-code\n---\n\n# Role\nGoes nowhere.\n"

// TestSync_SkillTargets_FiltersToListedEnvironments: a skill with a non-empty
// targets list installs only into the listed environments, even though other
// environments are also detected (ADR-0007 decision 1/3).
func TestSync_SkillTargets_FiltersToListedEnvironments(t *testing.T) {
	fakeHome := t.TempDir()
	for _, dir := range []string{".claude", ".cursor"} {
		if err := os.Mkdir(filepath.Join(fakeHome, dir), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	workDir := t.TempDir()
	skillsDir := filepath.Join(workDir, "skills")
	if err := os.Mkdir(skillsDir, 0755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "targeted-skill.md"), []byte(skillWithClaudeCodeTargetContent), 0644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	_, _, err := runSyncCmd(t, fakeHome, workDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	claudePath := filepath.Join(fakeHome, ".claude", "skills", "targeted-skill", "SKILL.md")
	if _, statErr := os.Stat(claudePath); statErr != nil {
		t.Errorf("expected targeted-skill installed in claude-code, got: %v", statErr)
	}
	cursorPath := filepath.Join(fakeHome, ".cursor", "skills", "targeted-skill", "SKILL.md")
	if _, statErr := os.Stat(cursorPath); !os.IsNotExist(statErr) {
		t.Errorf("expected targeted-skill NOT installed in cursor (not in targets), stat err = %v", statErr)
	}
}

// TestSync_SkillTargets_EmptyListInstallsEverywhere is the regression guard
// for ADR-0007 decision 2: an explicit empty targets list means "all
// discovered environments", exactly like a skill with no targets field.
func TestSync_SkillTargets_EmptyListInstallsEverywhere(t *testing.T) {
	fakeHome := t.TempDir()
	for _, dir := range []string{".claude", ".cursor"} {
		if err := os.Mkdir(filepath.Join(fakeHome, dir), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	workDir := t.TempDir()
	skillsDir := filepath.Join(workDir, "skills")
	if err := os.Mkdir(skillsDir, 0755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "everywhere-skill.md"), []byte(skillWithEmptyTargetsContent), 0644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	_, _, err := runSyncCmd(t, fakeHome, workDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, envDir := range []string{".claude", ".cursor"} {
		p := filepath.Join(fakeHome, envDir, "skills", "everywhere-skill", "SKILL.md")
		if _, statErr := os.Stat(p); statErr != nil {
			t.Errorf("expected everywhere-skill in %s, got: %v", envDir, statErr)
		}
	}
}

// TestSync_SkillTargets_EnvCountOnlyCountsInstalledEnvs: with two detected
// environments and a skill targeting only one, the success line must report
// 1 environment, not 2 (ADR-0007 decision 6: envCount counts environments
// that actually received a skill).
func TestSync_SkillTargets_EnvCountOnlyCountsInstalledEnvs(t *testing.T) {
	fakeHome := t.TempDir()
	for _, dir := range []string{".claude", ".cursor"} {
		if err := os.Mkdir(filepath.Join(fakeHome, dir), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	workDir := t.TempDir()
	skillsDir := filepath.Join(workDir, "skills")
	if err := os.Mkdir(skillsDir, 0755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "targeted-skill.md"), []byte(skillWithClaudeCodeTargetContent), 0644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	stdout, _, err := runSyncCmd(t, fakeHome, workDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(stdout, "synced:") {
		t.Errorf("expected 'synced:' in output, got: %q", stdout)
	}
	if !strings.Contains(stdout, "1 environment") {
		t.Errorf("expected '1 environment' (only claude-code received the skill), got: %q", stdout)
	}
	if strings.Contains(stdout, "2 environments") {
		t.Errorf("must not report 2 environments when only 1 received the skill, got: %q", stdout)
	}
}

// TestSync_SkillTargets_UnknownEnvName covers ADR-0007 decision 7: a typo in
// targets is not a validation error — the skill simply installs nowhere, and
// sync still succeeds. The success line's skill count is unaffected, and the
// "→ N environments" segment is absent (ADR-0007 decision 6).
func TestSync_SkillTargets_UnknownEnvName(t *testing.T) {
	fakeHome := t.TempDir()
	if err := os.Mkdir(filepath.Join(fakeHome, ".claude"), 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}

	workDir := t.TempDir()
	skillsDir := filepath.Join(workDir, "skills")
	if err := os.Mkdir(skillsDir, 0755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "typo-skill.md"), []byte(skillWithUnknownTargetContent), 0644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	stdout, _, err := runSyncCmd(t, fakeHome, workDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	installedPath := filepath.Join(fakeHome, ".claude", "skills", "typo-skill", "SKILL.md")
	if _, statErr := os.Stat(installedPath); !os.IsNotExist(statErr) {
		t.Errorf("expected typo-skill NOT installed anywhere, stat err = %v", statErr)
	}
	if !strings.Contains(stdout, "synced:") {
		t.Errorf("expected 'synced:' success line even though the skill matched no environment, got: %q", stdout)
	}
	if !strings.Contains(stdout, "1 skill") {
		t.Errorf("expected skill count to still count the skill (1 skill), got: %q", stdout)
	}
	if strings.Contains(stdout, "→") {
		t.Errorf("expected no '→ N environments' segment when the skill reached no environment, got: %q", stdout)
	}
}

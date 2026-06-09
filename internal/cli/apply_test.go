package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/axsmak/aim/internal/cli"
	"github.com/axsmak/aim/internal/localconfig"
)

// runApplyCmd executes `aiman apply` via cobra, chdir to workDir, HOME=fakeHome.
func runApplyCmd(t *testing.T, fakeHome, workDir string, args ...string) (stdout, stderr string, err error) {
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
	root.SetArgs(append([]string{"apply"}, args...))

	err = root.Execute()
	return outBuf.String(), errBuf.String(), err
}

// setupApplyWorkDir creates a minimal workdir with skills/ and a fake Claude env.
func setupApplyWorkDir(t *testing.T, fakeHome string) (workDir string) {
	t.Helper()
	workDir = t.TempDir()

	if err := os.MkdirAll(filepath.Join(fakeHome, ".claude"), 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workDir, "skills"), 0755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}
	if err := localconfig.Save(workDir, localconfig.Config{}); err != nil {
		t.Fatalf("save localconfig: %v", err)
	}
	return workDir
}

func TestApply_InstallsSkill(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupApplyWorkDir(t, fakeHome)

	if err := os.WriteFile(
		filepath.Join(workDir, "skills", "test-skill.md"),
		[]byte(validSkillContent),
		0644,
	); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	stdout, _, err := runApplyCmd(t, fakeHome, workDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	installPath := filepath.Join(fakeHome, ".claude", "skills", "test-skill", "SKILL.md")
	if _, statErr := os.Stat(installPath); statErr != nil {
		t.Errorf("skill not installed at %s: %v", installPath, statErr)
	}
	if !strings.Contains(stdout, "applied:") {
		t.Errorf("expected 'applied:' in output, got: %q", stdout)
	}
}

func TestApply_DryRun_WritesNothing(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupApplyWorkDir(t, fakeHome)

	if err := os.WriteFile(
		filepath.Join(workDir, "skills", "test-skill.md"),
		[]byte(validSkillContent),
		0644,
	); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	stdout, _, err := runApplyCmd(t, fakeHome, workDir, "--dry-run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(stdout, "[dry-run]") {
		t.Errorf("expected [dry-run] in output, got: %q", stdout)
	}
	if !strings.Contains(stdout, "test-skill") {
		t.Errorf("expected skill name in dry-run output, got: %q", stdout)
	}

	installPath := filepath.Join(fakeHome, ".claude", "skills", "test-skill", "SKILL.md")
	if _, statErr := os.Stat(installPath); !os.IsNotExist(statErr) {
		t.Error("skill must not be installed during --dry-run")
	}
}

func TestApply_DoesNotUpdateHashes(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupApplyWorkDir(t, fakeHome)

	if err := os.WriteFile(
		filepath.Join(workDir, "skills", "test-skill.md"),
		[]byte(validSkillContent),
		0644,
	); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	if _, _, err := runApplyCmd(t, fakeHome, workDir); err != nil {
		t.Fatalf("aiman apply failed: %v", err)
	}

	cfg, err := localconfig.Load(workDir)
	if err != nil {
		t.Fatalf("load localconfig: %v", err)
	}
	if cfg.SyncedHash != "" {
		t.Errorf("synced_hash must not be updated by aiman apply, got %q", cfg.SyncedHash)
	}
	if cfg.PublishedHash != "" {
		t.Errorf("published_hash must not be updated by aiman apply, got %q", cfg.PublishedHash)
	}
}

func TestApply_InvalidSkillWarned_ValidInstalled(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupApplyWorkDir(t, fakeHome)

	// Write one valid and one invalid skill
	if err := os.WriteFile(
		filepath.Join(workDir, "skills", "valid-skill.md"),
		[]byte("---\nname: valid-skill\ndescription: valid\n---\n\n# Role\nDoes something.\n"),
		0644,
	); err != nil {
		t.Fatalf("write valid skill: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(workDir, "skills", "invalid-skill.md"),
		[]byte("---\nno-name: x\n---\n\n# Role\nBody.\n"),
		0644,
	); err != nil {
		t.Fatalf("write invalid skill: %v", err)
	}

	_, stderr, err := runApplyCmd(t, fakeHome, workDir)
	// apply should succeed (invalid skill is warned, not fatal)
	if err != nil {
		t.Fatalf("aiman apply failed unexpectedly: %v", err)
	}
	if !strings.Contains(stderr, "warning") {
		t.Errorf("expected warning for invalid skill in stderr, got: %q", stderr)
	}

	// Valid skill should be installed; invalid one should not
	validPath := filepath.Join(fakeHome, ".claude", "skills", "valid-skill", "SKILL.md")
	if _, statErr := os.Stat(validPath); statErr != nil {
		t.Errorf("valid skill not installed: %v", statErr)
	}
	invalidPath := filepath.Join(fakeHome, ".claude", "skills", "invalid-skill", "SKILL.md")
	if _, statErr := os.Stat(invalidPath); !os.IsNotExist(statErr) {
		t.Error("invalid skill must not be installed")
	}
}

func TestApply_EmptyInventory(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupApplyWorkDir(t, fakeHome)

	// No skills, no MCP — should succeed without error
	stdout, _, err := runApplyCmd(t, fakeHome, workDir)
	if err != nil {
		t.Fatalf("aiman apply failed on empty inventory: %v", err)
	}
	// With no detected envs or no skills, output may be empty or show "0 skills"
	_ = stdout
}

func TestApply_DryRun_DoesNotPersistEnv(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupApplyWorkDir(t, fakeHome)

	// Read initial config state
	cfgBefore, err := localconfig.Load(workDir)
	if err != nil {
		t.Fatalf("load initial config: %v", err)
	}

	if err := os.WriteFile(
		filepath.Join(workDir, "skills", "test-skill.md"),
		[]byte(validSkillContent),
		0644,
	); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	if _, _, err := runApplyCmd(t, fakeHome, workDir, "--dry-run"); err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}

	cfgAfter, err := localconfig.Load(workDir)
	if err != nil {
		t.Fatalf("load config after dry-run: %v", err)
	}

	// synced_hash and published_hash must not have changed
	if cfgAfter.SyncedHash != cfgBefore.SyncedHash {
		t.Errorf("synced_hash changed during dry-run: before=%q after=%q", cfgBefore.SyncedHash, cfgAfter.SyncedHash)
	}
	if cfgAfter.PublishedHash != cfgBefore.PublishedHash {
		t.Errorf("published_hash changed during dry-run: before=%q after=%q", cfgBefore.PublishedHash, cfgAfter.PublishedHash)
	}
}

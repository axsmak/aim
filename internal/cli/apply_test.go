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

// TestApply_MCPNoTargetMatch_StillCountedInSuccessLine guards against a regression
// where installMCPs only counted an MCP server toward mcpCount if it matched at
// least one detected adapter's target. A server whose targets don't match any
// detected environment (typo, or environment not present on this machine) would
// silently vanish from the "applied:" success line instead of being reported as
// installed in 0 environments (issue #141).
func TestApply_MCPNoTargetMatch_StillCountedInSuccessLine(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupApplyWorkDir(t, fakeHome)

	if err := os.WriteFile(
		filepath.Join(workDir, "skills", "test-skill.md"),
		[]byte(validSkillContent),
		0644,
	); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	mcpDir := filepath.Join(workDir, "mcp")
	if err := os.MkdirAll(mcpDir, 0755); err != nil {
		t.Fatalf("mkdir mcp: %v", err)
	}
	noMatchMCPContent := `name: orphan-server
description: MCP server targeting an environment never detected on this machine
command: npx
args:
  - "-y"
  - orphan-mcp-pkg
targets:
  - unknown-env
env: []
`
	if err := os.WriteFile(filepath.Join(mcpDir, "orphan-server.yaml"), []byte(noMatchMCPContent), 0644); err != nil {
		t.Fatalf("write orphan-server.yaml: %v", err)
	}

	stdout, _, err := runApplyCmd(t, fakeHome, workDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(stdout, "1 MCP server") {
		t.Errorf("expected orphan MCP server to still be counted in success line, got: %q", stdout)
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

// --- sha256 delta edge cases (ADR 4.4/A1) ---

// TestApplyDryRun_SkillNotInAnyEnv: skill in inventory, not installed in any env → "A (new in all environments)".
func TestApplyDryRun_SkillNotInAnyEnv(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupApplyWorkDir(t, fakeHome)

	if err := os.WriteFile(
		filepath.Join(workDir, "skills", "refactor-helper.md"),
		[]byte("---\nname: refactor-helper\ndescription: Refactors code\n---\n\n# Role\nHelps refactor.\n"),
		0644,
	); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	// Claude env dir exists (.claude created by setupApplyWorkDir) but no skills installed.
	stdout, _, err := runApplyCmd(t, fakeHome, workDir, "--dry-run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(stdout, "[dry-run]") {
		t.Errorf("expected [dry-run] prefix, got: %q", stdout)
	}
	if !strings.Contains(stdout, "refactor-helper") {
		t.Errorf("expected skill name in output, got: %q", stdout)
	}
	if !strings.Contains(stdout, "new in all environments") {
		t.Errorf("expected 'new in all environments', got: %q", stdout)
	}
	if strings.Contains(stdout, "nothing to apply") {
		t.Errorf("must not say 'nothing to apply' when skill is new, got: %q", stdout)
	}
}

// TestApplyDryRun_SkillMissingInSubsetOfEnvs: skill in inventory, missing in only one env (claude-code),
// but present with matching content in another env — should show "A (new in claude-code)".
// We fake two adapters by using localconfig.Adapters overrides (cursor points at a dir that has the skill).
func TestApplyDryRun_SkillMissingInSubsetOfEnvs(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()

	// Set up fake claude-code env (exists, no skills installed).
	claudeDir := filepath.Join(fakeHome, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("mkdir claude: %v", err)
	}

	// Set up fake cursor env with skill already installed (matching content).
	cursorDir := filepath.Join(fakeHome, ".cursor")
	skillContent := "---\nname: commit-message\ndescription: Writes commit messages\n---\n\n# Role\nWrites commits.\n"
	installedPath := filepath.Join(cursorDir, "skills", "commit-message", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(installedPath), 0755); err != nil {
		t.Fatalf("mkdir cursor skill: %v", err)
	}
	if err := os.WriteFile(installedPath, []byte(skillContent), 0644); err != nil {
		t.Fatalf("write cursor skill: %v", err)
	}

	// Write skill to inventory.
	if err := os.MkdirAll(filepath.Join(workDir, "skills"), 0755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(workDir, "skills", "commit-message.md"),
		[]byte(skillContent),
		0644,
	); err != nil {
		t.Fatalf("write inventory skill: %v", err)
	}

	// Configure adapters via localconfig overrides so cursor points at cursorDir.
	cfg := localconfig.Config{}
	cfg.Adapters.ClaudeCode.BaseDir = claudeDir
	cfg.Adapters.Cursor.BaseDir = cursorDir
	if err := localconfig.Save(workDir, cfg); err != nil {
		t.Fatalf("save localconfig: %v", err)
	}

	stdout, _, err := runApplyCmd(t, fakeHome, workDir, "--dry-run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// commit-message is already up to date in cursor, but missing in claude-code.
	if !strings.Contains(stdout, "commit-message") {
		t.Errorf("expected skill name in output, got: %q", stdout)
	}
	// Should show A (new in claude-code), not "all environments".
	if strings.Contains(stdout, "all environments") {
		t.Errorf("must not say 'all environments' when only one env is missing, got: %q", stdout)
	}
	if !strings.Contains(stdout, "claude-code") {
		t.Errorf("expected 'claude-code' env listed in output, got: %q", stdout)
	}
}

// TestApplyDryRun_SkillContentDiffers: skill in inventory, installed in env with different content → "M (differs in ...)".
func TestApplyDryRun_SkillContentDiffers(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupApplyWorkDir(t, fakeHome)

	inventoryContent := "---\nname: commit-message\ndescription: Writes commit messages\n---\n\n# Role\nNew version.\n"
	installedContent := "---\nname: commit-message\ndescription: Writes commit messages\n---\n\n# Role\nOld version.\n"

	// Install old version in claude env.
	installedPath := filepath.Join(fakeHome, ".claude", "skills", "commit-message", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(installedPath), 0755); err != nil {
		t.Fatalf("mkdir installed skill: %v", err)
	}
	if err := os.WriteFile(installedPath, []byte(installedContent), 0644); err != nil {
		t.Fatalf("write installed skill: %v", err)
	}

	// Write new version to inventory.
	if err := os.WriteFile(
		filepath.Join(workDir, "skills", "commit-message.md"),
		[]byte(inventoryContent),
		0644,
	); err != nil {
		t.Fatalf("write inventory skill: %v", err)
	}

	stdout, _, err := runApplyCmd(t, fakeHome, workDir, "--dry-run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(stdout, "commit-message") {
		t.Errorf("expected skill name in output, got: %q", stdout)
	}
	if !strings.Contains(stdout, "differs in") {
		t.Errorf("expected 'differs in' for modified skill, got: %q", stdout)
	}
	// Must use M (modified) category.
	if !strings.Contains(stdout, "  M ") {
		t.Errorf("expected 'M' category for modified skill, got: %q", stdout)
	}
}

// TestApplyDryRun_MCPMissingEnv: MCP with missing required env var → trailing [missing env: VAR1].
func TestApplyDryRun_MCPMissingEnv(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupApplyWorkDir(t, fakeHome)

	// Write a valid MCP config with a required env var.
	mcpContent := "name: mcp-atlassian\ndescription: Atlassian MCP\ncommand: npx\nargs:\n  - -y\n  - mcp-atlassian\ntargets:\n  - claude-code\nenv:\n  - name: ATLASSIAN_TOKEN\n    description: Atlassian API token\n    required: true\n"
	mcpDir := filepath.Join(workDir, "mcp")
	if err := os.MkdirAll(mcpDir, 0755); err != nil {
		t.Fatalf("mkdir mcp: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mcpDir, "mcp-atlassian.yaml"), []byte(mcpContent), 0644); err != nil {
		t.Fatalf("write mcp: %v", err)
	}

	stdout, _, err := runApplyCmd(t, fakeHome, workDir, "--dry-run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(stdout, "mcp-atlassian") {
		t.Errorf("expected MCP name in output, got: %q", stdout)
	}
	if !strings.Contains(stdout, "[missing env:") {
		t.Errorf("expected '[missing env:' in output, got: %q", stdout)
	}
	if !strings.Contains(stdout, "ATLASSIAN_TOKEN") {
		t.Errorf("expected env var name in output, got: %q", stdout)
	}
}

// --- real-run delta block (ADR-0003 5.1) ---

// TestApply_DeltaNewInAllEnvs: skill in inventory, not installed anywhere →
// real run prints "A ... (new in all environments)" after success line.
func TestApply_DeltaNewInAllEnvs(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupApplyWorkDir(t, fakeHome)

	if err := os.WriteFile(
		filepath.Join(workDir, "skills", "refactor-helper.md"),
		[]byte("---\nname: refactor-helper\ndescription: Refactors code\n---\n\n# Role\nHelps refactor.\n"),
		0644,
	); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	stdout, _, err := runApplyCmd(t, fakeHome, workDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(stdout, "applied:") {
		t.Errorf("expected 'applied:' in output, got: %q", stdout)
	}
	if !strings.Contains(stdout, "refactor-helper") {
		t.Errorf("expected skill name in delta block, got: %q", stdout)
	}
	if !strings.Contains(stdout, "new in all environments") {
		t.Errorf("expected 'new in all environments' in delta block, got: %q", stdout)
	}
	if !strings.Contains(stdout, "  A ") {
		t.Errorf("expected '  A ' marker in delta block, got: %q", stdout)
	}
}

// TestApply_DeltaUpdatedInSubset: skill installed with old content in claude-code →
// real run prints "M ... (updated in claude-code)" after success line.
func TestApply_DeltaUpdatedInSubset(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupApplyWorkDir(t, fakeHome)

	inventoryContent := "---\nname: commit-message\ndescription: Writes commit messages\n---\n\n# Role\nNew version.\n"
	installedContent := "---\nname: commit-message\ndescription: Writes commit messages\n---\n\n# Role\nOld version.\n"

	// Install old version in claude env.
	installedPath := filepath.Join(fakeHome, ".claude", "skills", "commit-message", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(installedPath), 0755); err != nil {
		t.Fatalf("mkdir installed skill: %v", err)
	}
	if err := os.WriteFile(installedPath, []byte(installedContent), 0644); err != nil {
		t.Fatalf("write installed skill: %v", err)
	}

	// Write new version to inventory.
	if err := os.WriteFile(
		filepath.Join(workDir, "skills", "commit-message.md"),
		[]byte(inventoryContent),
		0644,
	); err != nil {
		t.Fatalf("write inventory skill: %v", err)
	}

	stdout, _, err := runApplyCmd(t, fakeHome, workDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(stdout, "applied:") {
		t.Errorf("expected 'applied:' in output, got: %q", stdout)
	}
	if !strings.Contains(stdout, "commit-message") {
		t.Errorf("expected skill name in delta block, got: %q", stdout)
	}
	if !strings.Contains(stdout, "updated in") {
		t.Errorf("expected 'updated in' qualifier in real-run delta block, got: %q", stdout)
	}
	if strings.Contains(stdout, "differs in") {
		t.Errorf("real-run must not use 'differs in' (dry-run qualifier), got: %q", stdout)
	}
	if !strings.Contains(stdout, "  M ") {
		t.Errorf("expected '  M ' marker in delta block, got: %q", stdout)
	}
}

// TestApply_DeltaEmptyWhenAlreadyInstalled: skill installed with identical content →
// success line only, no delta block.
func TestApply_DeltaEmptyWhenAlreadyInstalled(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupApplyWorkDir(t, fakeHome)

	skillContent := "---\nname: existing-skill\ndescription: Already installed\n---\n\n# Role\nDoes something.\n"

	// Install same content in claude env.
	installedPath := filepath.Join(fakeHome, ".claude", "skills", "existing-skill", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(installedPath), 0755); err != nil {
		t.Fatalf("mkdir installed skill: %v", err)
	}
	if err := os.WriteFile(installedPath, []byte(skillContent), 0644); err != nil {
		t.Fatalf("write installed skill: %v", err)
	}

	if err := os.WriteFile(
		filepath.Join(workDir, "skills", "existing-skill.md"),
		[]byte(skillContent),
		0644,
	); err != nil {
		t.Fatalf("write inventory skill: %v", err)
	}

	stdout, _, err := runApplyCmd(t, fakeHome, workDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(stdout, "applied:") {
		t.Errorf("expected 'applied:' in output, got: %q", stdout)
	}
	// No delta block expected when nothing changed.
	if strings.Contains(stdout, "existing-skill.md") {
		t.Errorf("no delta block expected when env already matches inventory, got: %q", stdout)
	}
}

// TestApplyDryRun_NothingToApply: skill in inventory already installed with identical content → "nothing to apply".
func TestApplyDryRun_NothingToApply(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupApplyWorkDir(t, fakeHome)

	skillContent := "---\nname: existing-skill\ndescription: Already installed\n---\n\n# Role\nDoes something.\n"

	// Install same content in claude env.
	installedPath := filepath.Join(fakeHome, ".claude", "skills", "existing-skill", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(installedPath), 0755); err != nil {
		t.Fatalf("mkdir installed skill: %v", err)
	}
	if err := os.WriteFile(installedPath, []byte(skillContent), 0644); err != nil {
		t.Fatalf("write installed skill: %v", err)
	}

	// Write same content to inventory.
	if err := os.WriteFile(
		filepath.Join(workDir, "skills", "existing-skill.md"),
		[]byte(skillContent),
		0644,
	); err != nil {
		t.Fatalf("write inventory skill: %v", err)
	}

	stdout, _, err := runApplyCmd(t, fakeHome, workDir, "--dry-run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(stdout, "nothing to apply") {
		t.Errorf("expected 'nothing to apply' when env matches inventory, got: %q", stdout)
	}
}

// TestApply_MCPTargetSubset_RealRun reproduces the reported scenario for issues
// #120 and #139: three environments detected, an MCP server targeting only one
// of them (claude-code).
//
//   - #120: the success line must not claim the MCP server reached all 3
//     environments — it must name the one environment it actually reached.
//   - #139: claude-code install must land in ~/.claude.json (which Claude Code
//     reads for MCP server definitions), not ~/.claude/settings.json.
func TestApply_MCPTargetSubset_RealRun(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()

	for _, dir := range []string{".claude", ".cursor", ".codex"} {
		if err := os.MkdirAll(filepath.Join(fakeHome, dir), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	if err := os.MkdirAll(filepath.Join(workDir, "skills"), 0755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(workDir, "skills", "test-skill.md"),
		[]byte(validSkillContent),
		0644,
	); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	if err := localconfig.Save(workDir, localconfig.Config{}); err != nil {
		t.Fatalf("save localconfig: %v", err)
	}

	mcpDir := filepath.Join(workDir, "mcp")
	if err := os.MkdirAll(mcpDir, 0755); err != nil {
		t.Fatalf("mkdir mcp: %v", err)
	}
	mcpContent := "name: mcp-atlassian\ndescription: Atlassian MCP\ncommand: npx\nargs:\n  - -y\n  - mcp-atlassian\ntargets:\n  - claude-code\nenv: []\n"
	if err := os.WriteFile(filepath.Join(mcpDir, "mcp-atlassian.yaml"), []byte(mcpContent), 0644); err != nil {
		t.Fatalf("write mcp: %v", err)
	}

	stdout, _, err := runApplyCmd(t, fakeHome, workDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// #120: success line must name claude-code for the MCP segment, not claim
	// all 3 environments.
	if !strings.Contains(stdout, "1 MCP server → claude-code") {
		t.Errorf("expected MCP segment scoped to claude-code, got: %q", stdout)
	}
	if !strings.Contains(stdout, "3 environments") {
		t.Errorf("expected skill segment to report 3 environments, got: %q", stdout)
	}

	// #139: the server definition must be readable from ~/.claude.json — the
	// file Claude Code actually reads — not settings.json.
	claudeJSONPath := filepath.Join(fakeHome, ".claude.json")
	data, err := os.ReadFile(claudeJSONPath)
	if err != nil {
		t.Fatalf(".claude.json not created: %v", err)
	}
	if !strings.Contains(string(data), "mcp-atlassian") {
		t.Errorf("expected mcp-atlassian in .claude.json, got: %s", string(data))
	}

	settingsPath := filepath.Join(fakeHome, ".claude", "settings.json")
	if _, err := os.Stat(settingsPath); !os.IsNotExist(err) {
		t.Errorf("settings.json should not be written by InstallMCP, stat err = %v", err)
	}

	// Cursor and codex must not receive the MCP server (targets: claude-code only).
	if _, err := os.Stat(filepath.Join(fakeHome, ".cursor", "mcp.json")); !os.IsNotExist(err) {
		t.Errorf("mcp.json should not be written to cursor (not in targets), stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(fakeHome, ".codex", "config.toml")); !os.IsNotExist(err) {
		t.Errorf("config.toml should not be written to codex (not in targets), stat err = %v", err)
	}
}

// --- item-level skill targets (ADR-0007), apply path ---

const skillTargetsClaudeCodeContent = "---\nname: targeted-skill\ndescription: Only for claude-code\ntargets:\n  - claude-code\n---\n\n# Role\nDoes something targeted.\n"

const skillTargetsUnknownEnvContent = "---\nname: typo-skill\ndescription: Targets a name that matches no adapter\ntargets:\n  - claud-code\n---\n\n# Role\nGoes nowhere.\n"

// TestApply_SkillTargets_FiltersInstallation: real run installs a targeted
// skill only into the listed environment, not into every detected one
// (ADR-0007 decision 1/3).
func TestApply_SkillTargets_FiltersInstallation(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()
	for _, dir := range []string{".claude", ".cursor"} {
		if err := os.MkdirAll(filepath.Join(fakeHome, dir), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	if err := os.MkdirAll(filepath.Join(workDir, "skills"), 0755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "skills", "targeted-skill.md"), []byte(skillTargetsClaudeCodeContent), 0644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	if err := localconfig.Save(workDir, localconfig.Config{}); err != nil {
		t.Fatalf("save localconfig: %v", err)
	}

	stdout, _, err := runApplyCmd(t, fakeHome, workDir)
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
	if !strings.Contains(stdout, "1 environment") {
		t.Errorf("expected success line to report 1 environment, got: %q", stdout)
	}
}

// TestApply_SkillTargets_DeltaExcludesDisallowedEnv: real-run delta must not
// name an environment the skill's targets exclude, even though the skill is
// new (would otherwise be "A" in every detected env).
func TestApply_SkillTargets_DeltaExcludesDisallowedEnv(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()
	for _, dir := range []string{".claude", ".cursor"} {
		if err := os.MkdirAll(filepath.Join(fakeHome, dir), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	if err := os.MkdirAll(filepath.Join(workDir, "skills"), 0755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "skills", "targeted-skill.md"), []byte(skillTargetsClaudeCodeContent), 0644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	if err := localconfig.Save(workDir, localconfig.Config{}); err != nil {
		t.Fatalf("save localconfig: %v", err)
	}

	stdout, _, err := runApplyCmd(t, fakeHome, workDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(stdout, "new in claude-code") {
		t.Errorf("expected delta scoped to claude-code, got: %q", stdout)
	}
	if strings.Contains(stdout, "all environments") {
		t.Errorf("must not say 'all environments' when cursor is excluded by targets, got: %q", stdout)
	}
}

// TestApplyDryRun_SkillTargets_DeltaExcludesDisallowedEnv is the dry-run twin
// of the above: computeApplyDelta is shared between real and dry-run, and
// the AC requires both to filter (166.md).
func TestApplyDryRun_SkillTargets_DeltaExcludesDisallowedEnv(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()
	for _, dir := range []string{".claude", ".cursor"} {
		if err := os.MkdirAll(filepath.Join(fakeHome, dir), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	if err := os.MkdirAll(filepath.Join(workDir, "skills"), 0755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "skills", "targeted-skill.md"), []byte(skillTargetsClaudeCodeContent), 0644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	if err := localconfig.Save(workDir, localconfig.Config{}); err != nil {
		t.Fatalf("save localconfig: %v", err)
	}

	stdout, _, err := runApplyCmd(t, fakeHome, workDir, "--dry-run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(stdout, "new in claude-code") {
		t.Errorf("expected dry-run delta scoped to claude-code, got: %q", stdout)
	}
	if strings.Contains(stdout, "all environments") {
		t.Errorf("must not say 'all environments' when cursor is excluded by targets, got: %q", stdout)
	}
	// AC (166.md): the dry-run summary line still lists every detected
	// environment — only the per-skill delta lines are filtered.
	if !strings.Contains(stdout, "cursor") {
		t.Errorf("expected dry-run summary to still list cursor among detected environments, got: %q", stdout)
	}
}

// TestApply_SkillTargets_UnknownEnvName_SucceedsWithNoEnvSegment covers
// ADR-0007 decision 7 (typo in targets is not a validation error) together
// with decision 6 (no "→ N environments" segment when nothing was reached).
func TestApply_SkillTargets_UnknownEnvName_SucceedsWithNoEnvSegment(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupApplyWorkDir(t, fakeHome)

	if err := os.WriteFile(filepath.Join(workDir, "skills", "typo-skill.md"), []byte(skillTargetsUnknownEnvContent), 0644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	stdout, _, err := runApplyCmd(t, fakeHome, workDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	installedPath := filepath.Join(fakeHome, ".claude", "skills", "typo-skill", "SKILL.md")
	if _, statErr := os.Stat(installedPath); !os.IsNotExist(statErr) {
		t.Errorf("expected typo-skill NOT installed anywhere, stat err = %v", statErr)
	}
	if !strings.Contains(stdout, "applied:") {
		t.Errorf("expected 'applied:' success line, got: %q", stdout)
	}
	if !strings.Contains(stdout, "1 skill") {
		t.Errorf("expected skill count to still count the skill (1 skill), got: %q", stdout)
	}
	if strings.Contains(stdout, "→") {
		t.Errorf("expected no '→ N environments' segment when the skill reached no environment, got: %q", stdout)
	}
}

// TestApply_SkillTargets_MixedInventory_SkillCountUnaffected: a mix of a
// targeted skill and an untargeted skill must report the full valid skill
// count in the success line, regardless of how targets filtered delivery.
func TestApply_SkillTargets_MixedInventory_SkillCountUnaffected(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupApplyWorkDir(t, fakeHome)

	if err := os.WriteFile(filepath.Join(workDir, "skills", "targeted-skill.md"), []byte(skillTargetsClaudeCodeContent), 0644); err != nil {
		t.Fatalf("write targeted skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "skills", "test-skill.md"), []byte(validSkillContent), 0644); err != nil {
		t.Fatalf("write untargeted skill: %v", err)
	}

	stdout, _, err := runApplyCmd(t, fakeHome, workDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(stdout, "2 skills") {
		t.Errorf("expected success line to count both valid skills (2 skills), got: %q", stdout)
	}
}

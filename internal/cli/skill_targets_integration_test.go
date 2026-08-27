package cli_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/axsmak/aim/internal/localconfig"
)

// --- Issue #168: shared fixture-inventory integration tests across the three
// independently implemented item-level skill targets code paths (ADR-0007):
// installSkillsInto/computeApplyDelta (additive sync/apply, sync.go/apply.go)
// and BuildReconcilePlan (declarative apply --loadout / pinned sync,
// reconcile.go). The risk this file closes: a fix applied to one or two of
// the three paths silently leaves the third out of sync.
//
// The acceptance-criteria fixture (168.md):
//   - claude-only    — targets: [claude-code]
//   - no-targets     — no targets field (pre-epic behavior: everywhere)
//   - empty-targets  — targets: [] (ADR-0007 decision 2: everywhere)
//   - typo-target    — targets: [claud-code], an unknown env name (nowhere)
//   - folder-skill   — folder-format skill with targets: [claude-code]
//   - ctx-server     — MCP item with targets: [claude-code], for combined output
//
// Helpers reused across the cli_test package:
//
//	runApplyCmd, setupApplyWorkDir                     — apply_test.go
//	runSyncCmd                                         — sync_integration_test.go
//	runAimCmd, runGitHelper                            — sync_git_integration_test.go
//	writeApplyLoadout                                  — apply_loadout_test.go
//	writeInventorySkill, writeInventorySkillWithTargets,
//	installEnvSkill, reconcileSkillContent,
//	writeInventoryMCP, mcpServerKeys                   — reconcile_test.go
//	mustExist, mustNotExist                            — apply_loadout_integration_test.go
//	pinGlobalConfig                                    — sync_pinned_test.go
//	assertNoDeltaBlock                                 — sync_pinned_integration_test.go

const fixtureClaudeOnlySkill = "---\nname: claude-only\ndescription: Only for claude-code\ntargets:\n  - claude-code\n---\n\n# Role\nClaude-only skill.\n"
const fixtureNoTargetsSkill = "---\nname: no-targets\ndescription: No targets field at all\n---\n\n# Role\nEverywhere skill.\n"
const fixtureEmptyTargetsSkill = "---\nname: empty-targets\ndescription: Empty targets list\ntargets: []\n---\n\n# Role\nEverywhere via empty list.\n"
const fixtureTypoTargetSkill = "---\nname: typo-target\ndescription: Targets an unknown env name\ntargets:\n  - claud-code\n---\n\n# Role\nGoes nowhere.\n"
const fixtureFolderSkillBody = "---\nname: folder-skill\ndescription: Folder format with targets\ntargets:\n  - claude-code\n---\n\n# Role\nFolder skill body.\n"

// mkdirAllEnvs creates fake ~/.claude, ~/.cursor and ~/.codex so all three
// adapters are detected — "несколько обнаруженных сред" (168.md AC).
func mkdirAllEnvs(t *testing.T, fakeHome string) {
	t.Helper()
	for _, dir := range []string{".claude", ".cursor", ".codex"} {
		if err := os.MkdirAll(filepath.Join(fakeHome, dir), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
}

// writeSkillTargetsFixture writes the ADR-0007 acceptance-criteria fixture
// (five skills plus one MCP item) into workDir/skills and workDir/mcp, so
// every command under test reads byte-identical inventory content.
func writeSkillTargetsFixture(t *testing.T, workDir string) {
	t.Helper()
	skillsDir := filepath.Join(workDir, "skills")
	mcpDir := filepath.Join(workDir, "mcp")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}
	if err := os.MkdirAll(mcpDir, 0755); err != nil {
		t.Fatalf("mkdir mcp: %v", err)
	}
	for name, content := range map[string]string{
		"claude-only.md":   fixtureClaudeOnlySkill,
		"no-targets.md":    fixtureNoTargetsSkill,
		"empty-targets.md": fixtureEmptyTargetsSkill,
		"typo-target.md":   fixtureTypoTargetSkill,
	} {
		if err := os.WriteFile(filepath.Join(skillsDir, name), []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	folderDir := filepath.Join(skillsDir, "folder-skill")
	if err := os.MkdirAll(folderDir, 0755); err != nil {
		t.Fatalf("mkdir folder-skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(folderDir, "SKILL.md"), []byte(fixtureFolderSkillBody), 0644); err != nil {
		t.Fatalf("write folder-skill/SKILL.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(folderDir, "reference.md"), []byte("# Reference\nExtra material.\n"), 0644); err != nil {
		t.Fatalf("write folder-skill/reference.md: %v", err)
	}
	writeInventoryMCP(t, workDir, "ctx-server", "claude-code")
}

// setupSkillTargetsWorkDir builds a fresh 3-env fake HOME plus a workDir
// carrying the fixture above.
func setupSkillTargetsWorkDir(t *testing.T, fakeHome string) string {
	t.Helper()
	mkdirAllEnvs(t, fakeHome)
	workDir := t.TempDir()
	writeSkillTargetsFixture(t, workDir)
	if err := localconfig.Save(workDir, localconfig.Config{}); err != nil {
		t.Fatalf("save localconfig: %v", err)
	}
	return workDir
}

// checkSkillPresence asserts a fixture skill's installed/absent state in one
// env dir under fakeHome.
func checkSkillPresence(t *testing.T, fakeHome, envDir, skillName string, want bool) {
	t.Helper()
	p := filepath.Join(fakeHome, envDir, "skills", skillName, "SKILL.md")
	if want {
		mustExist(t, p, fmt.Sprintf("%s in %s", skillName, envDir))
	} else {
		mustNotExist(t, p, fmt.Sprintf("%s in %s", skillName, envDir))
	}
}

// assertFixtureComposition checks the fixture's installed state in fakeHome
// against the expected per-env pattern: claude-only/folder-skill/ctx-server
// land only in claude-code; no-targets/empty-targets land everywhere;
// typo-target lands nowhere. Applicable to every additive path (apply, sync).
func assertFixtureComposition(t *testing.T, fakeHome string) {
	t.Helper()
	for _, envDir := range []string{".claude", ".cursor", ".codex"} {
		wantClaudeOnly := envDir == ".claude"
		checkSkillPresence(t, fakeHome, envDir, "claude-only", wantClaudeOnly)
		checkSkillPresence(t, fakeHome, envDir, "folder-skill", wantClaudeOnly)
		checkSkillPresence(t, fakeHome, envDir, "no-targets", true)
		checkSkillPresence(t, fakeHome, envDir, "empty-targets", true)
		checkSkillPresence(t, fakeHome, envDir, "typo-target", false)
	}

	// The folder skill's reference file follows the same targets filter as
	// SKILL.md itself (ADR-0007 "Ограничения": filtering applies whole-skill).
	mustExist(t, filepath.Join(fakeHome, ".claude", "skills", "folder-skill", "reference.md"),
		"folder-skill reference file admitted in claude-code")
	mustNotExist(t, filepath.Join(fakeHome, ".cursor", "skills", "folder-skill", "reference.md"),
		"folder-skill reference file not admitted in cursor")

	// MCP co-existence: ctx-server (targets: claude-code) lands only in
	// claude-code's config — same fixture, combined output (168.md AC).
	claudeKeys := mcpServerKeys(t, filepath.Join(fakeHome, ".claude.json"))
	if !claudeKeys["ctx-server"] {
		t.Errorf("expected ctx-server in claude-code MCP config, got keys: %v", claudeKeys)
	}
	cursorKeys := mcpServerKeys(t, filepath.Join(fakeHome, ".cursor", "mcp.json"))
	if cursorKeys["ctx-server"] {
		t.Error("ctx-server must not be installed in cursor (targets: claude-code only)")
	}
	mustNotExist(t, filepath.Join(fakeHome, ".codex", "config.toml"),
		"ctx-server must not create a codex config (targets: claude-code only)")
}

// --- aiman apply: composition matches targets across several detected envs ---

func TestSkillTargets_Apply_ComposesPerEnvByTargets(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupSkillTargetsWorkDir(t, fakeHome)

	stdout, _, err := runApplyCmd(t, fakeHome, workDir)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if !strings.Contains(stdout, "applied:") {
		t.Errorf("expected 'applied:' success line, got: %q", stdout)
	}
	assertFixtureComposition(t, fakeHome)
}

// --- aiman apply --dry-run: no invalid env named; second real apply is idempotent ---

func TestSkillTargets_ApplyDryRun_NoInvalidEnvNamed_ThenIdempotent(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupSkillTargetsWorkDir(t, fakeHome)

	dryStdout, _, err := runApplyCmd(t, fakeHome, workDir, "--dry-run")
	if err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}
	// typo-target's unknown env name must never appear as a destination in
	// the plan (ADR-0007 decision 7: not a validation error, just no match).
	if strings.Contains(dryStdout, "claud-code") {
		t.Errorf("dry-run plan must not name the unknown env from typo-target's targets, got: %q", dryStdout)
	}
	// Dry-run performs no writes.
	if _, statErr := os.Stat(filepath.Join(fakeHome, ".claude", "skills", "claude-only")); !os.IsNotExist(statErr) {
		t.Error("dry-run must not install anything")
	}

	if _, _, err := runApplyCmd(t, fakeHome, workDir); err != nil {
		t.Fatalf("first real apply failed: %v", err)
	}
	assertFixtureComposition(t, fakeHome)

	stdout2, _, err := runApplyCmd(t, fakeHome, workDir)
	if err != nil {
		t.Fatalf("second apply failed: %v", err)
	}
	assertNoDeltaBlock(t, stdout2)
}

// --- aiman sync (local and git branches): same composition as apply ---

func TestSkillTargets_Sync_Local_MatchesApplyComposition(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupSkillTargetsWorkDir(t, fakeHome)

	if _, _, err := runSyncCmd(t, fakeHome, workDir); err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	assertFixtureComposition(t, fakeHome)
}

func TestSkillTargets_Sync_Git_MatchesApplyComposition(t *testing.T) {
	bareDir := t.TempDir()
	runGitHelper(t, "", "init", "--bare", bareDir)

	srcWork := t.TempDir()
	runGitHelper(t, "", "clone", bareDir, srcWork)
	runGitHelper(t, srcWork, "config", "user.email", "test@test.com")
	runGitHelper(t, srcWork, "config", "user.name", "Test")

	if err := os.WriteFile(filepath.Join(srcWork, "aim.yaml"),
		[]byte("skill_paths:\n  claude-code: ~/.claude/skills\n"), 0644); err != nil {
		t.Fatalf("write aim.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcWork, ".gitignore"), []byte("aim.local.yaml\n"), 0644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}
	writeSkillTargetsFixture(t, srcWork)
	runGitHelper(t, srcWork, "add", ".")
	runGitHelper(t, srcWork, "commit", "-m", "Fixture with item-level skill targets")
	runGitHelper(t, srcWork, "branch", "-M", "main")
	runGitHelper(t, srcWork, "push", "origin", "main")
	runGitHelper(t, bareDir, "symbolic-ref", "HEAD", "refs/heads/main")

	fakeHome := t.TempDir()
	mkdirAllEnvs(t, fakeHome)
	workDir := t.TempDir()

	if _, _, err := runAimCmd(t, fakeHome, workDir, "init", "--path", workDir, bareDir); err != nil {
		t.Fatalf("aiman init: %v", err)
	}
	if _, _, err := runSyncCmd(t, fakeHome, workDir); err != nil {
		t.Fatalf("git sync failed: %v", err)
	}
	assertFixtureComposition(t, fakeHome)
}

// --- aiman apply --loadout / pinned sync: declarative D for both not-admitted
// and outside-loadout skills, and the two paths must agree ---

// setupTargetsLoadoutScenario seeds "residue" from before item-level targets
// existed: claude-only and folder-skill (targets: claude-code) already
// materialized in cursor too, plus "extra" (outside the loadout entirely)
// materialized in both envs. The loadout wants claude-only, no-targets,
// empty-targets, folder-skill and the MCP item, with no loadout-level targets
// of its own — so the plan covers all three detected environments.
func setupTargetsLoadoutScenario(t *testing.T) (fakeHome, workDir string) {
	t.Helper()
	fakeHome = t.TempDir()
	mkdirAllEnvs(t, fakeHome)
	workDir = t.TempDir()
	writeSkillTargetsFixture(t, workDir)
	writeInventorySkill(t, workDir, "extra", "Extra body outside loadout.")

	claudeDir := filepath.Join(fakeHome, ".claude")
	cursorDir := filepath.Join(fakeHome, ".cursor")
	installEnvSkill(t, cursorDir, "claude-only", fixtureClaudeOnlySkill)
	installEnvSkill(t, cursorDir, "folder-skill", fixtureFolderSkillBody)
	installEnvSkill(t, claudeDir, "extra", reconcileSkillContent("extra", "Extra body outside loadout."))
	installEnvSkill(t, cursorDir, "extra", reconcileSkillContent("extra", "Extra body outside loadout."))

	writeApplyLoadout(t, workDir, "targets-test.yaml",
		"name: targets-test\ndescription: d\nitems:\n"+
			"  - skill:claude-only\n  - skill:no-targets\n  - skill:empty-targets\n"+
			"  - skill:folder-skill\n  - mcp:ctx-server\n")
	if err := localconfig.Save(workDir, localconfig.Config{}); err != nil {
		t.Fatalf("save localconfig: %v", err)
	}
	return fakeHome, workDir
}

func assertTargetsLoadoutOutcome(t *testing.T, fakeHome string) {
	t.Helper()
	claudeDir := filepath.Join(fakeHome, ".claude")
	cursorDir := filepath.Join(fakeHome, ".cursor")

	mustExist(t, filepath.Join(claudeDir, "skills", "claude-only", "SKILL.md"), "claude-only admitted in claude-code")
	mustNotExist(t, filepath.Join(cursorDir, "skills", "claude-only"), "claude-only removed from cursor: not admitted by item targets")
	mustExist(t, filepath.Join(claudeDir, "skills", "folder-skill", "SKILL.md"), "folder-skill admitted in claude-code")
	mustNotExist(t, filepath.Join(cursorDir, "skills", "folder-skill"), "folder-skill removed from cursor: not admitted by item targets")
	mustExist(t, filepath.Join(claudeDir, "skills", "no-targets", "SKILL.md"), "no-targets everywhere (claude-code)")
	mustExist(t, filepath.Join(cursorDir, "skills", "no-targets", "SKILL.md"), "no-targets everywhere (cursor)")
	mustExist(t, filepath.Join(claudeDir, "skills", "empty-targets", "SKILL.md"), "empty-targets everywhere (claude-code)")
	mustExist(t, filepath.Join(cursorDir, "skills", "empty-targets", "SKILL.md"), "empty-targets everywhere (cursor)")
	mustNotExist(t, filepath.Join(claudeDir, "skills", "extra"), "extra outside loadout removed from claude-code")
	mustNotExist(t, filepath.Join(cursorDir, "skills", "extra"), "extra outside loadout removed from cursor")

	claudeKeys := mcpServerKeys(t, filepath.Join(fakeHome, ".claude.json"))
	if !claudeKeys["ctx-server"] {
		t.Errorf("expected ctx-server installed in claude-code via loadout, got keys: %v", claudeKeys)
	}
}

func TestSkillTargets_ApplyLoadout_RemovesNotAdmittedAndOutsideLoadout(t *testing.T) {
	fakeHome, workDir := setupTargetsLoadoutScenario(t)

	stdout, _, err := runApplyCmd(t, fakeHome, workDir, "--loadout", "targets-test")
	if err != nil {
		t.Fatalf("apply --loadout failed: %v", err)
	}
	if !strings.Contains(stdout, `applied loadout "targets-test"`) {
		t.Errorf("expected loadout success line, got: %q", stdout)
	}
	assertTargetsLoadoutOutcome(t, fakeHome)
}

func TestSkillTargets_PinnedSync_MatchesApplyLoadout(t *testing.T) {
	fakeHome, workDir := setupTargetsLoadoutScenario(t)
	pinGlobalConfig(t, fakeHome, "targets-test")

	stdout, _, err := runSyncCmd(t, fakeHome, workDir)
	if err != nil {
		t.Fatalf("pinned sync failed: %v", err)
	}
	if !strings.Contains(stdout, `applying loadout "targets-test" (pinned)`) {
		t.Errorf("expected pinned announce line, got: %q", stdout)
	}
	// Same fixture, same loadout, executed through the pin instead of the
	// explicit --loadout flag: the outcome must be identical to
	// TestSkillTargets_ApplyLoadout_RemovesNotAdmittedAndOutsideLoadout.
	assertTargetsLoadoutOutcome(t, fakeHome)
}

// --- Regression: an inventory with no item-level targets at all behaves
// identically to the pre-ADR-0007 codebase, across every command (168.md AC).

func writeNoTargetsFixture(t *testing.T, workDir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(workDir, "skills"), 0755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workDir, "mcp"), 0755); err != nil {
		t.Fatalf("mkdir mcp: %v", err)
	}
	writeInventorySkill(t, workDir, "plain-one", "Plain body one.")
	writeInventorySkill(t, workDir, "plain-two", "Plain body two.")
}

func assertNoTargetsFixtureEverywhere(t *testing.T, fakeHome string) {
	t.Helper()
	for _, envDir := range []string{".claude", ".cursor", ".codex"} {
		checkSkillPresence(t, fakeHome, envDir, "plain-one", true)
		checkSkillPresence(t, fakeHome, envDir, "plain-two", true)
	}
}

func setupNoTargetsWorkDir(t *testing.T, fakeHome string) string {
	t.Helper()
	mkdirAllEnvs(t, fakeHome)
	workDir := t.TempDir()
	writeNoTargetsFixture(t, workDir)
	if err := localconfig.Save(workDir, localconfig.Config{}); err != nil {
		t.Fatalf("save localconfig: %v", err)
	}
	return workDir
}

func TestSkillTargets_Regression_NoTargets_Apply(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupNoTargetsWorkDir(t, fakeHome)

	if _, _, err := runApplyCmd(t, fakeHome, workDir); err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	assertNoTargetsFixtureEverywhere(t, fakeHome)
}

func TestSkillTargets_Regression_NoTargets_SyncLocal(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupNoTargetsWorkDir(t, fakeHome)

	if _, _, err := runSyncCmd(t, fakeHome, workDir); err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	assertNoTargetsFixtureEverywhere(t, fakeHome)
}

func TestSkillTargets_Regression_NoTargets_SyncGit(t *testing.T) {
	bareDir := t.TempDir()
	runGitHelper(t, "", "init", "--bare", bareDir)

	srcWork := t.TempDir()
	runGitHelper(t, "", "clone", bareDir, srcWork)
	runGitHelper(t, srcWork, "config", "user.email", "test@test.com")
	runGitHelper(t, srcWork, "config", "user.name", "Test")

	if err := os.WriteFile(filepath.Join(srcWork, "aim.yaml"),
		[]byte("skill_paths:\n  claude-code: ~/.claude/skills\n"), 0644); err != nil {
		t.Fatalf("write aim.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcWork, ".gitignore"), []byte("aim.local.yaml\n"), 0644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}
	writeNoTargetsFixture(t, srcWork)
	runGitHelper(t, srcWork, "add", ".")
	runGitHelper(t, srcWork, "commit", "-m", "Fixture without any item-level targets")
	runGitHelper(t, srcWork, "branch", "-M", "main")
	runGitHelper(t, srcWork, "push", "origin", "main")
	runGitHelper(t, bareDir, "symbolic-ref", "HEAD", "refs/heads/main")

	fakeHome := t.TempDir()
	mkdirAllEnvs(t, fakeHome)
	workDir := t.TempDir()

	if _, _, err := runAimCmd(t, fakeHome, workDir, "init", "--path", workDir, bareDir); err != nil {
		t.Fatalf("aiman init: %v", err)
	}
	if _, _, err := runSyncCmd(t, fakeHome, workDir); err != nil {
		t.Fatalf("git sync failed: %v", err)
	}
	assertNoTargetsFixtureEverywhere(t, fakeHome)
}

func TestSkillTargets_Regression_NoTargets_ApplyLoadout(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupNoTargetsWorkDir(t, fakeHome)
	writeApplyLoadout(t, workDir, "plain.yaml",
		"name: plain\ndescription: d\nitems:\n  - skill:plain-one\n  - skill:plain-two\n")

	if _, _, err := runApplyCmd(t, fakeHome, workDir, "--loadout", "plain"); err != nil {
		t.Fatalf("apply --loadout failed: %v", err)
	}
	assertNoTargetsFixtureEverywhere(t, fakeHome)
}

func TestSkillTargets_Regression_NoTargets_PinnedSync(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupNoTargetsWorkDir(t, fakeHome)
	writeApplyLoadout(t, workDir, "plain.yaml",
		"name: plain\ndescription: d\nitems:\n  - skill:plain-one\n  - skill:plain-two\n")
	pinGlobalConfig(t, fakeHome, "plain")

	if _, _, err := runSyncCmd(t, fakeHome, workDir); err != nil {
		t.Fatalf("pinned sync failed: %v", err)
	}
	assertNoTargetsFixtureEverywhere(t, fakeHome)
}

// --- Additivity: narrowing targets on an already-installed skill must not
// remove it from the additive path (ADR-0007 decision 3) ---

func TestSkillTargets_Additivity_NarrowedTargetsSurvivesSync(t *testing.T) {
	fakeHome := t.TempDir()
	for _, dir := range []string{".claude", ".cursor"} {
		if err := os.MkdirAll(filepath.Join(fakeHome, dir), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	workDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workDir, "skills"), 0755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workDir, "mcp"), 0755); err != nil {
		t.Fatalf("mkdir mcp: %v", err)
	}

	claudeDir := filepath.Join(fakeHome, ".claude")
	cursorDir := filepath.Join(fakeHome, ".cursor")

	// Pre-epic residue: "narrowing" was installed everywhere before item-level
	// targets existed.
	oldContent := reconcileSkillContent("narrowing", "Original body.")
	installEnvSkill(t, cursorDir, "narrowing", oldContent)
	installEnvSkill(t, claudeDir, "narrowing", oldContent)

	// The inventory now narrows the skill's targets to claude-code only.
	writeInventorySkillWithTargets(t, workDir, "narrowing", "Original body.", "claude-code")
	if err := localconfig.Save(workDir, localconfig.Config{}); err != nil {
		t.Fatalf("save localconfig: %v", err)
	}

	if _, _, err := runSyncCmd(t, fakeHome, workDir); err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	// The additive path never deletes: cursor's stale copy must survive
	// byte-identical.
	got, err := os.ReadFile(filepath.Join(cursorDir, "skills", "narrowing", "SKILL.md"))
	if err != nil || string(got) != oldContent {
		t.Errorf("cursor copy must survive additive sync untouched: err=%v content=%q", err, got)
	}
	mustExist(t, filepath.Join(claudeDir, "skills", "narrowing", "SKILL.md"), "claude-code still admitted by targets")
}

func TestSkillTargets_Additivity_NarrowedTargetsSurvivesApply(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupApplyWorkDir(t, fakeHome)
	if err := os.MkdirAll(filepath.Join(fakeHome, ".cursor"), 0755); err != nil {
		t.Fatalf("mkdir .cursor: %v", err)
	}

	claudeDir := filepath.Join(fakeHome, ".claude")
	cursorDir := filepath.Join(fakeHome, ".cursor")

	oldContent := reconcileSkillContent("narrowing", "Original body.")
	installEnvSkill(t, cursorDir, "narrowing", oldContent)
	installEnvSkill(t, claudeDir, "narrowing", oldContent)

	writeInventorySkillWithTargets(t, workDir, "narrowing", "Original body.", "claude-code")

	if _, _, err := runApplyCmd(t, fakeHome, workDir); err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(cursorDir, "skills", "narrowing", "SKILL.md"))
	if err != nil || string(got) != oldContent {
		t.Errorf("cursor copy must survive additive apply untouched: err=%v content=%q", err, got)
	}
	mustExist(t, filepath.Join(claudeDir, "skills", "narrowing", "SKILL.md"), "claude-code still admitted by targets")
}

package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/axsmak/aim/internal/globalconfig"
	"github.com/axsmak/aim/internal/localconfig"
)

// --- Issue #162: fake-HOME integration through the real CLI surface ---
//
// sync_pinned_test.go already covers the pinned reconciliation engine itself
// (materializeSync) via the pinGlobalConfig test helper. This file drives the
// same scenarios through the actual `aiman apply --loadout X --pin` and
// `aiman switch` commands end to end — the CLI surface a human or script
// actually calls — plus the two gaps ADR-0006 calls out explicitly: pin
// idempotency across a second sync, and switch's effect on the *next* sync's
// behavior (not just the config field).
//
// Helpers reused across the cli_test package:
//   runAimCmd, runGitHelper                           — sync_git_integration_test.go
//   runApplyCmd                                        — apply_test.go / sync_integration_test.go
//   runSyncCmd                                         — sync_integration_test.go
//   setupLoadoutWorkDir, writeApplyLoadout             — apply_loadout_test.go
//   writeInventorySkill, installEnvSkill,
//   reconcileSkillContent, writeInventoryMCP,
//   writeMCPServersJSON, mcpServerKeys                 — reconcile_test.go
//   snapshotFS, diffSnapshots, mustExist, mustNotExist — apply_loadout_integration_test.go
//   pinGlobalConfig                                    — sync_pinned_test.go

// gitNarrowFixtureFiles is the bare-repo seed shared by the git-mode pinned
// integration tests: two skills, one loadout ("narrow") referencing both.
func gitNarrowFixtureFiles() map[string]string {
	return map[string]string{
		"aim.yaml":             "skill_paths:\n  claude-code: ~/.claude/skills\n",
		".gitignore":           "aim.local.yaml\n",
		"skills/hello.md":      "---\nname: hello\ndescription: Hello skill\n---\n\n# Role\nSay hello.\n",
		"skills/keep.md":       "---\nname: keep\ndescription: Keep skill\n---\n\n# Role\nKeep.\n",
		"loadouts/narrow.yaml": "name: narrow\ndescription: d\nitems:\n  - skill:hello\n  - skill:keep\n",
	}
}

// setupGitNarrowSource creates and pushes the fixture above, returning the
// bare repo path (clone URL) and the source working tree (for further pushes).
func setupGitNarrowSource(t *testing.T) (bareDir, srcWork string) {
	t.Helper()
	bareDir = t.TempDir()
	runGitHelper(t, "", "init", "--bare", bareDir)

	srcWork = t.TempDir()
	runGitHelper(t, "", "clone", bareDir, srcWork)
	runGitHelper(t, srcWork, "config", "user.email", "test@test.com")
	runGitHelper(t, srcWork, "config", "user.name", "Test")

	for name, content := range gitNarrowFixtureFiles() {
		path := filepath.Join(srcWork, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	runGitHelper(t, srcWork, "add", ".")
	runGitHelper(t, srcWork, "commit", "-m", "Initial library with narrow loadout")
	runGitHelper(t, srcWork, "branch", "-M", "main")
	runGitHelper(t, srcWork, "push", "origin", "main")
	runGitHelper(t, bareDir, "symbolic-ref", "HEAD", "refs/heads/main")
	return bareDir, srcWork
}

// assertNoDeltaBlock fails if stdout contains an A/M/D delta-block line
// (PrintDeltaBlock's 2-space indent), the same technique
// TestApplyLoadout_EmptyPlan_SuccessLineWithoutBlock and
// TestSyncIntegration_WithoutLoadout_StaysAdditive_Regression use.
func assertNoDeltaBlock(t *testing.T, stdout string) {
	t.Helper()
	for _, marker := range []string{"  A ", "  M ", "  D "} {
		if strings.Contains(stdout, marker) {
			t.Errorf("expected no delta block, got: %q", stdout)
		}
	}
}

// --- Git branch: apply --pin (real flag) -> remote change -> sync -> idempotent re-sync ---

func TestSyncGitIntegration_Pinned_ApplyPinRemoteChangeReconcilesAndIdempotent(t *testing.T) {
	bareDir, srcWork := setupGitNarrowSource(t)

	fakeHome := t.TempDir()
	if err := os.MkdirAll(filepath.Join(fakeHome, ".claude"), 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}
	workDir := t.TempDir()
	claudeDir := filepath.Join(fakeHome, ".claude")

	if _, _, err := runAimCmd(t, fakeHome, workDir, "init", "--path", workDir, bareDir); err != nil {
		t.Fatalf("aiman init: %v", err)
	}

	// Real CLI surface: apply --loadout narrow --pin (issue #159's flag), not
	// the pinGlobalConfig test helper — this is the whole point of #162.
	if _, _, err := runApplyCmd(t, fakeHome, workDir, "--loadout", "narrow", "--pin"); err != nil {
		t.Fatalf("apply --loadout narrow --pin: %v", err)
	}
	gcfg, err := globalconfig.Load(fakeHome)
	if err != nil || gcfg.Loadout != "narrow" {
		t.Fatalf("pin not persisted: cfg=%+v err=%v", gcfg, err)
	}
	mustExist(t, filepath.Join(claudeDir, "skills", "hello", "SKILL.md"), "after apply --pin")
	mustExist(t, filepath.Join(claudeDir, "skills", "keep", "SKILL.md"), "after apply --pin")

	// Remote-side change: narrow drops keep, gains added. A PLAIN aiman sync
	// (no --loadout flag exists on sync) must pick this up via the pin.
	if err := os.WriteFile(filepath.Join(srcWork, "skills", "added.md"),
		[]byte("---\nname: added\ndescription: Added skill\n---\n\n# Role\nAdded.\n"), 0644); err != nil {
		t.Fatalf("write added.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcWork, "loadouts", "narrow.yaml"),
		[]byte("name: narrow\ndescription: d\nitems:\n  - skill:hello\n  - skill:added\n"), 0644); err != nil {
		t.Fatalf("rewrite narrow.yaml: %v", err)
	}
	runGitHelper(t, srcWork, "add", ".")
	runGitHelper(t, srcWork, "commit", "-m", "narrow: drop keep, add added")
	runGitHelper(t, srcWork, "push", "origin", "main")

	stdout, _, err := runSyncCmd(t, fakeHome, workDir)
	if err != nil {
		t.Fatalf("pinned sync after remote change failed: %v", err)
	}
	if !strings.Contains(stdout, `applying loadout "narrow" (pinned)`) {
		t.Errorf("expected pinned announce line, got: %q", stdout)
	}
	for _, want := range []string{
		"A skills/added.md",
		"D skills/keep.md",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("expected %q in D-report, got: %q", want, stdout)
		}
	}
	if !strings.Contains(stdout, "synced:") {
		t.Errorf("expected success line, got: %q", stdout)
	}

	mustExist(t, filepath.Join(claudeDir, "skills", "hello", "SKILL.md"), "after reconcile")
	mustExist(t, filepath.Join(claudeDir, "skills", "added", "SKILL.md"), "after reconcile")
	mustNotExist(t, filepath.Join(claudeDir, "skills", "keep"), "removed — outside narrow after remote change")

	// Idempotency: nothing changed since, so the plan must be empty — no A/M/D
	// delta block at all, only the (still-printed) announce and success lines.
	before := snapshotFS(t, claudeDir)
	stdout2, _, err := runSyncCmd(t, fakeHome, workDir)
	if err != nil {
		t.Fatalf("idempotent re-sync failed: %v", err)
	}
	assertNoDeltaBlock(t, stdout2)
	if !strings.Contains(stdout2, "synced:") {
		t.Errorf("expected success line on idempotent re-sync, got: %q", stdout2)
	}
	after := snapshotFS(t, claudeDir)
	if !reflect.DeepEqual(before, after) {
		t.Errorf("idempotent re-sync modified the environment:\n%s", strings.Join(diffSnapshots(before, after), "\n"))
	}
}

// --- Local branch: same scenario, no .git, driven through the real --pin flag ---

func TestSyncLocalIntegration_Pinned_ApplyPinChangeReconcilesAndIdempotent(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupLoadoutWorkDir(t, fakeHome)
	claudeDir := filepath.Join(fakeHome, ".claude")

	writeInventorySkill(t, workDir, "hello", "Hello body.")
	writeInventorySkill(t, workDir, "keep", "Keep body.")
	writeApplyLoadout(t, workDir, "narrow.yaml",
		"name: narrow\ndescription: d\nitems:\n  - skill:hello\n  - skill:keep\n")

	if _, _, err := runApplyCmd(t, fakeHome, workDir, "--loadout", "narrow", "--pin"); err != nil {
		t.Fatalf("apply --loadout narrow --pin: %v", err)
	}
	gcfg, err := globalconfig.Load(fakeHome)
	if err != nil || gcfg.Loadout != "narrow" {
		t.Fatalf("pin not persisted: cfg=%+v err=%v", gcfg, err)
	}
	mustExist(t, filepath.Join(claudeDir, "skills", "hello", "SKILL.md"), "after apply --pin")
	mustExist(t, filepath.Join(claudeDir, "skills", "keep", "SKILL.md"), "after apply --pin")

	// Inventory change with no git involved — narrow drops keep, gains added.
	writeInventorySkill(t, workDir, "added", "Added body.")
	writeApplyLoadout(t, workDir, "narrow.yaml",
		"name: narrow\ndescription: d\nitems:\n  - skill:hello\n  - skill:added\n")

	stdout, _, err := runSyncCmd(t, fakeHome, workDir)
	if err != nil {
		t.Fatalf("pinned sync after inventory change failed: %v", err)
	}
	// Output composition matches the git-branch case: same announce line,
	// same D-report shape — no transport concept (no hash, no "origin/main").
	if !strings.Contains(stdout, `applying loadout "narrow" (pinned)`) {
		t.Errorf("expected pinned announce line, got: %q", stdout)
	}
	for _, want := range []string{
		"A skills/added.md",
		"D skills/keep.md",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("expected %q in D-report, got: %q", want, stdout)
		}
	}
	if !strings.Contains(stdout, "synced:") {
		t.Errorf("expected success line, got: %q", stdout)
	}

	mustExist(t, filepath.Join(claudeDir, "skills", "hello", "SKILL.md"), "after reconcile")
	mustExist(t, filepath.Join(claudeDir, "skills", "added", "SKILL.md"), "after reconcile")
	mustNotExist(t, filepath.Join(claudeDir, "skills", "keep"), "removed — outside narrow after inventory change")

	// Idempotency, mirroring the git-branch assertion above.
	before := snapshotFS(t, claudeDir)
	stdout2, _, err := runSyncCmd(t, fakeHome, workDir)
	if err != nil {
		t.Fatalf("idempotent re-sync failed: %v", err)
	}
	assertNoDeltaBlock(t, stdout2)
	if !strings.Contains(stdout2, "synced:") {
		t.Errorf("expected success line on idempotent re-sync, got: %q", stdout2)
	}
	after := snapshotFS(t, claudeDir)
	if !reflect.DeepEqual(before, after) {
		t.Errorf("idempotent re-sync modified the environment:\n%s", strings.Join(diffSnapshots(before, after), "\n"))
	}
}

// --- Namespace preservation during a PINNED sync's D-branch (ADR-0004 boundary) ---
//
// TestSyncIntegration_WithoutLoadout_StaysAdditive_Regression already proves
// foreign-file preservation for the unpinned path. This is the pinned-sync
// equivalent: a manual skill and a foreign MCP key with no inventory backing
// at all must survive even while the pin actively removes (D) managed items
// that fall outside the loadout.
func TestSyncIntegration_Pinned_PreservesForeignFilesAcrossDBranch(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupLoadoutWorkDir(t, fakeHome)
	claudeDir := filepath.Join(fakeHome, ".claude")
	claudeJSON := filepath.Join(fakeHome, ".claude.json")

	writeInventorySkill(t, workDir, "alpha", "Alpha body.")
	writeInventorySkill(t, workDir, "beta", "Beta body.")
	writeInventoryMCP(t, workDir, "inv-server", "claude-code")
	// Loadout excludes beta and inv-server — the pinned sync must D them out.
	writeApplyLoadout(t, workDir, "narrow.yaml",
		"name: narrow\ndescription: d\nitems:\n  - skill:alpha\n")

	// Pre-state: managed items outside the pinned loadout (removal candidates)
	// plus foreign state with no inventory backing at all (must survive).
	installEnvSkill(t, claudeDir, "beta", reconcileSkillContent("beta", "Beta body."))
	installEnvSkill(t, claudeDir, "manual-note", "# handmade, not in inventory\n")
	writeMCPServersJSON(t, claudeJSON, "inv-server", "foreign-server")

	pinGlobalConfig(t, fakeHome, "narrow")

	stdout, _, err := runSyncCmd(t, fakeHome, workDir)
	if err != nil {
		t.Fatalf("pinned sync failed: %v", err)
	}
	if !strings.Contains(stdout, `applying loadout "narrow" (pinned)`) {
		t.Errorf("expected pinned announce line, got: %q", stdout)
	}
	for _, want := range []string{
		"D skills/beta.md",
		"D mcp/inv-server.yaml",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("expected %q in D-report, got: %q", want, stdout)
		}
	}

	mustExist(t, filepath.Join(claudeDir, "skills", "alpha", "SKILL.md"), "in the pinned loadout")
	mustNotExist(t, filepath.Join(claudeDir, "skills", "beta"), "managed, outside the pinned loadout")
	// No corresponding inventory file at all — untouched by the D-branch.
	mustExist(t, filepath.Join(claudeDir, "skills", "manual-note", "SKILL.md"), "foreign skill must survive pinned D-branch")
	keys := mcpServerKeys(t, claudeJSON)
	if keys["inv-server"] {
		t.Error("inv-server (managed, outside loadout) must be removed from mcpServers")
	}
	if !keys["foreign-server"] {
		t.Error("foreign-server (no inventory backing) must survive pinned D-branch")
	}
}

// --- switch clears the pin -> the *next* sync actually runs unpinned ---
//
// TestSwitchClearsPinEvenWhenNewRepoHasSameLoadoutName (switch_test.go) only
// checks the config field after switch. This is the full-stack version: pin,
// switch to a different repo (which happens to have its own unrelated
// loadouts/), then run sync there and confirm it behaves like a normal
// unpinned sync.
func TestSwitchIntegration_ClearsPin_NextSyncRunsUnpinned(t *testing.T) {
	fakeHome := t.TempDir()
	oldWorkDir := setupLoadoutWorkDir(t, fakeHome)
	writeInventorySkill(t, oldWorkDir, "solo", "Solo body.")
	writeApplyLoadout(t, oldWorkDir, "dev.yaml", "name: dev\ndescription: d\nitems:\n  - skill:solo\n")

	if _, _, err := runApplyCmd(t, fakeHome, oldWorkDir, "--loadout", "dev", "--pin"); err != nil {
		t.Fatalf("apply --loadout dev --pin: %v", err)
	}
	gcfg, err := globalconfig.Load(fakeHome)
	if err != nil || gcfg.Loadout != "dev" {
		t.Fatalf("pin not persisted: cfg=%+v err=%v", gcfg, err)
	}

	// New repository: valid AIM repo via aim.local.yaml (switch_test.go's
	// TestSwitchValidRepoWithAimLocalYaml fixture), no .git — local-mode sync
	// target — with an unrelated loadouts/ directory of its own.
	newWorkDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(newWorkDir, "aim.local.yaml"), []byte("repo: .\n"), 0644); err != nil {
		t.Fatalf("write aim.local.yaml: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(newWorkDir, "skills"), 0755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(newWorkDir, "mcp"), 0755); err != nil {
		t.Fatalf("mkdir mcp: %v", err)
	}
	writeInventorySkill(t, newWorkDir, "widget", "Widget body.")
	writeApplyLoadout(t, newWorkDir, "unrelated.yaml",
		"name: unrelated\ndescription: d\nitems:\n  - skill:widget\n")

	if _, _, err := runAimCmd(t, fakeHome, oldWorkDir, "switch", newWorkDir); err != nil {
		t.Fatalf("aiman switch: %v", err)
	}
	gcfg, err = globalconfig.Load(fakeHome)
	if err != nil {
		t.Fatalf("load global config after switch: %v", err)
	}
	if gcfg.Loadout != "" {
		t.Errorf("expected pin cleared after switch, got %q", gcfg.Loadout)
	}

	stdout, _, err := runSyncCmd(t, fakeHome, newWorkDir)
	if err != nil {
		t.Fatalf("sync after switch failed: %v", err)
	}
	if strings.Contains(stdout, "applying loadout") {
		t.Errorf("sync after switch must run unpinned, got: %q", stdout)
	}
	assertNoDeltaBlock(t, stdout)
	if !strings.Contains(stdout, "synced:") {
		t.Errorf("expected additive success line, got: %q", stdout)
	}
	mustExist(t, filepath.Join(fakeHome, ".claude", "skills", "widget", "SKILL.md"), "additive sync after switch")
}

// --- Git-mode dry-run with an active pin: two separate plan blocks, no writes ---
//
// TestSyncLocal_Pinned_DryRun_ShowsPlanNoChanges (sync_pinned_test.go) covers
// this for local mode. The git branch has its own dry-run block
// (runGitSync's `if dryRun` path, sync.go) that was not exercised by any
// existing test: it prints the git-transport delta AND, separately, the
// pinned application plan — computed from on-disk state, since dry-run never
// resets (materializeSync's doc comment, "never before").
func TestSyncGitIntegration_Pinned_DryRun_ShowsPlanNoChanges(t *testing.T) {
	bareDir, srcWork := setupGitNarrowSource(t)

	fakeHome := t.TempDir()
	if err := os.MkdirAll(filepath.Join(fakeHome, ".claude"), 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}
	workDir := t.TempDir()
	claudeDir := filepath.Join(fakeHome, ".claude")

	if _, _, err := runAimCmd(t, fakeHome, workDir, "init", "--path", workDir, bareDir); err != nil {
		t.Fatalf("aiman init: %v", err)
	}
	if _, _, err := runApplyCmd(t, fakeHome, workDir, "--loadout", "narrow", "--pin"); err != nil {
		t.Fatalf("apply --loadout narrow --pin: %v", err)
	}
	mustExist(t, filepath.Join(claudeDir, "skills", "keep", "SKILL.md"), "after apply --pin")

	// Drift: keep disappears from the environment with no inventory change at
	// all. The pinned dry-run plan must show it coming back (A) — entirely
	// independent of the unrelated remote change pushed below.
	if err := os.RemoveAll(filepath.Join(claudeDir, "skills", "keep")); err != nil {
		t.Fatalf("remove keep from environment: %v", err)
	}

	// Unrelated remote change: proves the git-transport dry-run block and the
	// pinned application dry-run block are separate blocks.
	if err := os.WriteFile(filepath.Join(srcWork, "skills", "other.md"),
		[]byte("---\nname: other\ndescription: Other skill\n---\n\n# Role\nOther.\n"), 0644); err != nil {
		t.Fatalf("write other.md: %v", err)
	}
	runGitHelper(t, srcWork, "add", ".")
	runGitHelper(t, srcWork, "commit", "-m", "Unrelated remote addition")
	runGitHelper(t, srcWork, "push", "origin", "main")

	before := snapshotFS(t, claudeDir)
	cfgBefore, err := localconfig.Load(workDir)
	if err != nil {
		t.Fatalf("load localconfig: %v", err)
	}
	headBefore, err := exec.Command("git", "-C", workDir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD: %v", err)
	}

	stdout, _, err := runSyncCmd(t, fakeHome, workDir, "--dry-run")
	if err != nil {
		t.Fatalf("dry-run pinned sync failed: %v", err)
	}
	if !strings.Contains(stdout, "[dry-run] would sync") || !strings.Contains(stdout, "from origin/main") {
		t.Errorf("expected git-transport dry-run block, got: %q", stdout)
	}
	if !strings.Contains(stdout, `[dry-run] would sync loadout "narrow"`) {
		t.Errorf("expected pinned dry-run plan block, got: %q", stdout)
	}
	if !strings.Contains(stdout, "A skills/keep.md") {
		t.Errorf("expected pinned plan to show keep coming back, got: %q", stdout)
	}

	after := snapshotFS(t, claudeDir)
	if !reflect.DeepEqual(before, after) {
		t.Errorf("dry-run pinned sync modified the environment:\n%s", strings.Join(diffSnapshots(before, after), "\n"))
	}
	cfgAfter, err := localconfig.Load(workDir)
	if err != nil {
		t.Fatalf("load localconfig: %v", err)
	}
	if !reflect.DeepEqual(cfgBefore, cfgAfter) {
		t.Errorf("dry-run pinned sync modified aim.local.yaml: before=%+v after=%+v", cfgBefore, cfgAfter)
	}
	headAfter, err := exec.Command("git", "-C", workDir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD: %v", err)
	}
	if string(headBefore) != string(headAfter) {
		t.Errorf("dry-run must never reset: HEAD before=%q after=%q", headBefore, headAfter)
	}
	gcfgAfter, err := globalconfig.Load(fakeHome)
	if err != nil {
		t.Fatalf("load global config: %v", err)
	}
	if gcfgAfter.Loadout != "narrow" {
		t.Errorf("dry-run must not touch the pin, got %q", gcfgAfter.Loadout)
	}
}

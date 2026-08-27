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

// --- Issue #160: sync — shared materialization layer, pinned semantics ---
//
// Helpers reused across the cli_test package:
//   runSyncCmd                                       — sync_integration_test.go
//   runAimCmd, runGitHelper, setupGitSource           — sync_git_integration_test.go
//   setupLoadoutWorkDir, setupAMDScenario             — apply_loadout_test.go
//   writeInventorySkill, installEnvSkill,
//   reconcileSkillContent, writeInventoryMCP          — reconcile_test.go
//   writeApplyLoadout                                 — apply_loadout_test.go
//   snapshotFS, diffSnapshots, mustExist, mustNotExist — apply_loadout_integration_test.go

// pinGlobalConfig writes ~/.config/aim/config.yaml with a `loadout` pin, the
// same field aiman apply --pin would persist (ADR-0006 decision 1).
func pinGlobalConfig(t *testing.T, fakeHome, loadoutName string) {
	t.Helper()
	if err := globalconfig.Save(fakeHome, globalconfig.Config{Loadout: loadoutName}); err != nil {
		t.Fatalf("save global config: %v", err)
	}
}

// --- Branch selection: pinned sync reconciles (A/M/D), unpinned stays additive ---

func TestSyncLocal_Pinned_ReconcilesAMD(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupLoadoutWorkDir(t, fakeHome)
	setupAMDScenario(t, fakeHome, workDir) // loadout "Test": keep+update+add; env has keep/update/extra
	claudeDir := filepath.Join(fakeHome, ".claude")

	pinGlobalConfig(t, fakeHome, "Test")

	stdout, _, err := runSyncCmd(t, fakeHome, workDir)
	if err != nil {
		t.Fatalf("pinned sync failed: %v", err)
	}

	// ADR-0006 decision 5: pinned announce line before the success line.
	if !strings.Contains(stdout, `applying loadout "Test" (pinned)`) {
		t.Errorf("expected pinned announce line, got: %q", stdout)
	}
	if !strings.Contains(stdout, `synced: 3 skills → 1 environment`) {
		t.Errorf("expected synced success line, got: %q", stdout)
	}
	// Delta block: composition A/M/D with real-run qualifiers, from
	// plan.DeltaLines — not git.DiffSyncDelta (local mode has no git delta
	// anyway, so this also confirms the pinned block is the plan's own).
	for _, want := range []string{
		"A skills/add.md   (new in all environments)",
		"M skills/update.md   (updated in all environments)",
		"D skills/extra.md   (removed from all environments)",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("expected %q in delta block, got: %q", want, stdout)
		}
	}

	mustExist(t, filepath.Join(claudeDir, "skills", "add", "SKILL.md"), "after pinned sync")
	mustExist(t, filepath.Join(claudeDir, "skills", "keep", "SKILL.md"), "after pinned sync")
	got, readErr := os.ReadFile(filepath.Join(claudeDir, "skills", "update", "SKILL.md"))
	if readErr != nil || string(got) != reconcileSkillContent("update", "New body.") {
		t.Errorf("update not refreshed: err=%v content=%q", readErr, got)
	}
	mustNotExist(t, filepath.Join(claudeDir, "skills", "extra"), "outside the pinned loadout")
}

func TestSyncLocal_Pinned_DryRun_ShowsPlanNoChanges(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupLoadoutWorkDir(t, fakeHome)
	setupAMDScenario(t, fakeHome, workDir)
	claudeDir := filepath.Join(fakeHome, ".claude")
	pinGlobalConfig(t, fakeHome, "Test")

	before := snapshotFS(t, claudeDir)
	cfgBefore, err := localconfig.Load(workDir)
	if err != nil {
		t.Fatalf("load localconfig: %v", err)
	}

	stdout, _, err := runSyncCmd(t, fakeHome, workDir, "--dry-run")
	if err != nil {
		t.Fatalf("dry-run pinned sync failed: %v", err)
	}
	if !strings.Contains(stdout, "[dry-run]") {
		t.Errorf("expected [dry-run] marker, got: %q", stdout)
	}
	if !strings.Contains(stdout, `loadout "Test"`) {
		t.Errorf("expected loadout name in dry-run output, got: %q", stdout)
	}
	for _, want := range []string{
		"A skills/add.md",
		"M skills/update.md",
		"D skills/extra.md",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("expected %q in dry-run plan, got: %q", want, stdout)
		}
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
}

func TestSyncLocal_Pinned_InvalidLoadout_ExactError(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupLoadoutWorkDir(t, fakeHome)
	writeInventorySkill(t, workDir, "alpha", "Alpha body.")
	pinGlobalConfig(t, fakeHome, "ghost")

	_, _, err := runSyncCmd(t, fakeHome, workDir)
	if err == nil {
		t.Fatal("expected error for pin to a nonexistent loadout, got nil")
	}
	// Deliberately different wording from loadout.NotFoundError's message
	// (`loadout "X" not found in loadouts/`, the apply --loadout path) so the
	// two error sources stay distinguishable (ADR-0006, Technical Details).
	want := `pinned loadout "ghost" not found in inventory`
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestSyncLocal_Pinned_MissingRefs_WarnsNotFails(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupLoadoutWorkDir(t, fakeHome)
	writeInventorySkill(t, workDir, "alpha", "Alpha body.")
	// "ghost" has no backing inventory item — must warn, not fail the sync.
	writeApplyLoadout(t, workDir, "partial.yaml",
		"name: partial\ndescription: d\nitems:\n  - skill:alpha\n  - skill:ghost\n")
	pinGlobalConfig(t, fakeHome, "partial")

	_, stderr, err := runSyncCmd(t, fakeHome, workDir)
	if err != nil {
		t.Fatalf("sync with a missing loadout ref must not fail, got: %v", err)
	}
	if !strings.Contains(stderr, `no valid inventory item for skill:ghost (skipped)`) {
		t.Errorf("expected MissingRefs warning, got stderr: %q", stderr)
	}
	mustExist(t, filepath.Join(fakeHome, ".claude", "skills", "alpha", "SKILL.md"), "after sync with a missing ref")
}

// --- Git mode: pin validated after transport, error flows into combinedErr ---

func TestSyncGit_Pinned_ValidatedAfterTransport_SyncedHashNotSaved(t *testing.T) {
	bareDir := t.TempDir()
	runGitHelper(t, "", "init", "--bare", bareDir)

	srcWork := t.TempDir()
	runGitHelper(t, "", "clone", bareDir, srcWork)
	runGitHelper(t, srcWork, "config", "user.email", "test@test.com")
	runGitHelper(t, srcWork, "config", "user.name", "Test")

	files := map[string]string{
		"aim.yaml":             "skill_paths:\n  claude-code: ~/.claude/skills\n",
		".gitignore":           "aim.local.yaml\n",
		"skills/hello.md":      "---\nname: hello\ndescription: Hello skill\n---\n\n# Role\nSay hello.\n",
		"loadouts/narrow.yaml": "name: narrow\ndescription: d\nitems:\n  - skill:hello\n",
	}
	for name, content := range files {
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

	fakeHome := t.TempDir()
	if err := os.MkdirAll(filepath.Join(fakeHome, ".claude"), 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}
	workDir := t.TempDir()

	if _, _, err := runAimCmd(t, fakeHome, workDir, "init", "--path", workDir, bareDir); err != nil {
		t.Fatalf("aiman init: %v", err)
	}
	pinGlobalConfig(t, fakeHome, "narrow")

	// The loadout exists at this point: pinned sync must succeed.
	if _, _, err := runSyncCmd(t, fakeHome, workDir); err != nil {
		t.Fatalf("initial pinned sync failed: %v", err)
	}
	cfgBefore, err := localconfig.Load(workDir)
	if err != nil || cfgBefore.SyncedHash == "" {
		t.Fatalf("synced_hash not set after initial pinned sync: cfg=%+v err=%v", cfgBefore, err)
	}

	// Remote deletes the pinned loadout in a NEWER commit.
	if err := os.Remove(filepath.Join(srcWork, "loadouts", "narrow.yaml")); err != nil {
		t.Fatalf("remove narrow.yaml: %v", err)
	}
	runGitHelper(t, srcWork, "add", ".")
	runGitHelper(t, srcWork, "commit", "-m", "Remove narrow loadout")
	runGitHelper(t, srcWork, "push", "origin", "main")

	// Sync again: fetch/reset (transport) must still happen unconditionally;
	// only AFTER that does pin validation see the loadout is gone — never
	// resolved against the stale pre-fetch checkout (ADR-0006, ordering).
	_, _, syncErr := runSyncCmd(t, fakeHome, workDir)
	if syncErr == nil {
		t.Fatal("expected error when the pinned loadout is deleted remotely, got nil")
	}
	wantErr := `pinned loadout "narrow" not found in inventory`
	if syncErr.Error() != wantErr {
		t.Errorf("error = %q, want %q", syncErr.Error(), wantErr)
	}

	// Proof the transport DID run despite the pin failing afterward: HEAD
	// advanced and the file is gone from the working tree.
	mustNotExist(t, filepath.Join(workDir, "loadouts", "narrow.yaml"), "after transport removed it remotely")
	headOut, err := exec.Command("git", "-C", workDir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD: %v", err)
	}
	if strings.TrimSpace(string(headOut)) == cfgBefore.SyncedHash {
		t.Error("expected HEAD to advance past the deletion commit despite the pin failure")
	}

	// combinedErr chain: synced_hash must NOT be updated on a failed pin.
	cfgAfter, err := localconfig.Load(workDir)
	if err != nil {
		t.Fatalf("load localconfig: %v", err)
	}
	if cfgAfter.SyncedHash != cfgBefore.SyncedHash {
		t.Errorf("synced_hash changed despite pin failure: before=%q after=%q", cfgBefore.SyncedHash, cfgAfter.SyncedHash)
	}
}

// --- Observability: --force report and the pinned D-report are separate blocks ---

func TestSyncGit_Force_And_PinnedDelta_AreSeparateBlocks(t *testing.T) {
	bareDir := t.TempDir()
	runGitHelper(t, "", "init", "--bare", bareDir)

	srcWork := t.TempDir()
	runGitHelper(t, "", "clone", bareDir, srcWork)
	runGitHelper(t, srcWork, "config", "user.email", "test@test.com")
	runGitHelper(t, srcWork, "config", "user.name", "Test")

	files := map[string]string{
		"aim.yaml":             "skill_paths:\n  claude-code: ~/.claude/skills\n",
		".gitignore":           "aim.local.yaml\n",
		"skills/hello.md":      "---\nname: hello\ndescription: Hello skill\n---\n\n# Role\nSay hello.\n",
		"skills/extra.md":      "---\nname: extra\ndescription: Extra skill\n---\n\n# Role\nExtra.\n",
		"loadouts/narrow.yaml": "name: narrow\ndescription: d\nitems:\n  - skill:hello\n",
	}
	for name, content := range files {
		path := filepath.Join(srcWork, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	runGitHelper(t, srcWork, "add", ".")
	runGitHelper(t, srcWork, "commit", "-m", "Initial library")
	runGitHelper(t, srcWork, "branch", "-M", "main")
	runGitHelper(t, srcWork, "push", "origin", "main")
	runGitHelper(t, bareDir, "symbolic-ref", "HEAD", "refs/heads/main")

	fakeHome := t.TempDir()
	if err := os.MkdirAll(filepath.Join(fakeHome, ".claude"), 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}
	workDir := t.TempDir()

	if _, _, err := runAimCmd(t, fakeHome, workDir, "init", "--path", workDir, bareDir); err != nil {
		t.Fatalf("aiman init: %v", err)
	}
	// First sync unpinned: both hello and extra land in the environment.
	if _, _, err := runSyncCmd(t, fakeHome, workDir); err != nil {
		t.Fatalf("initial unpinned sync failed: %v", err)
	}
	mustExist(t, filepath.Join(fakeHome, ".claude", "skills", "extra", "SKILL.md"), "before pinning")

	// Now pin to "narrow" (excludes extra) and create an untracked file for
	// --force to discard.
	pinGlobalConfig(t, fakeHome, "narrow")
	untrackedFile := filepath.Join(workDir, "skills", "untracked-new.md")
	if err := os.WriteFile(untrackedFile, []byte("---\nname: new\ndescription: Untracked\n---\n\n# Role\nNew.\n"), 0644); err != nil {
		t.Fatalf("create untracked file: %v", err)
	}

	stdout, _, err := runSyncCmd(t, fakeHome, workDir, "--force")
	if err != nil {
		t.Fatalf("pinned sync --force failed: %v", err)
	}

	forceIdx := strings.Index(stdout, "discarded untracked files (--force):")
	deltaIdx := strings.Index(stdout, "D skills/extra.md")
	if forceIdx == -1 {
		t.Fatalf("expected --force report block, got: %q", stdout)
	}
	if deltaIdx == -1 {
		t.Fatalf("expected pinned D-report for extra.md, got: %q", stdout)
	}
	if strings.Contains(stdout, "discarded untracked files (--force):\n  D skills/extra.md") {
		t.Errorf("--force report and pinned D-report must not be merged into one block, got: %q", stdout)
	}
	// The --force block only ever lists the untracked file, never extra.md;
	// the pinned D-report only ever lists extra.md, never the untracked file.
	// The two blocks are not separated by a blank line (ADR-0003 5.1/5.3
	// never insert one between the --force report and the success line
	// either), so bound the --force block by the start of the success line
	// that always follows it, not by "\n\n".
	successIdx := strings.Index(stdout, "\nsynced:")
	if successIdx == -1 {
		t.Fatalf("expected a success line after the --force report, got: %q", stdout)
	}
	forceBlock := stdout[forceIdx:successIdx]
	if strings.Contains(forceBlock, "extra.md") {
		t.Errorf("--force block must not mention extra.md, got block: %q", forceBlock)
	}

	mustExist(t, filepath.Join(fakeHome, ".claude", "skills", "hello", "SKILL.md"), "still in the pinned loadout")
	mustNotExist(t, filepath.Join(fakeHome, ".claude", "skills", "extra"), "removed by the pinned loadout's D action")
	mustNotExist(t, untrackedFile, "removed by --force")
}

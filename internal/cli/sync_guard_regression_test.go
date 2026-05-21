package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/axsmak/aim/internal/localconfig"
)

// TestSyncAfterPushWithStaleSyncedHash is the CORE regression test for the
// push→sync false positive bug.
//
// Before the fix: sync guard compared skills/ against synced_hash (old hash X)
// and found a diff (because push created a new commit Y), then blocked sync with
// "uncommitted local changes" even though the worktree was perfectly clean.
//
// After the fix: sync guard checks actual git dirty state → clean → allows sync.
func TestSyncAfterPushWithStaleSyncedHash(t *testing.T) {
	bareURL := setupGitSource(t)

	fakeHome := t.TempDir()
	if err := os.MkdirAll(filepath.Join(fakeHome, ".claude"), 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}

	workDir := t.TempDir()

	// Step 1: aiman init
	if _, _, err := runAimCmd(t, fakeHome, workDir, "init", "--path", workDir, bareURL); err != nil {
		t.Fatalf("aiman init: %v", err)
	}

	// Step 2: aiman sync → synced_hash = HEAD (call it hash X)
	if _, _, err := runSyncCmd(t, fakeHome, workDir); err != nil {
		t.Fatalf("aiman sync (initial): %v", err)
	}

	cfgAfterSync, err := localconfig.Load(workDir)
	if err != nil {
		t.Fatalf("load config after sync: %v", err)
	}
	hashX := cfgAfterSync.SyncedHash
	if hashX == "" {
		t.Fatal("synced_hash is empty after initial sync")
	}

	// Step 3: Modify skills/hello.md and commit (simulate what aiman push does)
	skillFile := filepath.Join(workDir, "skills", "hello.md")
	if err := os.WriteFile(skillFile, []byte("---\nname: hello\ndescription: Updated hello\n---\n\n# Role\nSay hello loudly.\n"), 0644); err != nil {
		t.Fatalf("modify skill: %v", err)
	}
	runGitHelper(t, workDir, "config", "user.email", "test@test.com")
	runGitHelper(t, workDir, "config", "user.name", "Test")
	runGitHelper(t, workDir, "add", "skills/hello.md")
	runGitHelper(t, workDir, "commit", "-m", "Update hello skill")

	// Step 4: Push commit Y to origin
	runGitHelper(t, workDir, "push", "origin", "main")

	// Step 5: Simulate the bug condition — published_hash=Y, synced_hash=X (stale)
	hashYOut, err := exec.Command("git", "-C", workDir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD: %v", err)
	}
	hashY := strings.TrimSpace(string(hashYOut))

	cfg, err := localconfig.Load(workDir)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.PublishedHash = hashY
	cfg.SyncedHash = hashX // deliberately stale: still points to old commit X
	if err := localconfig.Save(workDir, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	// Step 6: Sanity check — worktree must be clean (no uncommitted changes)
	statusOut, err := exec.Command("git", "-C", workDir, "status", "--porcelain").Output()
	if err != nil {
		t.Fatalf("git status: %v", err)
	}
	if strings.TrimSpace(string(statusOut)) != "" {
		t.Fatalf("precondition failed: worktree is not clean: %q", string(statusOut))
	}

	// Step 7: Run aiman sync — MUST succeed despite stale synced_hash
	// Old code would block here with "uncommitted local changes"
	_, stderr, syncErr := runSyncCmd(t, fakeHome, workDir)
	if syncErr != nil {
		t.Errorf("aiman sync failed after push with stale synced_hash (regression): %v\nstderr: %s", syncErr, stderr)
	}

	// Step 8: After sync, synced_hash must be updated to Y
	cfgAfter, err := localconfig.Load(workDir)
	if err != nil {
		t.Fatalf("load config after sync: %v", err)
	}
	if cfgAfter.SyncedHash != hashY {
		t.Errorf("synced_hash after sync: want %q (Y), got %q", hashY, cfgAfter.SyncedHash)
	}
}

// TestSyncBlocksDirtyManagedPaths verifies that aiman sync without --force is blocked
// when managed files (skills/) have uncommitted local changes (dirty worktree).
func TestSyncBlocksDirtyManagedPaths(t *testing.T) {
	bareURL := setupGitSource(t)

	fakeHome := t.TempDir()
	if err := os.MkdirAll(filepath.Join(fakeHome, ".claude"), 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}

	workDir := t.TempDir()

	// Init + first sync
	if _, _, err := runAimCmd(t, fakeHome, workDir, "init", "--path", workDir, bareURL); err != nil {
		t.Fatalf("aiman init: %v", err)
	}
	if _, _, err := runSyncCmd(t, fakeHome, workDir); err != nil {
		t.Fatalf("aiman sync (initial): %v", err)
	}

	// Dirty the worktree: modify skills/hello.md but do NOT git add
	skillFile := filepath.Join(workDir, "skills", "hello.md")
	if err := os.WriteFile(skillFile, []byte("---\nname: hello\ndescription: Dirty local change\n---\n\n# Role\nDirty.\n"), 0644); err != nil {
		t.Fatalf("modify skill: %v", err)
	}

	// aiman sync without --force must fail
	_, stderr, err := runSyncCmd(t, fakeHome, workDir)
	if err == nil {
		t.Fatal("expected aiman sync to fail with dirty managed files, got success")
	}

	// Must contain "tracked or staged" (wording for dirty tracked files in fixed implementation)
	combined := err.Error() + " " + stderr
	if !strings.Contains(combined, "tracked or staged") {
		t.Errorf("expected error to mention 'tracked or staged', got: %v / stderr: %s", err, stderr)
	}

	// Must NOT contain old wording "uncommitted local changes"
	if strings.Contains(combined, "uncommitted local changes") {
		t.Errorf("error still uses old wording 'uncommitted local changes' (regression): %v / stderr: %s", err, stderr)
	}
}

// TestSyncAllowsCleanStaleSyncedHash verifies that a stale synced_hash alone (without
// any actual file modifications) does NOT block aiman sync. The old hash stored in
// aim.local.yaml must not be used to detect "dirty" state.
func TestSyncAllowsCleanStaleSyncedHash(t *testing.T) {
	bareURL := setupGitSource(t)

	fakeHome := t.TempDir()
	if err := os.MkdirAll(filepath.Join(fakeHome, ".claude"), 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}

	workDir := t.TempDir()

	// Init + first sync
	if _, _, err := runAimCmd(t, fakeHome, workDir, "init", "--path", workDir, bareURL); err != nil {
		t.Fatalf("aiman init: %v", err)
	}
	if _, _, err := runSyncCmd(t, fakeHome, workDir); err != nil {
		t.Fatalf("aiman sync (initial): %v", err)
	}

	// Corrupt synced_hash to an obviously fake/old value
	cfg, err := localconfig.Load(workDir)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.SyncedHash = "deadbeefdeadbeefdeadbeefdeadbeef12345678" // nonexistent hash
	if err := localconfig.Save(workDir, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	// Verify no files were actually modified (git status is clean)
	statusOut, err := exec.Command("git", "-C", workDir, "status", "--porcelain").Output()
	if err != nil {
		t.Fatalf("git status: %v", err)
	}
	if strings.TrimSpace(string(statusOut)) != "" {
		t.Fatalf("precondition failed: worktree not clean: %q", string(statusOut))
	}

	// aiman sync must succeed: stale synced_hash alone is not a blocker
	_, stderr, syncErr := runSyncCmd(t, fakeHome, workDir)
	if syncErr != nil {
		t.Errorf("aiman sync blocked by stale synced_hash alone (regression): %v\nstderr: %s", syncErr, stderr)
	}
}

// TestSyncBlocksLocalAheadWithPreciseMessage verifies that aiman sync fails with
// "local commits are not published" when HEAD is ahead of origin/main (unpublished
// local commits), and NOT with "uncommitted" or "history diverged".
func TestSyncBlocksLocalAheadWithPreciseMessage(t *testing.T) {
	bareURL := setupGitSource(t)

	fakeHome := t.TempDir()
	if err := os.MkdirAll(filepath.Join(fakeHome, ".claude"), 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}

	workDir := t.TempDir()

	// Init + first sync
	if _, _, err := runAimCmd(t, fakeHome, workDir, "init", "--path", workDir, bareURL); err != nil {
		t.Fatalf("aiman init: %v", err)
	}
	if _, _, err := runSyncCmd(t, fakeHome, workDir); err != nil {
		t.Fatalf("aiman sync (initial): %v", err)
	}

	// Create a local commit that is NOT pushed to origin
	skillFile := filepath.Join(workDir, "skills", "hello.md")
	if err := os.WriteFile(skillFile, []byte("---\nname: hello\ndescription: Local unpublished\n---\n\n# Role\nUnpublished.\n"), 0644); err != nil {
		t.Fatalf("modify skill: %v", err)
	}
	runGitHelper(t, workDir, "config", "user.email", "test@test.com")
	runGitHelper(t, workDir, "config", "user.name", "Test")
	runGitHelper(t, workDir, "add", "skills/hello.md")
	runGitHelper(t, workDir, "commit", "-m", "Local unpublished commit")
	// Intentionally do NOT push to origin

	// aiman sync must fail with "local commits are not published"
	_, stderr, err := runSyncCmd(t, fakeHome, workDir)
	if err == nil {
		t.Fatal("expected aiman sync to fail when local is ahead of origin, got success")
	}

	combined := err.Error() + " " + stderr
	if !strings.Contains(combined, "local commits are not published") {
		t.Errorf("expected 'local commits are not published' in error, got: %v / stderr: %s", err, stderr)
	}

	// Must NOT contain misleading wording
	if strings.Contains(combined, "history diverged") {
		t.Errorf("got 'history diverged' but expected 'local commits are not published': %v / stderr: %s", err, stderr)
	}
	if strings.Contains(combined, "uncommitted") {
		t.Errorf("got 'uncommitted' but expected 'local commits are not published': %v / stderr: %s", err, stderr)
	}
}

// TestSyncBlocksDivergedHistory verifies that aiman sync fails with "history diverged"
// when the local and remote histories have genuinely diverged (neither is an ancestor
// of the other).
func TestSyncBlocksDivergedHistory(t *testing.T) {
	bareURL := setupGitSource(t)

	fakeHome := t.TempDir()
	if err := os.MkdirAll(filepath.Join(fakeHome, ".claude"), 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}

	workDir := t.TempDir()

	// Init + first sync
	if _, _, err := runAimCmd(t, fakeHome, workDir, "init", "--path", workDir, bareURL); err != nil {
		t.Fatalf("aiman init: %v", err)
	}
	if _, _, err := runSyncCmd(t, fakeHome, workDir); err != nil {
		t.Fatalf("aiman sync (initial): %v", err)
	}

	// Create a local commit in workDir (not pushed)
	localSkill := filepath.Join(workDir, "skills", "hello.md")
	if err := os.WriteFile(localSkill, []byte("---\nname: hello\ndescription: Local diverge\n---\n\n# Role\nLocal.\n"), 0644); err != nil {
		t.Fatalf("modify skill (local): %v", err)
	}
	runGitHelper(t, workDir, "config", "user.email", "test@test.com")
	runGitHelper(t, workDir, "config", "user.name", "Test")
	runGitHelper(t, workDir, "add", "skills/hello.md")
	runGitHelper(t, workDir, "commit", "-m", "Local diverging commit")

	// In a separate clone, push a DIFFERENT commit to origin, making histories diverge
	srcWork := t.TempDir()
	runGitHelper(t, "", "clone", bareURL, srcWork)
	runGitHelper(t, srcWork, "config", "user.email", "test@test.com")
	runGitHelper(t, srcWork, "config", "user.name", "Test")

	remoteSkill := filepath.Join(srcWork, "skills", "hello.md")
	if err := os.WriteFile(remoteSkill, []byte("---\nname: hello\ndescription: Remote diverge\n---\n\n# Role\nRemote.\n"), 0644); err != nil {
		t.Fatalf("modify skill (remote): %v", err)
	}
	runGitHelper(t, srcWork, "add", "skills/hello.md")
	runGitHelper(t, srcWork, "commit", "-m", "Remote diverging commit")
	runGitHelper(t, srcWork, "push", "origin", "main")

	// aiman sync must fail with "history diverged"
	_, stderr, err := runSyncCmd(t, fakeHome, workDir)
	if err == nil {
		t.Fatal("expected aiman sync to fail with diverged history, got success")
	}

	combined := err.Error() + " " + stderr
	if !strings.Contains(combined, "history diverged") {
		t.Errorf("expected 'history diverged' in error, got: %v / stderr: %s", err, stderr)
	}
}

// TestStatusUsesActiveRepoNotCwd verifies that aiman status uses the active repo
// from global config (set by aiman switch), NOT the current working directory.
// Before the fix, running aiman status from an alien cwd (that is not a git repo)
// caused "exit status 128" because git commands were run in the wrong directory.
func TestStatusUsesActiveRepoNotCwd(t *testing.T) {
	bareURL := setupGitSource(t)

	fakeHome := t.TempDir()
	if err := os.MkdirAll(filepath.Join(fakeHome, ".claude"), 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}

	skillsRepo := t.TempDir()

	// Init and sync the skills repo
	if _, _, err := runAimCmd(t, fakeHome, skillsRepo, "init", "--path", skillsRepo, bareURL); err != nil {
		t.Fatalf("aiman init: %v", err)
	}
	if _, _, err := runSyncCmd(t, fakeHome, skillsRepo); err != nil {
		t.Fatalf("aiman sync: %v", err)
	}

	// Use aiman switch to set skillsRepo as the active global repo
	if _, _, err := runAimCmd(t, fakeHome, skillsRepo, "switch", skillsRepo); err != nil {
		t.Fatalf("aiman switch: %v", err)
	}

	// Create an "alien" directory that is NOT a git repo
	alienDir := t.TempDir()

	// Run aiman status from alienDir — must NOT crash with exit status 128
	// The fix: resolveWorkDir reads global config → returns skillsRepo path,
	// so git commands are run in skillsRepo, not alienDir.
	_, stderr, err := runAimCmd(t, fakeHome, alienDir, "status")
	if err != nil {
		t.Errorf("aiman status from alien cwd failed (regression): %v\nstderr: %s", err, stderr)
	}

	// Confirm that the alien dir itself has no git repo (proving we ran from the right place)
	if _, statErr := os.Stat(filepath.Join(alienDir, ".git")); !os.IsNotExist(statErr) {
		t.Log("note: alienDir has a .git dir, test is less meaningful but still valid")
	}
}

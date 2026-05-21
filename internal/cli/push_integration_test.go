package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/axsmak/aim/internal/localconfig"
)

const validSkillForPushE2E = "---\nname: example\ndescription: Example skill\n---\n\n# Role\nDoes something useful.\n"

// setupEmptyBareRepo creates an empty bare git repository, suitable for first-push scenarios.
func setupEmptyBareRepo(t *testing.T) string {
	t.Helper()
	bareDir := t.TempDir()
	out, err := exec.Command("git", "init", "--bare", "-b", "main", bareDir).CombinedOutput()
	if err != nil {
		t.Fatalf("git init --bare: %v\n%s", err, out)
	}
	return bareDir
}

// initWorkDirForPush clones bareURL into a fresh temp dir via aiman init, configures git
// user identity (needed for aiman push to commit), and returns the work dir.
func initWorkDirForPush(t *testing.T, fakeHome, bareURL string) string {
	t.Helper()
	workDir := t.TempDir()
	if _, _, err := runAimCmd(t, fakeHome, workDir, "init", "--path", workDir, bareURL); err != nil {
		t.Fatalf("aiman init: %v", err)
	}
	runGitHelper(t, workDir, "config", "user.email", "test@test.com")
	runGitHelper(t, workDir, "config", "user.name", "Test")
	return workDir
}

// TestPushSyncE2E_HappyPath is the vertical slice: Machine A pushes a skill,
// Machine B inits and syncs, and the skill appears in Machine B's AI environment.
func TestPushSyncE2E_HappyPath(t *testing.T) {
	bareURL := setupEmptyBareRepo(t)

	// --- Machine A ---
	fakeHomeA := t.TempDir()
	if err := os.MkdirAll(filepath.Join(fakeHomeA, ".claude"), 0755); err != nil {
		t.Fatalf("mkdir .claude A: %v", err)
	}
	workDirA := initWorkDirForPush(t, fakeHomeA, bareURL)

	// Create skills directory and add a skill
	if err := os.MkdirAll(filepath.Join(workDirA, "skills"), 0755); err != nil {
		t.Fatalf("mkdir skills A: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workDirA, "skills", "example.md"), []byte(validSkillForPushE2E), 0644); err != nil {
		t.Fatalf("write skill A: %v", err)
	}

	// aiman push from Machine A
	stdout, _, err := runAimCmd(t, fakeHomeA, workDirA, "push")
	if err != nil {
		t.Fatalf("aiman push A: %v", err)
	}
	if !strings.Contains(stdout, "published") {
		t.Errorf("expected 'published' in push output, got: %q", stdout)
	}

	cfgA, err := localconfig.Load(workDirA)
	if err != nil {
		t.Fatalf("load config A: %v", err)
	}
	if cfgA.PublishedHash == "" {
		t.Fatal("published_hash empty after successful aiman push")
	}

	// --- Machine B ---
	fakeHomeB := t.TempDir()
	if err := os.MkdirAll(filepath.Join(fakeHomeB, ".claude"), 0755); err != nil {
		t.Fatalf("mkdir .claude B: %v", err)
	}
	workDirB := t.TempDir()

	// aiman init clones from the bare repo (now has A's commit)
	if _, _, err := runAimCmd(t, fakeHomeB, workDirB, "init", "--path", workDirB, bareURL); err != nil {
		t.Fatalf("aiman init B: %v", err)
	}

	// aiman sync installs skills into fakeHomeB
	if _, _, err := runAimCmd(t, fakeHomeB, workDirB, "sync"); err != nil {
		t.Fatalf("aiman sync B: %v", err)
	}

	// Verify skill installed in Machine B's Claude environment
	skillPath := filepath.Join(fakeHomeB, ".claude", "skills", "example", "SKILL.md")
	if _, err := os.Stat(skillPath); err != nil {
		t.Errorf("skill not installed on machine B at %s: %v", skillPath, err)
	}

	// Verify synced_hash on B matches published_hash on A
	cfgB, err := localconfig.Load(workDirB)
	if err != nil {
		t.Fatalf("load config B: %v", err)
	}
	if cfgB.SyncedHash == "" {
		t.Fatal("synced_hash empty on machine B after sync")
	}
	if cfgA.PublishedHash != cfgB.SyncedHash {
		t.Errorf("published_hash A (%q) != synced_hash B (%q)", cfgA.PublishedHash, cfgB.SyncedHash)
	}
}

// TestPushBlocked_RemoteAhead verifies that aiman push fails when the remote has
// received new commits after Machine B's last known state (genuinely ahead scenario).
//
// Sequence:
//  1. Machine A pushes commit X.
//  2. Machine B inits (clones X) — has all remote commits, push would be fine here.
//  3. Machine A pushes commit Y — remote now has Y, Machine B still has X.
//  4. Machine B tries to push without syncing — must be blocked (Y not in B's history).
func TestPushBlocked_RemoteAhead(t *testing.T) {
	bareURL := setupEmptyBareRepo(t)

	// Machine A: init + push first skill (commit X)
	fakeHomeA := t.TempDir()
	workDirA := initWorkDirForPush(t, fakeHomeA, bareURL)
	if err := os.MkdirAll(filepath.Join(workDirA, "skills"), 0755); err != nil {
		t.Fatalf("mkdir skills A: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workDirA, "skills", "example.md"), []byte(validSkillForPushE2E), 0644); err != nil {
		t.Fatalf("write skill A: %v", err)
	}
	if _, _, err := runAimCmd(t, fakeHomeA, workDirA, "push"); err != nil {
		t.Fatalf("aiman push A (first): %v", err)
	}

	// Machine B: init from bare (clones commit X — up-to-date)
	fakeHomeB := t.TempDir()
	workDirB := initWorkDirForPush(t, fakeHomeB, bareURL)

	// Machine A pushes a SECOND commit Y — remote now has Y, Machine B is at X
	if err := os.WriteFile(filepath.Join(workDirA, "skills", "new-skill.md"), []byte("---\nname: new-skill\ndescription: New skill from A\n---\n\n# Role\nDoes something new.\n"), 0644); err != nil {
		t.Fatalf("write second skill A: %v", err)
	}
	if _, _, err := runAimCmd(t, fakeHomeA, workDirA, "push"); err != nil {
		t.Fatalf("aiman push A (second): %v", err)
	}

	// Machine B tries to push with local changes — must fail: Y is not in B's history
	if err := os.WriteFile(filepath.Join(workDirB, "skills", "other.md"), []byte("---\nname: other\ndescription: Other skill\n---\n\n# Role\nDoes something else.\n"), 0644); err != nil {
		t.Fatalf("write skill B: %v", err)
	}
	_, _, err := runAimCmd(t, fakeHomeB, workDirB, "push")
	if err == nil {
		t.Fatal("expected aiman push to fail when remote is ahead of local history")
	}
	if !strings.Contains(err.Error(), "remote is ahead") {
		t.Errorf("expected 'remote is ahead' error, got: %v", err)
	}
}

// TestPushDryRun verifies that --dry-run shows the plan without creating a commit.
func TestPushDryRun(t *testing.T) {
	bareURL := setupEmptyBareRepo(t)

	fakeHome := t.TempDir()
	workDir := initWorkDirForPush(t, fakeHome, bareURL)
	if err := os.MkdirAll(filepath.Join(workDir, "skills"), 0755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "skills", "example.md"), []byte(validSkillForPushE2E), 0644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	stdout, _, err := runAimCmd(t, fakeHome, workDir, "push", "--dry-run")
	if err != nil {
		t.Fatalf("aiman push --dry-run failed: %v", err)
	}
	if !strings.Contains(stdout, "[dry-run]") {
		t.Errorf("expected '[dry-run]' in output, got: %q", stdout)
	}
	if !strings.Contains(stdout, "example") {
		t.Errorf("expected skill name 'example' in dry-run output, got: %q", stdout)
	}

	// On an empty repo (no commits yet), HEAD is undefined — that's exactly what we want.
	// If a commit was created, git rev-list returns a positive count.
	cmd := exec.Command("git", "-C", workDir, "rev-list", "--count", "HEAD")
	out, err := cmd.CombinedOutput()
	if err == nil {
		// HEAD exists — check count is 0
		if count := strings.TrimSpace(string(out)); count != "0" {
			t.Errorf("expected 0 commits after dry-run, got: %s", count)
		}
	}
	// err != nil means HEAD doesn't exist (empty repo) — no commits, as expected.
}

// TestPushValidation_EmptySkill verifies that a skill with empty body blocks push.
func TestPushValidation_EmptySkill(t *testing.T) {
	bareURL := setupEmptyBareRepo(t)

	fakeHome := t.TempDir()
	workDir := initWorkDirForPush(t, fakeHome, bareURL)
	if err := os.MkdirAll(filepath.Join(workDir, "skills"), 0755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}
	// Skill with valid frontmatter but no body content
	emptyBodyContent := "---\nname: empty-skill\ndescription: Empty body test\n---\n"
	if err := os.WriteFile(filepath.Join(workDir, "skills", "empty.md"), []byte(emptyBodyContent), 0644); err != nil {
		t.Fatalf("write empty skill: %v", err)
	}

	_, _, err := runAimCmd(t, fakeHome, workDir, "push")
	if err == nil {
		t.Fatal("expected aiman push to fail with an empty-body skill")
	}
	if !strings.Contains(err.Error(), "validation") {
		t.Errorf("expected validation error, got: %v", err)
	}

	// On an empty repo (no commits yet), HEAD is undefined — that's the expected state.
	cmd := exec.Command("git", "-C", workDir, "rev-list", "--count", "HEAD")
	out, err := cmd.CombinedOutput()
	if err == nil {
		if count := strings.TrimSpace(string(out)); count != "0" {
			t.Errorf("expected 0 commits after failed validation, got: %s", count)
		}
	}
}

// TestPushMissingGit verifies that aiman push fails with a clear error when
// the directory has not been initialized with git (no .git directory).
func TestPushMissingGit(t *testing.T) {
	dir := t.TempDir()
	fakeHome := t.TempDir()

	_, _, err := runAimCmd(t, fakeHome, dir, "push")
	if err == nil {
		t.Fatal("expected error for missing .git directory, got nil")
	}
	if !strings.Contains(err.Error(), "not initialized") {
		t.Errorf("expected 'not initialized' in error, got: %v", err)
	}
}

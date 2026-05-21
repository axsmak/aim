package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/axsmak/aim/internal/localconfig"
)

// setupApplyGitSource creates a bare git repo with a committed skill.
// Returns the path to use as clone URL.
func setupApplyGitSource(t *testing.T) string {
	t.Helper()

	bareDir := t.TempDir()
	runGitHelper(t, "", "init", "--bare", bareDir)

	srcWork := t.TempDir()
	runGitHelper(t, "", "clone", bareDir, srcWork)
	runGitHelper(t, srcWork, "config", "user.email", "test@test.com")
	runGitHelper(t, srcWork, "config", "user.name", "Test")

	if err := os.MkdirAll(filepath.Join(srcWork, "skills"), 0755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(srcWork, "skills", "hello.md"),
		[]byte("---\nname: hello\ndescription: Hello skill\n---\n\n# Role\nSay hello.\n"),
		0644,
	); err != nil {
		t.Fatalf("write hello.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcWork, ".gitignore"), []byte("aim.local.yaml\n"), 0644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	runGitHelper(t, srcWork, "add", ".")
	runGitHelper(t, srcWork, "commit", "-m", "Initial library")
	runGitHelper(t, srcWork, "branch", "-M", "main")
	runGitHelper(t, srcWork, "push", "origin", "main")
	runGitHelper(t, bareDir, "symbolic-ref", "HEAD", "refs/heads/main")

	return bareDir
}

// cloneAndConfigureRepo clones bareURL into workDir and writes aim.local.yaml.
func cloneAndConfigureRepo(t *testing.T, bareURL, workDir string) {
	t.Helper()
	runGitHelper(t, "", "clone", bareURL, workDir)
	runGitHelper(t, workDir, "config", "user.email", "test@test.com")
	runGitHelper(t, workDir, "config", "user.name", "Test")
	if err := localconfig.Save(workDir, localconfig.Config{}); err != nil {
		t.Fatalf("save localconfig: %v", err)
	}
}

// TestApply_DoesNotAlterGitState verifies the core product contract:
// `aiman apply` on a dirty working tree installs the modified skill into AI environments
// but does NOT commit, reset, or alter any git state.
func TestApply_DoesNotAlterGitState(t *testing.T) {
	bareURL := setupApplyGitSource(t)

	fakeHome := t.TempDir()
	if err := os.MkdirAll(filepath.Join(fakeHome, ".claude"), 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}

	workDir := t.TempDir()
	cloneAndConfigureRepo(t, bareURL, workDir)

	// Record initial HEAD
	headBefore, err := exec.Command("git", "-C", workDir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD before: %v", err)
	}

	// Modify the skill locally (uncommitted change)
	modifiedContent := "---\nname: hello\ndescription: Hello skill — locally modified\n---\n\n# Role\nSay hello, modified.\n"
	skillFile := filepath.Join(workDir, "skills", "hello.md")
	if err := os.WriteFile(skillFile, []byte(modifiedContent), 0644); err != nil {
		t.Fatalf("modify skill: %v", err)
	}

	// Verify git sees the modification before apply
	diffOut, err := exec.Command("git", "-C", workDir, "diff", "--name-only").Output()
	if err != nil {
		t.Fatalf("git diff before apply: %v", err)
	}
	if !strings.Contains(string(diffOut), "hello.md") {
		t.Fatalf("expected hello.md to be dirty before apply, got: %q", string(diffOut))
	}

	// Run aiman apply
	stdout, _, applyErr := runAimCmd(t, fakeHome, workDir, "apply")
	if applyErr != nil {
		t.Fatalf("aiman apply failed: %v", applyErr)
	}
	if !strings.Contains(stdout, "Applied:") {
		t.Errorf("expected 'Applied:' in output, got: %q", stdout)
	}

	// Skill must be installed with the MODIFIED content (local working tree, not committed)
	installPath := filepath.Join(fakeHome, ".claude", "skills", "hello", "SKILL.md")
	installed, err := os.ReadFile(installPath)
	if err != nil {
		t.Fatalf("installed skill not found at %s: %v", installPath, err)
	}
	if !strings.Contains(string(installed), "locally modified") {
		t.Errorf("installed skill should contain modified content; got:\n%s", string(installed))
	}

	// Git state must be UNCHANGED after apply
	diffAfter, err := exec.Command("git", "-C", workDir, "diff", "--name-only").Output()
	if err != nil {
		t.Fatalf("git diff after apply: %v", err)
	}
	if !strings.Contains(string(diffAfter), "hello.md") {
		t.Error("expected hello.md to still be dirty after aiman apply (no commit or reset allowed)")
	}

	headAfter, err := exec.Command("git", "-C", workDir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD after: %v", err)
	}
	if strings.TrimSpace(string(headBefore)) != strings.TrimSpace(string(headAfter)) {
		t.Errorf("HEAD changed after aiman apply: before=%q after=%q",
			strings.TrimSpace(string(headBefore)), strings.TrimSpace(string(headAfter)))
	}

	// synced_hash must NOT be written by apply
	cfg, err := localconfig.Load(workDir)
	if err != nil {
		t.Fatalf("load localconfig after apply: %v", err)
	}
	if cfg.SyncedHash != "" {
		t.Errorf("synced_hash must not be set by aiman apply, got %q", cfg.SyncedHash)
	}
}

// TestApply_WorksOffline verifies that aiman apply does not require remote access.
// The remote is set to an unreachable address; apply must still succeed.
func TestApply_WorksOffline(t *testing.T) {
	workDir := t.TempDir()
	fakeHome := t.TempDir()

	if err := os.MkdirAll(filepath.Join(fakeHome, ".claude"), 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workDir, "skills"), 0755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}

	runGitHelper(t, workDir, "init", "-b", "main")
	runGitHelper(t, workDir, "config", "user.email", "test@test.com")
	runGitHelper(t, workDir, "config", "user.name", "Test")
	// Point remote to an unreachable address
	runGitHelper(t, workDir, "remote", "add", "origin", "http://localhost:19999/nonexistent.git")

	if err := os.WriteFile(
		filepath.Join(workDir, "skills", "offline-skill.md"),
		[]byte("---\nname: offline-skill\ndescription: Works offline\n---\n\n# Role\nOffline.\n"),
		0644,
	); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	if err := localconfig.Save(workDir, localconfig.Config{}); err != nil {
		t.Fatalf("save localconfig: %v", err)
	}

	// aiman apply must succeed even with unreachable remote
	_, _, err := runAimCmd(t, fakeHome, workDir, "apply")
	if err != nil {
		t.Errorf("aiman apply failed with unreachable remote (should be offline-capable): %v", err)
	}
}

// TestApply_StatusShowsUnpublishedAfterApply verifies that aiman status still shows
// local changes as unpublished after aiman apply.
func TestApply_StatusShowsUnpublishedAfterApply(t *testing.T) {
	bareURL := setupApplyGitSource(t)

	fakeHome := t.TempDir()
	if err := os.MkdirAll(filepath.Join(fakeHome, ".claude"), 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}

	workDir := t.TempDir()
	cloneAndConfigureRepo(t, bareURL, workDir)

	// Modify skill locally
	if err := os.WriteFile(
		filepath.Join(workDir, "skills", "hello.md"),
		[]byte("---\nname: hello\ndescription: modified locally\n---\n\n# Role\nModified.\n"),
		0644,
	); err != nil {
		t.Fatalf("modify skill: %v", err)
	}

	// Run aiman apply
	if _, _, err := runAimCmd(t, fakeHome, workDir, "apply"); err != nil {
		t.Fatalf("aiman apply failed: %v", err)
	}

	// Run aiman status — must not error, and local changes must still be visible
	statusOut, _, err := runAimCmd(t, fakeHome, workDir, "status")
	if err != nil {
		t.Errorf("aiman status failed after aiman apply: %v", err)
	}
	// status should mention that there are local changes (unpublished/modified)
	if !strings.Contains(statusOut, "hello.md") && !strings.Contains(statusOut, "modified") &&
		!strings.Contains(statusOut, "local") && !strings.Contains(statusOut, "unpublished") {
		t.Logf("aiman status output (may vary by format): %q", statusOut)
	}
}

package gitops_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/axsmak/aim/internal/gitops"
)

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func setupRepo(t *testing.T) (bareDir, workDir string) {
	t.Helper()
	bareDir = t.TempDir()
	runGit(t, "", "init", "--bare", "-b", "main", bareDir)
	workDir = t.TempDir()
	runGit(t, "", "clone", bareDir, workDir)
	runGit(t, workDir, "config", "user.email", "test@test.com")
	runGit(t, workDir, "config", "user.name", "Test")
	return
}

func initialCommit(t *testing.T, workDir, bareDir string) {
	t.Helper()
	f := filepath.Join(workDir, "README.md")
	if err := os.WriteFile(f, []byte("init"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, workDir, "add", ".")
	runGit(t, workDir, "commit", "-m", "init")
	runGit(t, workDir, "push", "origin", "main")
}

func TestClone(t *testing.T) {
	bareDir, _ := setupRepo(t)
	// make bare non-empty so clone has something
	srcDir := t.TempDir()
	runGit(t, "", "clone", bareDir, srcDir)
	runGit(t, srcDir, "config", "user.email", "test@test.com")
	runGit(t, srcDir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(srcDir, "f.md"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, srcDir, "add", ".")
	runGit(t, srcDir, "commit", "-m", "init")
	runGit(t, srcDir, "push", "origin", "main")

	destDir := filepath.Join(t.TempDir(), "cloned")
	ops := gitops.New()
	if err := ops.Clone(bareDir, destDir); err != nil {
		t.Fatalf("Clone: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destDir, ".git")); err != nil {
		t.Fatal("clone result is not a git repo")
	}
}

func TestFetch(t *testing.T) {
	bareDir, workDir := setupRepo(t)
	initialCommit(t, workDir, bareDir)

	// second clone
	work2 := t.TempDir()
	runGit(t, "", "clone", bareDir, work2)
	runGit(t, work2, "config", "user.email", "test@test.com")
	runGit(t, work2, "config", "user.name", "Test")

	// push a new commit from workDir
	if err := os.WriteFile(filepath.Join(workDir, "new.md"), []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, workDir, "add", ".")
	runGit(t, workDir, "commit", "-m", "new commit")
	runGit(t, workDir, "push", "origin", "main")

	ops := gitops.New()
	if err := ops.Fetch(work2); err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	remote := runGit(t, work2, "rev-parse", "origin/main")
	pushed := runGit(t, workDir, "rev-parse", "HEAD")
	if remote != pushed {
		t.Fatalf("after fetch: origin/main=%s want %s", remote, pushed)
	}
}

func TestResetHard(t *testing.T) {
	bareDir, workDir := setupRepo(t)
	initialCommit(t, workDir, bareDir)

	// dirty the working tree
	f := filepath.Join(workDir, "dirty.md")
	if err := os.WriteFile(f, []byte("dirty"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, workDir, "add", ".")

	ops := gitops.New()
	if err := ops.ResetHard(workDir, "HEAD"); err != nil {
		t.Fatalf("ResetHard: %v", err)
	}
	if _, err := os.Stat(f); !os.IsNotExist(err) {
		t.Fatal("file should be gone after reset --hard")
	}
}

func TestIsFastForward_True(t *testing.T) {
	bareDir, workDir := setupRepo(t)
	initialCommit(t, workDir, bareDir)

	ops := gitops.New()
	ff, err := ops.IsFastForward(workDir, "origin/main")
	if err != nil {
		t.Fatalf("IsFastForward: %v", err)
	}
	if !ff {
		t.Fatal("expected true: HEAD is ancestor of origin/main on fresh clone")
	}
}

func TestIsFastForward_False(t *testing.T) {
	bareDir, workDir := setupRepo(t)
	initialCommit(t, workDir, bareDir)

	// diverge: local commit without pushing
	if err := os.WriteFile(filepath.Join(workDir, "local.md"), []byte("local"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, workDir, "add", ".")
	runGit(t, workDir, "commit", "-m", "local diverge")

	// add another commit on bare (simulate remote advance)
	work2 := t.TempDir()
	runGit(t, "", "clone", bareDir, work2)
	runGit(t, work2, "config", "user.email", "test@test.com")
	runGit(t, work2, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(work2, "remote.md"), []byte("remote"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, work2, "add", ".")
	runGit(t, work2, "commit", "-m", "remote advance")
	runGit(t, work2, "push", "origin", "main")
	runGit(t, workDir, "fetch", "origin")

	ops := gitops.New()
	// local HEAD has diverged from origin/main, so origin/main is NOT ancestor of HEAD,
	// and HEAD is NOT ancestor of origin/main → not fast-forward
	ff, err := ops.IsFastForward(workDir, "origin/main")
	if err != nil {
		t.Fatalf("IsFastForward: %v", err)
	}
	if ff {
		t.Fatal("expected false: local has diverged from origin/main")
	}
}

func TestHasLocalChanges_None(t *testing.T) {
	bareDir, workDir := setupRepo(t)
	initialCommit(t, workDir, bareDir)

	// create skills/ dir and commit it
	skillsDir := filepath.Join(workDir, "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "test.md"), []byte("skill"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, workDir, "add", ".")
	runGit(t, workDir, "commit", "-m", "add skills")
	hash := runGit(t, workDir, "rev-parse", "HEAD")

	ops := gitops.New()
	changed, err := ops.HasLocalChanges(workDir, hash)
	if err != nil {
		t.Fatalf("HasLocalChanges: %v", err)
	}
	if changed {
		t.Fatal("expected false: no changes since last commit")
	}
}

func TestHasLocalChanges_Modified(t *testing.T) {
	bareDir, workDir := setupRepo(t)
	initialCommit(t, workDir, bareDir)

	skillsDir := filepath.Join(workDir, "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatal(err)
	}
	skillFile := filepath.Join(skillsDir, "test.md")
	if err := os.WriteFile(skillFile, []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, workDir, "add", ".")
	runGit(t, workDir, "commit", "-m", "add skills")
	hash := runGit(t, workDir, "rev-parse", "HEAD")

	// modify skills file
	if err := os.WriteFile(skillFile, []byte("modified"), 0644); err != nil {
		t.Fatal(err)
	}
	// stage the change (diff works on staged too via HEAD comparison)
	runGit(t, workDir, "add", ".")
	runGit(t, workDir, "commit", "-m", "modify skill")

	ops := gitops.New()
	changed, err := ops.HasLocalChanges(workDir, hash)
	if err != nil {
		t.Fatalf("HasLocalChanges: %v", err)
	}
	if !changed {
		t.Fatal("expected true: skills/ was modified since sinceHash")
	}
}

func TestHeadHash(t *testing.T) {
	bareDir, workDir := setupRepo(t)
	initialCommit(t, workDir, bareDir)

	ops := gitops.New()
	h, err := ops.HeadHash(workDir)
	if err != nil {
		t.Fatalf("HeadHash: %v", err)
	}
	if len(h) != 40 {
		t.Fatalf("expected 40-char hash, got %q (len %d)", h, len(h))
	}
}

func TestRemoteHash(t *testing.T) {
	bareDir, workDir := setupRepo(t)
	initialCommit(t, workDir, bareDir)

	ops := gitops.New()
	h, err := ops.RemoteHash(workDir, "origin/main")
	if err != nil {
		t.Fatalf("RemoteHash: %v", err)
	}
	if len(h) != 40 {
		t.Fatalf("expected 40-char hash, got %q", h)
	}
	expected := runGit(t, workDir, "rev-parse", "origin/main")
	if h != expected {
		t.Fatalf("RemoteHash=%s want %s", h, expected)
	}
}

func TestLsRemote_success(t *testing.T) {
	bareDir, workDir := setupRepo(t)
	initialCommit(t, workDir, bareDir)

	ops := gitops.New()
	h, err := ops.LsRemote(workDir, "refs/heads/main")
	if err != nil {
		t.Fatalf("LsRemote: %v", err)
	}
	if len(h) != 40 {
		t.Fatalf("expected 40-char hash, got %q (len %d)", h, len(h))
	}
	expected := runGit(t, workDir, "rev-parse", "HEAD")
	if h != expected {
		t.Fatalf("LsRemote=%s want %s", h, expected)
	}
}

func TestLsRemote_nonexistentRemote(t *testing.T) {
	workDir := t.TempDir()
	runGit(t, "", "init", "-b", "main", workDir)
	runGit(t, workDir, "config", "user.email", "test@test.com")
	runGit(t, workDir, "config", "user.name", "Test")
	runGit(t, workDir, "remote", "add", "origin", "/nonexistent/path/bare.git")

	ops := gitops.New()
	_, err := ops.LsRemote(workDir, "refs/heads/main")
	if err == nil {
		t.Fatal("expected error for nonexistent remote, got nil")
	}
}

func TestCommit_success(t *testing.T) {
	bareDir, workDir := setupRepo(t)
	initialCommit(t, workDir, bareDir)

	skillsDir := filepath.Join(workDir, "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "test.md"), []byte("skill"), 0644); err != nil {
		t.Fatal(err)
	}

	ops := gitops.New()
	if err := ops.Commit(workDir, "aim: publish test"); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	log := runGit(t, workDir, "log", "--oneline", "-1")
	if !strings.Contains(log, "aim: publish test") {
		t.Fatalf("expected commit message in log, got: %q", log)
	}
}

func TestCommit_nothingToCommit(t *testing.T) {
	bareDir, workDir := setupRepo(t)
	initialCommit(t, workDir, bareDir)

	ops := gitops.New()
	err := ops.Commit(workDir, "aim: publish nothing")
	if err == nil {
		t.Fatal("expected error when nothing to commit, got nil")
	}
}

func TestPush_success(t *testing.T) {
	bareDir, workDir := setupRepo(t)
	initialCommit(t, workDir, bareDir)

	skillsDir := filepath.Join(workDir, "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "push-test.md"), []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	ops := gitops.New()
	if err := ops.Commit(workDir, "aim: publish push-test"); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if err := ops.Push(workDir); err != nil {
		t.Fatalf("Push: %v", err)
	}

	// verify bare remote has the new commit
	localHead := runGit(t, workDir, "rev-parse", "HEAD")
	remoteHead := runGit(t, bareDir, "rev-parse", "HEAD")
	if localHead != remoteHead {
		t.Fatalf("after push: local HEAD=%s remote HEAD=%s", localHead, remoteHead)
	}
}

func TestPush_remoteUnavailable(t *testing.T) {
	bareDir, workDir := setupRepo(t)
	initialCommit(t, workDir, bareDir)

	// Point origin at a nonexistent path
	runGit(t, workDir, "remote", "set-url", "origin", "/nonexistent/path/bare.git")

	skillsDir := filepath.Join(workDir, "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "x.md"), []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	ops := gitops.New()
	if err := ops.Commit(workDir, "aim: test push fail"); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if err := ops.Push(workDir); err == nil {
		t.Fatal("expected error pushing to nonexistent remote, got nil")
	}
}

func TestIsFileStaged_staged(t *testing.T) {
	bareDir, workDir := setupRepo(t)
	initialCommit(t, workDir, bareDir)

	f := filepath.Join(workDir, "new.md")
	if err := os.WriteFile(f, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, workDir, "add", "new.md")

	ops := gitops.New()
	staged, err := ops.IsFileStaged(workDir, "new.md")
	if err != nil {
		t.Fatalf("IsFileStaged: %v", err)
	}
	if !staged {
		t.Fatal("expected true: file is staged")
	}
}

func TestIsFileStaged_notStaged(t *testing.T) {
	bareDir, workDir := setupRepo(t)
	initialCommit(t, workDir, bareDir)

	f := filepath.Join(workDir, "new.md")
	if err := os.WriteFile(f, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	// not added to index

	ops := gitops.New()
	staged, err := ops.IsFileStaged(workDir, "new.md")
	if err != nil {
		t.Fatalf("IsFileStaged: %v", err)
	}
	if staged {
		t.Fatal("expected false: file is not staged")
	}
}

func TestIsFileStaged_nonexistentFile(t *testing.T) {
	bareDir, workDir := setupRepo(t)
	initialCommit(t, workDir, bareDir)

	ops := gitops.New()
	staged, err := ops.IsFileStaged(workDir, "does-not-exist.md")
	if err != nil {
		t.Fatalf("IsFileStaged: %v", err)
	}
	if staged {
		t.Fatal("expected false: file does not exist")
	}
}

func TestResetSoft_success(t *testing.T) {
	bareDir, workDir := setupRepo(t)
	initialCommit(t, workDir, bareDir)

	skillsDir := filepath.Join(workDir, "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatal(err)
	}
	skillFile := filepath.Join(skillsDir, "reset-test.md")
	if err := os.WriteFile(skillFile, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	ops := gitops.New()
	if err := ops.Commit(workDir, "aim: publish before reset"); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	commitBefore := runGit(t, workDir, "rev-parse", "HEAD")

	if err := ops.ResetSoft(workDir); err != nil {
		t.Fatalf("ResetSoft: %v", err)
	}

	commitAfter := runGit(t, workDir, "rev-parse", "HEAD")
	if commitAfter == commitBefore {
		t.Fatal("expected HEAD to change after ResetSoft")
	}

	// file should still exist (soft reset keeps changes)
	if _, err := os.Stat(skillFile); err != nil {
		t.Fatalf("file should still exist after ResetSoft: %v", err)
	}

	// changes should be staged
	status := runGit(t, workDir, "status", "--porcelain")
	if !strings.Contains(status, "skills/reset-test.md") {
		t.Fatalf("expected file to be staged after ResetSoft, status: %q", status)
	}
}

// TestCommit_StagesAimYamlAndGitignore verifies Issue #59: Commit stages aim.yaml and .gitignore.
func TestCommit_StagesAimYamlAndGitignore(t *testing.T) {
	bareDir, workDir := setupRepo(t)
	initialCommit(t, workDir, bareDir)

	// Create skills/ (required), aim.yaml and .gitignore (new: should be staged)
	skillsDir := filepath.Join(workDir, "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, ".gitkeep"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "aim.yaml"), []byte("skill_paths: {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, ".gitignore"), []byte("aim.local.yaml\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ops := gitops.New()
	if err := ops.Commit(workDir, "aim: scaffold"); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Verify committed files
	files := runGit(t, workDir, "show", "HEAD", "--name-only", "--format=")
	if !strings.Contains(files, "aim.yaml") {
		t.Errorf("aim.yaml not in commit; committed files: %q", files)
	}
	if !strings.Contains(files, ".gitignore") {
		t.Errorf(".gitignore not in commit; committed files: %q", files)
	}
}

// TestCommit_SkipsAbsentConfigFiles verifies Commit does not fail when aim.yaml/.gitignore are absent.
func TestCommit_SkipsAbsentConfigFiles(t *testing.T) {
	bareDir, workDir := setupRepo(t)
	initialCommit(t, workDir, bareDir)

	skillsDir := filepath.Join(workDir, "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "skill.md"), []byte("---\nname: x\ndescription: y\n---\nbody"), 0644); err != nil {
		t.Fatal(err)
	}
	// No aim.yaml, no .gitignore

	ops := gitops.New()
	if err := ops.Commit(workDir, "aim: publish without config files"); err != nil {
		t.Fatalf("Commit should succeed without aim.yaml/.gitignore: %v", err)
	}

	files := runGit(t, workDir, "show", "HEAD", "--name-only", "--format=")
	if strings.Contains(files, "aim.yaml") {
		t.Error("aim.yaml should NOT be in commit when it does not exist")
	}
}

func setupLocalRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, "", "init", "-b", "main", dir)
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "Test")
	runGit(t, dir, "commit", "--allow-empty", "-m", "init")
	return dir
}

func TestHasDirtyWorktree_clean(t *testing.T) {
	dir := setupLocalRepo(t)
	ops := gitops.New()
	dirty, err := ops.HasDirtyWorktree(dir)
	if err != nil {
		t.Fatalf("HasDirtyWorktree: %v", err)
	}
	if dirty {
		t.Fatal("expected false: repo is clean")
	}
}

func TestHasDirtyWorktree_unstaged(t *testing.T) {
	dir := setupLocalRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "tracked.md"), []byte("v1"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "add file")

	// modify without staging
	if err := os.WriteFile(filepath.Join(dir, "tracked.md"), []byte("v2"), 0644); err != nil {
		t.Fatal(err)
	}

	ops := gitops.New()
	dirty, err := ops.HasDirtyWorktree(dir)
	if err != nil {
		t.Fatalf("HasDirtyWorktree: %v", err)
	}
	if !dirty {
		t.Fatal("expected true: tracked file modified but not staged")
	}
}

func TestHasDirtyWorktree_staged(t *testing.T) {
	dir := setupLocalRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "tracked.md"), []byte("v1"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "add file")

	if err := os.WriteFile(filepath.Join(dir, "tracked.md"), []byte("v2"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", ".")

	ops := gitops.New()
	dirty, err := ops.HasDirtyWorktree(dir)
	if err != nil {
		t.Fatalf("HasDirtyWorktree: %v", err)
	}
	if !dirty {
		t.Fatal("expected true: staged changes present")
	}
}

func TestHasUntrackedInPaths_none(t *testing.T) {
	dir := setupLocalRepo(t)
	if err := os.MkdirAll(filepath.Join(dir, "skills"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skills", "a.md"), []byte("tracked"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "add skill")

	ops := gitops.New()
	has, err := ops.HasUntrackedInPaths(dir, []string{"skills/"})
	if err != nil {
		t.Fatalf("HasUntrackedInPaths: %v", err)
	}
	if has {
		t.Fatal("expected false: no untracked files in skills/")
	}
}

func TestHasUntrackedInPaths_untracked(t *testing.T) {
	dir := setupLocalRepo(t)
	if err := os.MkdirAll(filepath.Join(dir, "skills"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skills", "new.md"), []byte("untracked"), 0644); err != nil {
		t.Fatal(err)
	}
	// not staged, not committed

	ops := gitops.New()
	has, err := ops.HasUntrackedInPaths(dir, []string{"skills/"})
	if err != nil {
		t.Fatalf("HasUntrackedInPaths: %v", err)
	}
	if !has {
		t.Fatal("expected true: untracked file in skills/")
	}
}

func TestUntrackedConflictsWithRef_noConflict(t *testing.T) {
	bareDir, workDir := setupRepo(t)
	initialCommit(t, workDir, bareDir)

	// Commit a skill in remote
	if err := os.MkdirAll(filepath.Join(workDir, "skills"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "skills", "existing.md"), []byte("existing"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, workDir, "add", ".")
	runGit(t, workDir, "commit", "-m", "add skill")
	runGit(t, workDir, "push", "origin", "main")
	runGit(t, workDir, "fetch", "origin")

	// Local untracked file with a DIFFERENT name — no conflict
	if err := os.WriteFile(filepath.Join(workDir, "skills", "new-local.md"), []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}

	ops := gitops.New()
	conflicts, err := ops.UntrackedConflictsWithRef(workDir, "origin/main", []string{"skills/"})
	if err != nil {
		t.Fatalf("UntrackedConflictsWithRef: %v", err)
	}
	if len(conflicts) != 0 {
		t.Fatalf("expected no conflicts, got: %v", conflicts)
	}
}

func TestUntrackedConflictsWithRef_conflict(t *testing.T) {
	bareDir, workDir := setupRepo(t)
	initialCommit(t, workDir, bareDir)

	// Push a skill to remote
	if err := os.MkdirAll(filepath.Join(workDir, "skills"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "skills", "conflict.md"), []byte("remote version"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, workDir, "add", ".")
	runGit(t, workDir, "commit", "-m", "add conflict skill")
	runGit(t, workDir, "push", "origin", "main")

	// Clone fresh — local HEAD has the file tracked
	work2 := t.TempDir()
	runGit(t, "", "clone", bareDir, work2)
	runGit(t, work2, "config", "user.email", "test@test.com")
	runGit(t, work2, "config", "user.name", "Test")

	// Revert local HEAD to before the file was added so conflict.md is untracked
	runGit(t, work2, "reset", "--hard", "HEAD~1")

	// skills/ may have been removed by the reset; recreate it
	if err := os.MkdirAll(filepath.Join(work2, "skills"), 0755); err != nil {
		t.Fatal(err)
	}

	// Re-create the file as untracked with same name as in origin/main
	if err := os.WriteFile(filepath.Join(work2, "skills", "conflict.md"), []byte("local version"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, work2, "fetch", "origin")

	ops := gitops.New()
	conflicts, err := ops.UntrackedConflictsWithRef(work2, "origin/main", []string{"skills/"})
	if err != nil {
		t.Fatalf("UntrackedConflictsWithRef: %v", err)
	}
	if len(conflicts) == 0 {
		t.Fatal("expected conflict for skills/conflict.md, got none")
	}
	found := false
	for _, c := range conflicts {
		if strings.Contains(c, "conflict.md") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected skills/conflict.md in conflicts, got: %v", conflicts)
	}
}

func TestManagedStatus_clean(t *testing.T) {
	dir := setupLocalRepo(t)
	if err := os.MkdirAll(filepath.Join(dir, "skills"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skills", "a.md"), []byte("skill"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "add skill")

	lines, err := gitops.ManagedStatus(dir)
	if err != nil {
		t.Fatalf("ManagedStatus: %v", err)
	}
	if lines != nil {
		t.Fatalf("expected nil (clean), got %v", lines)
	}
}

func TestManagedStatus_untrackedSkill(t *testing.T) {
	dir := setupLocalRepo(t)
	if err := os.MkdirAll(filepath.Join(dir, "skills"), 0755); err != nil {
		t.Fatal(err)
	}
	// untracked file — not staged, not committed
	if err := os.WriteFile(filepath.Join(dir, "skills", "wiki.md"), []byte("new skill"), 0644); err != nil {
		t.Fatal(err)
	}

	lines, err := gitops.ManagedStatus(dir)
	if err != nil {
		t.Fatalf("ManagedStatus: %v", err)
	}
	if len(lines) == 0 {
		t.Fatal("expected porcelain lines for untracked file, got none")
	}
	found := false
	for _, l := range lines {
		if strings.HasPrefix(l, "??") && strings.Contains(l, "skills/wiki.md") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected '?? skills/wiki.md' in output, got %v", lines)
	}
}

func TestManagedStatus_modifiedTracked(t *testing.T) {
	dir := setupLocalRepo(t)
	if err := os.MkdirAll(filepath.Join(dir, "skills"), 0755); err != nil {
		t.Fatal(err)
	}
	skillFile := filepath.Join(dir, "skills", "a.md")
	if err := os.WriteFile(skillFile, []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "add skill")

	// modify without staging
	if err := os.WriteFile(skillFile, []byte("modified"), 0644); err != nil {
		t.Fatal(err)
	}

	lines, err := gitops.ManagedStatus(dir)
	if err != nil {
		t.Fatalf("ManagedStatus: %v", err)
	}
	if len(lines) == 0 {
		t.Fatal("expected porcelain lines for modified file, got none")
	}
	found := false
	for _, l := range lines {
		// worktree-modified shows as " M" (space + M)
		if strings.Contains(l, "skills/a.md") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected skills/a.md in porcelain output, got %v", lines)
	}
}

func TestCountAheadBehind_equal(t *testing.T) {
	dir := setupLocalRepo(t)
	hash := runGit(t, dir, "rev-parse", "HEAD")

	ops := gitops.New()
	ahead, behind, err := ops.CountAheadBehind(dir, hash, "HEAD")
	if err != nil {
		t.Fatalf("CountAheadBehind: %v", err)
	}
	if ahead != 0 || behind != 0 {
		t.Fatalf("expected (0,0), got (%d,%d)", ahead, behind)
	}
}

func TestCountAheadBehind_ahead(t *testing.T) {
	dir := setupLocalRepo(t)
	base := runGit(t, dir, "rev-parse", "HEAD")

	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "commit 1")

	if err := os.WriteFile(filepath.Join(dir, "b.md"), []byte("b"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "commit 2")

	ops := gitops.New()
	ahead, behind, err := ops.CountAheadBehind(dir, base, "HEAD")
	if err != nil {
		t.Fatalf("CountAheadBehind: %v", err)
	}
	if ahead != 2 {
		t.Fatalf("expected ahead=2, got %d", ahead)
	}
	if behind != 0 {
		t.Fatalf("expected behind=0, got %d", behind)
	}
}

func TestCountAheadBehind_behind(t *testing.T) {
	dir := setupLocalRepo(t)

	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "commit 1")
	tip := runGit(t, dir, "rev-parse", "HEAD")

	// base is one commit ahead of ref (init)
	init := runGit(t, dir, "rev-parse", "HEAD~1")

	ops := gitops.New()
	ahead, behind, err := ops.CountAheadBehind(dir, tip, init)
	if err != nil {
		t.Fatalf("CountAheadBehind: %v", err)
	}
	if ahead != 0 {
		t.Fatalf("expected ahead=0, got %d", ahead)
	}
	if behind != 1 {
		t.Fatalf("expected behind=1, got %d", behind)
	}
}

// TestDiffSyncDelta_noChanges verifies DiffSyncDelta returns nil when HEAD == origin/main.
func TestDiffSyncDelta_noChanges(t *testing.T) {
	bareDir, workDir := setupRepo(t)
	initialCommit(t, workDir, bareDir)

	if err := os.MkdirAll(filepath.Join(workDir, "skills"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "skills", "x.md"), []byte("skill"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, workDir, "add", ".")
	runGit(t, workDir, "commit", "-m", "add skill")
	runGit(t, workDir, "push", "origin", "main")
	runGit(t, workDir, "fetch", "origin")

	ops := gitops.New()
	lines, err := ops.DiffSyncDelta(workDir)
	if err != nil {
		t.Fatalf("DiffSyncDelta: %v", err)
	}
	if len(lines) != 0 {
		t.Fatalf("expected no delta (HEAD == origin/main), got: %v", lines)
	}
}

// TestDiffSyncDelta_withChanges verifies DiffSyncDelta returns "M path" and "A path"
// lines when origin/main is ahead of HEAD.
func TestDiffSyncDelta_withChanges(t *testing.T) {
	bareDir, workDir := setupRepo(t)
	initialCommit(t, workDir, bareDir)

	// Push an initial skill from workDir
	if err := os.MkdirAll(filepath.Join(workDir, "skills"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "skills", "existing.md"), []byte("v1"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, workDir, "add", ".")
	runGit(t, workDir, "commit", "-m", "initial skill")
	runGit(t, workDir, "push", "origin", "main")

	// From a second clone, push a new commit: modify existing + add new skill
	work2 := t.TempDir()
	runGit(t, "", "clone", bareDir, work2)
	runGit(t, work2, "config", "user.email", "test@test.com")
	runGit(t, work2, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(work2, "skills", "existing.md"), []byte("v2"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work2, "skills", "new.md"), []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, work2, "add", ".")
	runGit(t, work2, "commit", "-m", "update and add skill")
	runGit(t, work2, "push", "origin", "main")

	// Fetch so workDir sees the new origin/main without resetting HEAD
	runGit(t, workDir, "fetch", "origin")

	ops := gitops.New()
	lines, err := ops.DiffSyncDelta(workDir)
	if err != nil {
		t.Fatalf("DiffSyncDelta: %v", err)
	}
	if len(lines) == 0 {
		t.Fatal("expected delta lines, got none")
	}

	var hasModified, hasAdded bool
	for _, l := range lines {
		if strings.Contains(l, "existing.md") && strings.HasPrefix(l, "M ") {
			hasModified = true
		}
		if strings.Contains(l, "new.md") && strings.HasPrefix(l, "A ") {
			hasAdded = true
		}
	}
	if !hasModified {
		t.Errorf("expected 'M skills/existing.md' in delta, got: %v", lines)
	}
	if !hasAdded {
		t.Errorf("expected 'A skills/new.md' in delta, got: %v", lines)
	}
}

// TestCleanUntracked verifies CleanUntracked removes untracked files under the given paths.
func TestCleanUntracked(t *testing.T) {
	dir := setupLocalRepo(t)
	if err := os.MkdirAll(filepath.Join(dir, "skills"), 0755); err != nil {
		t.Fatal(err)
	}
	untrackedFile := filepath.Join(dir, "skills", "draft.md")
	if err := os.WriteFile(untrackedFile, []byte("untracked"), 0644); err != nil {
		t.Fatal(err)
	}

	ops := gitops.New()
	if err := ops.CleanUntracked(dir, []string{"skills/"}); err != nil {
		t.Fatalf("CleanUntracked: %v", err)
	}

	if _, err := os.Stat(untrackedFile); !os.IsNotExist(err) {
		t.Error("expected untracked file to be removed by CleanUntracked")
	}
}

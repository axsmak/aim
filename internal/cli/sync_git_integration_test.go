package cli_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/axsmak/aim/internal/cli"
	"github.com/axsmak/aim/internal/localconfig"
)

// aimBinPath is set by TestMain when the binary is successfully built.
var aimBinPath string

func TestMain(m *testing.M) {
	// Build binary for subprocess tests (best-effort; tests that need it will skip if absent).
	binDir, err := os.MkdirTemp("", "aiman-test-bin-*")
	if err == nil {
		bin := filepath.Join(binDir, "aiman")
		// Determine module root: this file is at internal/cli/, root is ../..
		pkgDir, _ := os.Getwd()
		moduleRoot := filepath.Clean(filepath.Join(pkgDir, "../.."))
		cmd := exec.Command("go", "build", "-o", bin, "./cmd/aim")
		cmd.Dir = moduleRoot
		if cmd.Run() == nil {
			aimBinPath = bin
		}
		defer os.RemoveAll(binDir)
	}
	os.Exit(m.Run())
}

// runGitHelper runs git in dir (empty dir = current directory).
func runGitHelper(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// runAimCmd executes any aiman subcommand via cobra, chdir to workDir, HOME=fakeHome.
func runAimCmd(t *testing.T, fakeHome, workDir string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	t.Setenv("HOME", fakeHome)

	oldDir, cdErr := os.Getwd()
	if cdErr != nil {
		t.Fatalf("getwd: %v", cdErr)
	}
	t.Cleanup(func() { os.Chdir(oldDir) })
	if cdErr := os.Chdir(workDir); cdErr != nil {
		t.Fatalf("chdir %s: %v", workDir, cdErr)
	}

	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	root := cli.NewRootCmd("test")
	root.SetOut(outBuf)
	root.SetErr(errBuf)
	root.SetArgs(args)

	err = root.Execute()
	return outBuf.String(), errBuf.String(), err
}

// setupGitSource creates a bare git repo with aim.yaml + skills/hello.md.
// Returns the path to use as clone URL.
func setupGitSource(t *testing.T) string {
	t.Helper()

	bareDir := t.TempDir()
	runGitHelper(t, "", "init", "--bare", bareDir)

	srcWork := t.TempDir()
	runGitHelper(t, "", "clone", bareDir, srcWork)
	runGitHelper(t, srcWork, "config", "user.email", "test@test.com")
	runGitHelper(t, srcWork, "config", "user.name", "Test")

	if err := os.WriteFile(
		filepath.Join(srcWork, "aim.yaml"),
		[]byte("skill_paths:\n  claude-code: ~/.claude/skills\n"),
		0644,
	); err != nil {
		t.Fatalf("write aim.yaml: %v", err)
	}
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
	// Ensure bare repo HEAD points to main (default is master on older git).
	runGitHelper(t, bareDir, "symbolic-ref", "HEAD", "refs/heads/main")

	return bareDir
}

func TestGitBackedSync_InitSyncStatus(t *testing.T) {
	bareURL := setupGitSource(t)

	fakeHome := t.TempDir()
	if err := os.MkdirAll(filepath.Join(fakeHome, ".claude"), 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}

	workDir := t.TempDir() // must be empty for aiman init

	// Step 1: aiman init
	stdout, _, err := runAimCmd(t, fakeHome, workDir, "init", "--path", workDir, bareURL)
	if err != nil {
		t.Fatalf("aiman init failed: %v", err)
	}
	if !strings.Contains(stdout, "initialized") {
		t.Errorf("expected init success message, got: %q", stdout)
	}

	// Verify aim.local.yaml has repo URL
	cfg, err := localconfig.Load(workDir)
	if err != nil {
		t.Fatalf("load config after init: %v", err)
	}
	if cfg.Repo != bareURL {
		t.Errorf("repo: want %q, got %q", bareURL, cfg.Repo)
	}
	// aiman init on an existing library writes synced_hash = HEAD (clone state is known).
	if cfg.SyncedHash == "" {
		t.Error("synced_hash should be set after aiman init on existing library")
	}

	// Step 2: aiman sync
	_, _, err = runSyncCmd(t, fakeHome, workDir)
	if err != nil {
		t.Fatalf("aiman sync failed: %v", err)
	}

	// Verify skill installed in fake HOME
	skillPath := filepath.Join(fakeHome, ".claude", "skills", "hello", "SKILL.md")
	if _, err := os.Stat(skillPath); err != nil {
		t.Errorf("skill not installed at %s: %v", skillPath, err)
	}

	// Verify synced_hash written and matches HEAD
	cfg, err = localconfig.Load(workDir)
	if err != nil {
		t.Fatalf("load config after sync: %v", err)
	}
	if cfg.SyncedHash == "" {
		t.Fatal("synced_hash empty after successful sync")
	}
	headOut, err := exec.Command("git", "-C", workDir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD: %v", err)
	}
	headHash := strings.TrimSpace(string(headOut))
	if cfg.SyncedHash != headHash {
		t.Errorf("synced_hash=%q, HEAD=%q", cfg.SyncedHash, headHash)
	}

	// Step 3: aiman status (verify no error; output goes to real stdout)
	_, _, err = runAimCmd(t, fakeHome, workDir, "status")
	if err != nil {
		t.Errorf("aiman status failed: %v", err)
	}
}

func TestGitBackedSync_ForceOverridesLocalChanges(t *testing.T) {
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
		t.Fatalf("aiman sync: %v", err)
	}

	// Create an UNTRACKED file in managed paths (not tracked by git, so --force can clean it)
	untrackedFile := filepath.Join(workDir, "skills", "untracked-new.md")
	if err := os.WriteFile(untrackedFile, []byte("---\nname: new\ndescription: Untracked\n---\n\n# Role\nNew.\n"), 0644); err != nil {
		t.Fatalf("create untracked file: %v", err)
	}

	// aiman sync --force must succeed (removes untracked files in managed paths)
	_, _, err := runSyncCmd(t, fakeHome, workDir, "--force")
	if err != nil {
		t.Errorf("aiman sync --force failed unexpectedly: %v", err)
	}

	// Untracked file must have been cleaned by git clean -fd
	if _, statErr := os.Stat(untrackedFile); !os.IsNotExist(statErr) {
		t.Error("expected untracked file to be removed by --force sync")
	}

	// Skill should still be installed
	skillPath := filepath.Join(fakeHome, ".claude", "skills", "hello", "SKILL.md")
	if _, err := os.Stat(skillPath); err != nil {
		t.Errorf("skill missing after --force sync: %v", err)
	}
}

// TestGitBackedSync_LocalChangesBlocked verifies that aiman sync exits non-zero
// when local changes exist and --force is not given. Uses subprocess to test
// real binary exit code behavior via main.go.
func TestGitBackedSync_LocalChangesBlocked(t *testing.T) {
	if aimBinPath == "" {
		t.Skip("aiman binary not available (build failed); skipping subprocess test")
	}

	bareURL := setupGitSource(t)

	fakeHome := t.TempDir()
	if err := os.MkdirAll(filepath.Join(fakeHome, ".claude"), 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}

	workDir := t.TempDir()

	// Init + first sync via subprocess to set synced_hash
	initCmd := exec.Command(aimBinPath, "init", "--path", workDir, bareURL)
	initCmd.Dir = workDir
	initCmd.Env = append(os.Environ(), "HOME="+fakeHome)
	if out, err := initCmd.CombinedOutput(); err != nil {
		t.Fatalf("aiman init: %v\n%s", err, out)
	}

	syncCmd1 := exec.Command(aimBinPath, "sync")
	syncCmd1.Dir = workDir
	syncCmd1.Env = append(os.Environ(), "HOME="+fakeHome)
	if out, err := syncCmd1.CombinedOutput(); err != nil {
		t.Fatalf("aiman sync: %v\n%s", err, out)
	}

	// Verify synced_hash is now set
	cfg, err := localconfig.Load(workDir)
	if err != nil || cfg.SyncedHash == "" {
		t.Fatalf("synced_hash not set after initial sync")
	}

	// Modify a skill file
	skillFile := filepath.Join(workDir, "skills", "hello.md")
	if err := os.WriteFile(skillFile, []byte("---\nname: hello\ndescription: modified\n---\n# modified\n"), 0644); err != nil {
		t.Fatalf("modify skill: %v", err)
	}

	// aiman sync without --force must exit non-zero
	syncCmd2 := exec.Command(aimBinPath, "sync")
	syncCmd2.Dir = workDir
	syncCmd2.Env = append(os.Environ(), "HOME="+fakeHome)
	err = syncCmd2.Run()
	if err == nil {
		t.Error("expected aiman sync to fail with local changes, got exit 0")
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		if exitErr.ExitCode() != 1 {
			t.Errorf("expected exit code 1, got %d", exitErr.ExitCode())
		}
	}
}

// TestSyncCorruptYaml verifies that aiman sync fails with a clear error when
// aim.local.yaml contains invalid YAML.
func TestSyncCorruptYaml(t *testing.T) {
	dir := t.TempDir()
	fakeHome := t.TempDir()

	if err := os.Mkdir(filepath.Join(dir, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "aim.local.yaml"), []byte("key: [unclosed bracket"), 0644); err != nil {
		t.Fatal(err)
	}

	_, _, err := runAimCmd(t, fakeHome, dir, "sync")
	if err == nil {
		t.Fatal("expected error for corrupt aim.local.yaml, got nil")
	}
	if !strings.Contains(err.Error(), "cannot parse aim.local.yaml") {
		t.Errorf("expected 'cannot parse aim.local.yaml' in error, got: %v", err)
	}
}

// TestSyncRemoteUnreachable verifies that aiman sync fails with a clear error
// when the remote repository is unreachable.
func TestSyncRemoteUnreachable(t *testing.T) {
	dir := t.TempDir()
	fakeHome := t.TempDir()

	runGitHelper(t, dir, "init", "-b", "main")
	runGitHelper(t, dir, "config", "user.email", "test@test.com")
	runGitHelper(t, dir, "config", "user.name", "Test")
	runGitHelper(t, dir, "remote", "add", "origin", "http://localhost:19999/nonexistent.git")

	cfg := "repo: http://localhost:19999/nonexistent.git\n"
	if err := os.WriteFile(filepath.Join(dir, "aim.local.yaml"), []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}

	_, _, err := runAimCmd(t, fakeHome, dir, "sync")
	if err == nil {
		t.Fatal("expected error when remote is unreachable, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "reach") {
		t.Errorf("expected 'reach' in error (unreachable remote), got: %v", err)
	}
}

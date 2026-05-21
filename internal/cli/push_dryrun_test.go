package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/axsmak/aim/internal/localconfig"
)

// setupDryRunRepo creates a real git repo with an initial commit and a valid
// aim.local.yaml, ready for push --dry-run testing.
func setupDryRunRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("command %v: %v\n%s", args, err, out)
		}
	}
	run("git", "init", "-b", "main")
	run("git", "config", "user.email", "test@test.com")
	run("git", "config", "user.name", "Test")
	run("git", "commit", "--allow-empty", "-m", "init")

	if err := localconfig.Save(dir, localconfig.Config{}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "skills"), 0755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func runDryPush(t *testing.T, dir string) (stdout string, err error) {
	t.Helper()
	var out, errOut bytes.Buffer
	fake := &fakeGitOps{lsRemoteResult: ""}
	err = runPush(true, dir, fake, &out, &errOut)
	return out.String(), err
}

// TestPushDryRun_untrackedSkill: new untracked skills/wiki.md →
// dry-run shows it as a publishable change with ?? prefix.
func TestPushDryRun_untrackedSkill(t *testing.T) {
	dir := setupDryRunRepo(t)

	content := "---\nname: wiki\ndescription: wiki skill\n---\n\nWiki body."
	if err := os.WriteFile(filepath.Join(dir, "skills", "wiki.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	output, err := runDryPush(t, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(output, "[dry-run] aiman push: would publish managed changes") {
		t.Errorf("expected publish-intent header, got: %q", output)
	}
	if !strings.Contains(output, "skills/wiki.md") {
		t.Errorf("expected skills/wiki.md in porcelain output, got: %q", output)
	}
	if !strings.Contains(output, "??") {
		t.Errorf("expected untracked marker '??' in output, got: %q", output)
	}
	if !strings.Contains(output, "Validated inventory:") {
		t.Errorf("expected inventory summary, got: %q", output)
	}
	if strings.Contains(output, "no changes applied") {
		t.Errorf("must not contain old misleading message, got: %q", output)
	}
}

// TestPushDryRun_clean: clean working tree → dry-run says nothing to publish.
func TestPushDryRun_clean(t *testing.T) {
	dir := setupDryRunRepo(t)

	// Commit a valid skill so validation passes, working tree stays clean.
	content := "---\nname: existing\ndescription: already tracked\n---\n\nBody."
	skillFile := filepath.Join(dir, "skills", "existing.md")
	if err := os.WriteFile(skillFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("command %v: %v\n%s", args, err, out)
		}
	}
	run("git", "add", "skills/existing.md")
	run("git", "commit", "-m", "add existing skill")

	output, err := runDryPush(t, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(output, "nothing to publish — working tree is clean") {
		t.Errorf("expected clean-repo message, got: %q", output)
	}
	if strings.Contains(output, "no changes applied") {
		t.Errorf("must not contain old misleading message, got: %q", output)
	}
	if strings.Contains(output, "would publish managed changes") {
		t.Errorf("must not report changes when tree is clean, got: %q", output)
	}
}

// TestPushDryRun_stagedSkill: skill file staged (git add) →
// dry-run shows it with 'A ' index-added prefix.
func TestPushDryRun_stagedSkill(t *testing.T) {
	dir := setupDryRunRepo(t)

	content := "---\nname: wiki\ndescription: wiki skill\n---\n\nWiki body."
	if err := os.WriteFile(filepath.Join(dir, "skills", "wiki.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("git", "add", "skills/wiki.md")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}

	output, err := runDryPush(t, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(output, "[dry-run] aiman push: would publish managed changes") {
		t.Errorf("expected publish-intent header, got: %q", output)
	}
	if !strings.Contains(output, "skills/wiki.md") {
		t.Errorf("expected skills/wiki.md in porcelain output, got: %q", output)
	}
	// Staged new file shows as 'A ' in porcelain format.
	if !strings.Contains(output, "A ") {
		t.Errorf("expected staged marker 'A ' in output, got: %q", output)
	}
}

package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/axsmak/aim/internal/globalconfig"
)

func TestRunStatus_NotInitialized(t *testing.T) {
	dir := t.TempDir()
	var out, errOut bytes.Buffer
	err := runStatus(t.TempDir(), dir, &fakeGitOps{}, &out, &errOut)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if !strings.Contains(out.String(), "Repository not initialized") {
		t.Errorf("expected not-initialized message, got: %q", out.String())
	}
}

func TestRunStatus_NoDelta(t *testing.T) {
	dir := writeStatusConfig(t, "git@gitlab.com:test/repo.git", "")
	git := &fakeGitOps{
		headHashResult:       "abc1234567890",
		diffNameStatusResult: nil,
		listUntrackedResult:  nil,
	}
	var out, errOut bytes.Buffer
	if err := runStatus(t.TempDir(), dir, git, &out, &errOut); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	assertContains(t, got, "Repository:   git@gitlab.com:test/repo.git")
	assertContains(t, got, "Position:     up-to-date with origin/main")
	assertContains(t, got, "Working tree matches origin/main · nothing to publish")
	if strings.Contains(got, "Changes not yet published") {
		t.Errorf("expected no Changes section, got: %q", got)
	}
}

func TestRunStatus_WithDelta(t *testing.T) {
	dir := writeStatusConfig(t, "git@gitlab.com:test/repo.git", "")
	git := &fakeGitOps{
		headHashResult: "abc1234567890",
		diffNameStatusResult: []string{
			"A\tskills/refactor-helper.md",
			"M\tskills/commit-message.md",
			"D\tmcp/old-server.yaml",
		},
		listUntrackedResult: nil,
	}
	var out, errOut bytes.Buffer
	if err := runStatus(t.TempDir(), dir, git, &out, &errOut); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	assertContains(t, got, "Changes not yet published (origin/main → working tree):")
	assertContains(t, got, "  A skills/refactor-helper.md")
	assertContains(t, got, "  M skills/commit-message.md")
	assertContains(t, got, "  D mcp/old-server.yaml")
	assertContains(t, got, "  run aiman push to publish")
}

func TestRunStatus_WithUntracked(t *testing.T) {
	dir := writeStatusConfig(t, "git@gitlab.com:test/repo.git", "")
	git := &fakeGitOps{
		headHashResult:       "abc1234567890",
		diffNameStatusResult: nil,
		listUntrackedResult:  []string{"skills/new-skill.md"},
	}
	var out, errOut bytes.Buffer
	if err := runStatus(t.TempDir(), dir, git, &out, &errOut); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	assertContains(t, got, "Changes not yet published (origin/main → working tree):")
	assertContains(t, got, "  A skills/new-skill.md")
	assertContains(t, got, "  run aiman push to publish")
}

func TestRunStatus_DeltaTruncated(t *testing.T) {
	dir := writeStatusConfig(t, "git@gitlab.com:test/repo.git", "")

	// Build 57 diff lines to exceed the 20-line threshold.
	diffLines := make([]string, 57)
	for i := range diffLines {
		diffLines[i] = fmt.Sprintf("A\tskills/skill-%02d.md", i+1)
	}
	git := &fakeGitOps{
		headHashResult:       "abc1234567890",
		diffNameStatusResult: diffLines,
		listUntrackedResult:  nil,
	}
	var out, errOut bytes.Buffer
	if err := runStatus(t.TempDir(), dir, git, &out, &errOut); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	assertContains(t, got, "  A skills/skill-01.md")
	assertContains(t, got, "  A skills/skill-20.md")
	if strings.Contains(got, "  A skills/skill-21.md") {
		t.Errorf("line 21 must be truncated, got: %q", got)
	}
	assertContains(t, got, "  … and 37 more")
}

func TestRunStatus_Position_Ahead(t *testing.T) {
	dir := writeStatusConfig(t, "git@gitlab.com:test/repo.git", "")
	git := &fakeGitOps{
		headHashResult: "abc1234567890",
	}
	git.countAheadBehindFn = func(dir, base, ref string) (int, int, error) {
		return 3, 0, nil
	}
	var out, errOut bytes.Buffer
	if err := runStatus(t.TempDir(), dir, git, &out, &errOut); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 4.8 plural, 4.9 text format
	assertContains(t, out.String(), "Position:     3 commits ahead of origin/main")
}

func TestRunStatus_Position_AheadSingular(t *testing.T) {
	dir := writeStatusConfig(t, "git@gitlab.com:test/repo.git", "")
	git := &fakeGitOps{
		headHashResult: "abc1234567890",
	}
	git.countAheadBehindFn = func(dir, base, ref string) (int, int, error) {
		return 1, 0, nil
	}
	var out, errOut bytes.Buffer
	if err := runStatus(t.TempDir(), dir, git, &out, &errOut); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertContains(t, out.String(), "Position:     1 commit ahead of origin/main")
}

func TestRunStatus_Position_Behind(t *testing.T) {
	dir := writeStatusConfig(t, "git@gitlab.com:test/repo.git", "")
	git := &fakeGitOps{
		headHashResult: "abc1234567890",
	}
	git.countAheadBehindFn = func(dir, base, ref string) (int, int, error) {
		return 0, 2, nil
	}
	var out, errOut bytes.Buffer
	if err := runStatus(t.TempDir(), dir, git, &out, &errOut); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 4.9 text format
	assertContains(t, out.String(), "Position:     2 commits behind of origin/main")
}

func TestRunStatus_Position_Behind_Hint(t *testing.T) {
	dir := writeStatusConfig(t, "git@gitlab.com:test/repo.git", "oldhash1234567")
	git := &fakeGitOps{
		headHashResult: "abc1234567890",
	}
	git.countAheadBehindFn = func(dir, base, ref string) (int, int, error) {
		return 0, 1, nil
	}
	var out, errOut bytes.Buffer
	if err := runStatus(t.TempDir(), dir, git, &out, &errOut); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// ADR-0003 4.10: behind → sync hint naming the remote cause.
	got := out.String()
	assertContains(t, got, "behind of origin/main")
	assertContains(t, got, "  run aiman sync to apply remote changes")
}

func TestRunStatus_Position_Diverged_Hint(t *testing.T) {
	dir := writeStatusConfig(t, "git@gitlab.com:test/repo.git", "")
	git := &fakeGitOps{
		headHashResult: "abc1234567890",
	}
	git.countAheadBehindFn = func(dir, base, ref string) (int, int, error) {
		return 1, 1, nil
	}
	var out, errOut bytes.Buffer
	if err := runStatus(t.TempDir(), dir, git, &out, &errOut); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// ADR-0003 4.10: diverged → resolve-with-git hint, distinct from plain behind.
	assertContains(t, out.String(), "  history diverged — resolve with git, then run aiman sync")
}

func TestRunStatus_NeedsSync_Hint(t *testing.T) {
	dir := writeStatusConfig(t, "git@gitlab.com:test/repo.git", "oldhash1234567")
	git := &fakeGitOps{headHashResult: "newhash1234567"}
	git.countAheadBehindFn = func(dir, base, ref string) (int, int, error) {
		if base == "oldhash1234567" {
			return 1, 0, nil
		}
		return 0, 0, nil
	}
	var out, errOut bytes.Buffer
	if err := runStatus(t.TempDir(), dir, git, &out, &errOut); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// ADR-0003 4.10: position up-to-date but environments stale → generic sync hint.
	got := out.String()
	assertContains(t, got, "Working tree matches origin/main · nothing to publish")
	assertContains(t, got, "  run aiman sync to apply")
}

func TestRunStatus_PushAndSyncHints_BothPrinted(t *testing.T) {
	dir := writeStatusConfig(t, "git@gitlab.com:test/repo.git", "oldhash1234567")
	git := &fakeGitOps{
		headHashResult: "newhash1234567",
		diffNameStatusResult: []string{
			"M\tskills/commit-message.md",
		},
	}
	git.countAheadBehindFn = func(dir, base, ref string) (int, int, error) {
		if base == "oldhash1234567" {
			return 1, 0, nil
		}
		return 0, 0, nil
	}
	var out, errOut bytes.Buffer
	if err := runStatus(t.TempDir(), dir, git, &out, &errOut); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// ADR-0003 4.10: both unpublished changes and stale environments → both hints,
	// push before sync, each on its own line.
	got := out.String()
	pushIdx := strings.Index(got, "run aiman push to publish")
	syncIdx := strings.Index(got, "run aiman sync to apply")
	if pushIdx == -1 || syncIdx == -1 {
		t.Fatalf("expected both hints, got: %q", got)
	}
	if pushIdx > syncIdx {
		t.Errorf("expected push hint before sync hint, got: %q", got)
	}
}

func TestRunStatus_Position_Diverged(t *testing.T) {
	dir := writeStatusConfig(t, "git@gitlab.com:test/repo.git", "")
	git := &fakeGitOps{
		headHashResult: "abc1234567890",
	}
	git.countAheadBehindFn = func(dir, base, ref string) (int, int, error) {
		return 1, 1, nil
	}
	var out, errOut bytes.Buffer
	if err := runStatus(t.TempDir(), dir, git, &out, &errOut); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 4.9 diverged format
	assertContains(t, out.String(), "Position:     diverged from origin/main (1 commit ahead, 1 commit behind)")
}

func TestRunStatus_Position_Diverged_Plural(t *testing.T) {
	dir := writeStatusConfig(t, "git@gitlab.com:test/repo.git", "")
	git := &fakeGitOps{
		headHashResult: "abc1234567890",
	}
	git.countAheadBehindFn = func(dir, base, ref string) (int, int, error) {
		return 2, 3, nil
	}
	var out, errOut bytes.Buffer
	if err := runStatus(t.TempDir(), dir, git, &out, &errOut); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertContains(t, out.String(), "Position:     diverged from origin/main (2 commits ahead, 3 commits behind)")
}

func TestRunStatus_Position_Unreachable_FetchFail(t *testing.T) {
	dir := writeStatusConfig(t, "git@gitlab.com:test/repo.git", "")
	git := &fakeGitOps{
		headHashResult: "abc1234567890",
		fetchErr:       errors.New("connection refused"),
	}
	var out, errOut bytes.Buffer
	// exit 0: runStatus must not return error
	if err := runStatus(t.TempDir(), dir, git, &out, &errOut); err != nil {
		t.Fatalf("expected exit 0, got error: %v", err)
	}
	// 4.9 / 5.4: Position text
	assertContains(t, out.String(), "Position:     unknown (remote unreachable)")
	// 4.10 / 5.4: warning in stderr
	assertContains(t, errOut.String(), "warning: cannot reach remote repository")
}

func TestRunStatus_Unreachable_NoSyncHint_EvenWithNeedsSync(t *testing.T) {
	// Code review fix: aiman sync needs the same remote status just failed to
	// reach, so suggesting it here would point to a command that fails too.
	dir := writeStatusConfig(t, "git@gitlab.com:test/repo.git", "oldhash1234567")
	git := &fakeGitOps{
		headHashResult: "newhash1234567",
		fetchErr:       errors.New("connection refused"),
	}
	git.countAheadBehindFn = func(dir, base, ref string) (int, int, error) {
		return 1, 0, nil // needsSync from synced_hash vs HEAD, no remote call
	}
	var out, errOut bytes.Buffer
	if err := runStatus(t.TempDir(), dir, git, &out, &errOut); err != nil {
		t.Fatalf("expected exit 0, got error: %v", err)
	}
	got := out.String()
	assertContains(t, got, "Environments: needs sync (1 commit not applied)")
	if strings.Contains(got, "run aiman sync") {
		t.Errorf("expected no sync hint when remote is unreachable, got: %q", got)
	}
}

func TestRunStatus_Position_Unreachable_CountAheadBehindFail(t *testing.T) {
	dir := writeStatusConfig(t, "git@gitlab.com:test/repo.git", "")
	git := &fakeGitOps{
		headHashResult: "abc1234567890",
	}
	git.countAheadBehindFn = func(dir, base, ref string) (int, int, error) {
		return 0, 0, &fakeErr{"remote unreachable"}
	}
	var out, errOut bytes.Buffer
	if err := runStatus(t.TempDir(), dir, git, &out, &errOut); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertContains(t, out.String(), "Position:     unknown (remote unreachable)")
	assertContains(t, errOut.String(), "warning: cannot reach remote repository")
}

func TestRunStatus_FetchOK_PositionCalculated(t *testing.T) {
	dir := writeStatusConfig(t, "git@gitlab.com:test/repo.git", "")
	git := &fakeGitOps{
		headHashResult: "abc1234567890",
		fetchErr:       nil, // fetch succeeds
	}
	git.countAheadBehindFn = func(dir, base, ref string) (int, int, error) {
		return 0, 1, nil // remote has 1 new commit
	}
	var out, errOut bytes.Buffer
	if err := runStatus(t.TempDir(), dir, git, &out, &errOut); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Position reflects remote state after successful fetch
	assertContains(t, out.String(), "Position:     1 commit behind of origin/main")
	// no warning in stderr
	if errOut.Len() != 0 {
		t.Errorf("expected empty stderr, got: %q", errOut.String())
	}
}

func TestRunStatus_Environments_Applied(t *testing.T) {
	dir := writeStatusConfig(t, "git@gitlab.com:test/repo.git", "abc1234567890abcdef")
	git := &fakeGitOps{headHashResult: "abc1234567890abcdef"}
	var out, errOut bytes.Buffer
	if err := runStatus(t.TempDir(), dir, git, &out, &errOut); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertContains(t, out.String(), "Environments: applied (synced abc1234)")
}

func TestRunStatus_Environments_NeedsSync(t *testing.T) {
	dir := writeStatusConfig(t, "git@gitlab.com:test/repo.git", "oldhash1234567")
	git := &fakeGitOps{headHashResult: "newhash1234567"}
	git.countAheadBehindFn = func(dir, base, ref string) (int, int, error) {
		if base == "oldhash1234567" {
			return 2, 0, nil
		}
		return 0, 0, nil
	}
	var out, errOut bytes.Buffer
	if err := runStatus(t.TempDir(), dir, git, &out, &errOut); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 4.8: plural helper in env status
	assertContains(t, out.String(), "Environments: needs sync (2 commits not applied)")
}

func TestRunStatus_Environments_NeedsSync_Singular(t *testing.T) {
	dir := writeStatusConfig(t, "git@gitlab.com:test/repo.git", "oldhash1234567")
	git := &fakeGitOps{headHashResult: "newhash1234567"}
	git.countAheadBehindFn = func(dir, base, ref string) (int, int, error) {
		if base == "oldhash1234567" {
			return 1, 0, nil
		}
		return 0, 0, nil
	}
	var out, errOut bytes.Buffer
	if err := runStatus(t.TempDir(), dir, git, &out, &errOut); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertContains(t, out.String(), "Environments: needs sync (1 commit not applied)")
}

func TestRunStatus_Environments_Unknown(t *testing.T) {
	dir := writeStatusConfig(t, "git@gitlab.com:test/repo.git", "")
	git := &fakeGitOps{headHashResult: "abc1234567890"}
	var out, errOut bytes.Buffer
	if err := runStatus(t.TempDir(), dir, git, &out, &errOut); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertContains(t, out.String(), "Environments: unknown")
}

func TestRunStatus_Pin_None(t *testing.T) {
	dir := writeStatusConfig(t, "git@gitlab.com:test/repo.git", "")
	homeDir := t.TempDir()
	git := &fakeGitOps{headHashResult: "abc1234567890"}
	var out, errOut bytes.Buffer
	if err := runStatus(homeDir, dir, git, &out, &errOut); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertContains(t, out.String(), "Pinned loadout: none")
}

func TestRunStatus_Pin_Set(t *testing.T) {
	dir := writeStatusConfig(t, "git@gitlab.com:test/repo.git", "")
	homeDir := t.TempDir()
	if err := globalconfig.Save(homeDir, globalconfig.Config{Repo: dir, Loadout: "backend"}); err != nil {
		t.Fatalf("failed to save global config: %v", err)
	}
	git := &fakeGitOps{headHashResult: "abc1234567890"}
	var out, errOut bytes.Buffer
	if err := runStatus(homeDir, dir, git, &out, &errOut); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertContains(t, out.String(), "Pinned loadout: backend")
}

func TestRunStatus_Pin_PrintedEvenWhenRemoteUnreachable(t *testing.T) {
	dir := writeStatusConfig(t, "git@gitlab.com:test/repo.git", "")
	homeDir := t.TempDir()
	if err := globalconfig.Save(homeDir, globalconfig.Config{Repo: dir, Loadout: "backend"}); err != nil {
		t.Fatalf("failed to save global config: %v", err)
	}
	git := &fakeGitOps{
		headHashResult: "abc1234567890",
		fetchErr:       errors.New("connection refused"),
	}
	var out, errOut bytes.Buffer
	if err := runStatus(homeDir, dir, git, &out, &errOut); err != nil {
		t.Fatalf("expected exit 0, got error: %v", err)
	}
	// The pin is local state, unrelated to remote reachability: it must still
	// print alongside the warning, per ADR-0006.
	assertContains(t, out.String(), "Position:     unknown (remote unreachable)")
	assertContains(t, out.String(), "Pinned loadout: backend")
}

func TestRunStatus_Pin_GlobalConfigLoadErrorDoesNotFailStatus(t *testing.T) {
	dir := writeStatusConfig(t, "git@gitlab.com:test/repo.git", "")
	homeDir := t.TempDir()
	// A directory where the global config file should be makes globalconfig.Load
	// fail (it's not "file does not exist"), but status must degrade gracefully.
	configDir := filepath.Join(homeDir, ".config", "aim")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	if err := os.Mkdir(filepath.Join(configDir, "config.yaml"), 0755); err != nil {
		t.Fatalf("failed to create config.yaml as a directory: %v", err)
	}
	git := &fakeGitOps{headHashResult: "abc1234567890"}
	var out, errOut bytes.Buffer
	if err := runStatus(homeDir, dir, git, &out, &errOut); err != nil {
		t.Fatalf("expected status to succeed despite global config load error, got: %v", err)
	}
	assertContains(t, out.String(), "Pinned loadout: none")
}

// helpers

type fakeErr struct{ msg string }

func (e *fakeErr) Error() string { return e.msg }

func writeStatusConfig(t *testing.T, repo, syncedHash string) string {
	t.Helper()
	dir := t.TempDir()
	content := "repo: " + repo + "\n"
	if syncedHash != "" {
		content += "synced_hash: " + syncedHash + "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "aim.local.yaml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Errorf("expected output to contain %q\ngot:\n%s", want, got)
	}
}

package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunStatus_NotInitialized(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	err := runStatus(dir, &fakeGitOps{}, &out)
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
	var out bytes.Buffer
	if err := runStatus(dir, git, &out); err != nil {
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
	var out bytes.Buffer
	if err := runStatus(dir, git, &out); err != nil {
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
	var out bytes.Buffer
	if err := runStatus(dir, git, &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	assertContains(t, got, "Changes not yet published (origin/main → working tree):")
	assertContains(t, got, "  A skills/new-skill.md")
	assertContains(t, got, "  run aiman push to publish")
}

func TestRunStatus_Position_Ahead(t *testing.T) {
	dir := writeStatusConfig(t, "git@gitlab.com:test/repo.git", "")
	git := &fakeGitOps{
		headHashResult: "abc1234567890",
	}
	git.countAheadBehindFn = func(dir, base, ref string) (int, int, error) {
		return 3, 0, nil
	}
	var out bytes.Buffer
	if err := runStatus(dir, git, &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertContains(t, out.String(), "Position:     ahead 3 commit(s)")
}

func TestRunStatus_Position_Behind(t *testing.T) {
	dir := writeStatusConfig(t, "git@gitlab.com:test/repo.git", "")
	git := &fakeGitOps{
		headHashResult: "abc1234567890",
	}
	git.countAheadBehindFn = func(dir, base, ref string) (int, int, error) {
		return 0, 2, nil
	}
	var out bytes.Buffer
	if err := runStatus(dir, git, &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertContains(t, out.String(), "Position:     behind 2 commit(s)")
}

func TestRunStatus_Position_Diverged(t *testing.T) {
	dir := writeStatusConfig(t, "git@gitlab.com:test/repo.git", "")
	git := &fakeGitOps{
		headHashResult: "abc1234567890",
	}
	git.countAheadBehindFn = func(dir, base, ref string) (int, int, error) {
		return 1, 1, nil
	}
	var out bytes.Buffer
	if err := runStatus(dir, git, &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertContains(t, out.String(), "Position:     diverged")
}

func TestRunStatus_Position_Unreachable(t *testing.T) {
	dir := writeStatusConfig(t, "git@gitlab.com:test/repo.git", "")
	git := &fakeGitOps{
		headHashResult: "abc1234567890",
	}
	git.countAheadBehindFn = func(dir, base, ref string) (int, int, error) {
		return 0, 0, &fakeErr{"remote unreachable"}
	}
	var out bytes.Buffer
	if err := runStatus(dir, git, &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertContains(t, out.String(), "Position:     unreachable")
}

func TestRunStatus_Environments_Applied(t *testing.T) {
	dir := writeStatusConfig(t, "git@gitlab.com:test/repo.git", "abc1234567890abcdef")
	git := &fakeGitOps{headHashResult: "abc1234567890abcdef"}
	var out bytes.Buffer
	if err := runStatus(dir, git, &out); err != nil {
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
	var out bytes.Buffer
	if err := runStatus(dir, git, &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertContains(t, out.String(), "Environments: needs sync (2 commit(s) not applied)")
}

func TestRunStatus_Environments_Unknown(t *testing.T) {
	dir := writeStatusConfig(t, "git@gitlab.com:test/repo.git", "")
	git := &fakeGitOps{headHashResult: "abc1234567890"}
	var out bytes.Buffer
	if err := runStatus(dir, git, &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertContains(t, out.String(), "Environments: unknown")
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

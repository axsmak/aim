package cli

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/axsmak/aim/internal/localconfig"
)

func initPushGitRepo(t *testing.T, dir string) {
	t.Helper()
	run := func(args ...string) {
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
}

func setupPushDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	// ManagedStatus calls git status, so a real repo is required.
	initPushGitRepo(t, dir)
	if err := localconfig.Save(dir, localconfig.Config{}); err != nil {
		t.Fatal(err)
	}
	skillsDir := filepath.Join(dir, "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func writeValidSkill(t *testing.T, dir, name string) {
	t.Helper()
	content := "---\nname: " + name + "\ndescription: a test skill\n---\n\nSkill body."
	path := filepath.Join(dir, "skills", name+".md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestRunPush_noGitDir(t *testing.T) {
	dir := t.TempDir()
	var out, errOut bytes.Buffer
	err := runPush(false, dir, &fakeGitOps{}, &out, &errOut)
	if err == nil {
		t.Fatal("expected error for missing .git, got nil")
	}
	if !strings.Contains(err.Error(), "not initialized") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunPush_remoteAhead(t *testing.T) {
	dir := setupPushDir(t)
	// published_hash is empty but remote has a commit → remote is ahead
	fake := &fakeGitOps{lsRemoteResult: "deadbeef1234567890123456789012345678abcd"}
	var out, errOut bytes.Buffer
	err := runPush(false, dir, fake, &out, &errOut)
	if err == nil {
		t.Fatal("expected error when remote is ahead")
	}
	if !strings.Contains(err.Error(), "remote is ahead") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunPush_nothingToCommit(t *testing.T) {
	dir := setupPushDir(t)
	writeValidSkill(t, dir, "example")
	fake := &fakeGitOps{
		lsRemoteResult: "", // empty repo, no block
		commitErr:      errors.New("nothing to commit, working tree clean"),
	}
	var out, errOut bytes.Buffer
	err := runPush(false, dir, fake, &out, &errOut)
	if err == nil {
		t.Fatal("expected error for nothing to commit")
	}
	if !strings.Contains(err.Error(), "nothing to publish") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunPush_dryRun(t *testing.T) {
	dir := setupPushDir(t)
	writeValidSkill(t, dir, "example")
	fake := &fakeGitOps{lsRemoteResult: ""}
	var out, errOut bytes.Buffer
	err := runPush(true, dir, fake, &out, &errOut)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "[dry-run]") {
		t.Errorf("expected dry-run in output, got: %q", output)
	}
	if !strings.Contains(output, "example") {
		t.Errorf("expected skill name in output, got: %q", output)
	}
	if fake.commitCalled {
		t.Error("Commit should not be called in dry-run mode")
	}
	if fake.pushCalled {
		t.Error("Push should not be called in dry-run mode")
	}
}

func TestRunPush_success(t *testing.T) {
	dir := setupPushDir(t)
	writeValidSkill(t, dir, "example")
	fake := &fakeGitOps{
		lsRemoteResult: "",
		headHashResult: "abc1234567890123456789012345678901234abc0",
	}
	var out, errOut bytes.Buffer
	err := runPush(false, dir, fake, &out, &errOut)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fake.commitCalled {
		t.Error("expected Commit to be called")
	}
	if !fake.pushCalled {
		t.Error("expected Push to be called")
	}
	output := out.String()
	if !strings.Contains(output, "published:") {
		t.Errorf("expected published message, got: %q", output)
	}
	// published_hash should be saved
	cfg, err := localconfig.Load(dir)
	if err != nil {
		t.Fatalf("cannot load config: %v", err)
	}
	if cfg.PublishedHash == "" {
		t.Error("expected published_hash to be saved")
	}
}

func TestRunPush_pushFails_rollsBack(t *testing.T) {
	dir := setupPushDir(t)
	writeValidSkill(t, dir, "example")
	fake := &fakeGitOps{
		lsRemoteResult: "",
		pushErr:        errors.New("remote rejected"),
	}
	var out, errOut bytes.Buffer
	err := runPush(false, dir, fake, &out, &errOut)
	if err == nil {
		t.Fatal("expected error on push failure")
	}
	if !strings.Contains(err.Error(), "push failed") {
		t.Errorf("unexpected error: %v", err)
	}
	if !fake.resetSoftCalled {
		t.Error("expected ResetSoft to be called on push failure")
	}
}

func writeValidMCP(t *testing.T, dir, name string) {
	t.Helper()
	content := "name: " + name + "\ndescription: a test MCP server\ncommand: npx\nargs:\n  - -y\n  - " + name + "\ntargets:\n  - claude\nenv: []\n"
	mcpDir := filepath.Join(dir, "mcp")
	if err := os.MkdirAll(mcpDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mcpDir, name+".yaml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func writeInvalidMCP(t *testing.T, dir, name string) {
	t.Helper()
	// Missing required fields (name, command, targets)
	content := "description: broken\n"
	mcpDir := filepath.Join(dir, "mcp")
	if err := os.MkdirAll(mcpDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mcpDir, name+".yaml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestRunPush_invalidMCP_blocksPublish(t *testing.T) {
	dir := setupPushDir(t)
	writeValidSkill(t, dir, "example")
	writeInvalidMCP(t, dir, "broken-server")
	fake := &fakeGitOps{lsRemoteResult: ""}
	var out, errOut bytes.Buffer
	err := runPush(false, dir, fake, &out, &errOut)
	if err == nil {
		t.Fatal("expected error for invalid MCP, got nil")
	}
	if !strings.Contains(err.Error(), "validation failed") {
		t.Errorf("unexpected error: %v", err)
	}
	if fake.commitCalled {
		t.Error("Commit should not be called when MCP validation fails")
	}
	errOutput := errOut.String()
	if !strings.Contains(errOutput, "error:") {
		t.Errorf("expected error line in errOut, got: %q", errOutput)
	}
}

func TestRunPush_aimLocalYamlStaged_blocksPublish(t *testing.T) {
	dir := setupPushDir(t)
	writeValidSkill(t, dir, "example")
	fake := &fakeGitOps{
		lsRemoteResult:     "",
		isFileStagedResult: map[string]bool{"aim.local.yaml": true},
	}
	var out, errOut bytes.Buffer
	err := runPush(false, dir, fake, &out, &errOut)
	if err == nil {
		t.Fatal("expected error when aim.local.yaml is staged")
	}
	if !strings.Contains(err.Error(), "aim.local.yaml") {
		t.Errorf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "git reset HEAD") {
		t.Errorf("expected git reset hint in error, got: %v", err)
	}
	if fake.commitCalled {
		t.Error("Commit should not be called when aim.local.yaml is staged")
	}
}

func TestRunPush_dryRun_withMCP(t *testing.T) {
	dir := setupPushDir(t)
	writeValidSkill(t, dir, "example")
	writeValidMCP(t, dir, "my-server")
	fake := &fakeGitOps{lsRemoteResult: ""}
	var out, errOut bytes.Buffer
	err := runPush(true, dir, fake, &out, &errOut)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "[dry-run]") {
		t.Errorf("expected dry-run in output, got: %q", output)
	}
	if !strings.Contains(output, "MCP servers") {
		t.Errorf("expected 'MCP servers' in output, got: %q", output)
	}
}

func TestRunPush_success_withMCP(t *testing.T) {
	dir := setupPushDir(t)
	writeValidSkill(t, dir, "example")
	writeValidMCP(t, dir, "my-server")
	fake := &fakeGitOps{
		lsRemoteResult: "",
		headHashResult: "abc1234567890123456789012345678901234abc0",
	}
	var out, errOut bytes.Buffer
	err := runPush(false, dir, fake, &out, &errOut)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "MCP server") {
		t.Errorf("expected MCP count in output, got: %q", output)
	}
	if !strings.Contains(output, "skill") {
		t.Errorf("expected skill count in output, got: %q", output)
	}
}

// TestRunPush_dryRun_emptyMCP covers AC: dry-run with empty mcp/ → stdout does NOT contain "MCP servers:"
func TestRunPush_dryRun_emptyMCP(t *testing.T) {
	dir := setupPushDir(t)
	writeValidSkill(t, dir, "example")
	// No MCP files written — mcp/ directory is absent
	fake := &fakeGitOps{lsRemoteResult: ""}
	var out, errOut bytes.Buffer
	err := runPush(true, dir, fake, &out, &errOut)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "[dry-run]") {
		t.Errorf("expected dry-run marker in output, got: %q", output)
	}
	if strings.Contains(output, "MCP servers:") {
		t.Errorf("expected NO 'MCP servers:' in output when mcp/ is empty, got: %q", output)
	}
}

// TestRunPush_success_withoutMCP covers AC: success output without MCP uses old format (no MCP count)
func TestRunPush_success_withoutMCP(t *testing.T) {
	dir := setupPushDir(t)
	writeValidSkill(t, dir, "example")
	// No MCP files — mcp/ directory is absent
	fake := &fakeGitOps{
		lsRemoteResult: "",
		headHashResult: "abc1234567890123456789012345678901234abc0",
	}
	var out, errOut bytes.Buffer
	err := runPush(false, dir, fake, &out, &errOut)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "published:") {
		t.Errorf("expected 'published:' in output, got: %q", output)
	}
	if strings.Contains(output, "MCP servers") {
		t.Errorf("expected NO MCP count in output when no MCP servers, got: %q", output)
	}
}

// --- Issue #65: allow push after sync when published_hash is stale ---

func TestRunPush_afterSync_allowsPush(t *testing.T) {
	dir := setupPushDir(t)
	writeValidSkill(t, dir, "example")
	remoteHash := "deadbeef1234567890123456789012345678abcd"
	// guard uses IsAncestor, not stored hashes — set isAncestorResult: true to allow push
	if err := localconfig.Save(dir, localconfig.Config{SyncedHash: remoteHash}); err != nil {
		t.Fatal(err)
	}
	fake := &fakeGitOps{
		lsRemoteResult:   remoteHash,
		headHashResult:   "newcommit1234567890123456789012345678abc0",
		isAncestorResult: true, // remoteHash is reachable from HEAD after sync
	}
	var out, errOut bytes.Buffer
	err := runPush(false, dir, fake, &out, &errOut)
	if err != nil {
		t.Fatalf("expected push to succeed after sync, got: %v", err)
	}
	if !strings.Contains(out.String(), "published:") {
		t.Errorf("expected published message, got: %q", out.String())
	}
}

func TestRunPush_afterSync_remoteAdvanced_blocks(t *testing.T) {
	dir := setupPushDir(t)
	writeValidSkill(t, dir, "example")
	syncedHash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	remoteHash := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	// remote moved ahead after our sync — must block
	if err := localconfig.Save(dir, localconfig.Config{SyncedHash: syncedHash}); err != nil {
		t.Fatal(err)
	}
	fake := &fakeGitOps{lsRemoteResult: remoteHash}
	var out, errOut bytes.Buffer
	err := runPush(false, dir, fake, &out, &errOut)
	if err == nil {
		t.Fatal("expected block when remote advanced after sync")
	}
	if !strings.Contains(err.Error(), "remote is ahead") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- Issue #60: guard for missing skills/ ---

func TestRunPush_SkillsDirMissing(t *testing.T) {
	dir := t.TempDir()
	// .git and aim.local.yaml exist but no skills/
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := localconfig.Save(dir, localconfig.Config{}); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	err := runPush(false, dir, &fakeGitOps{lsRemoteResult: ""}, &out, &errOut)
	if err == nil {
		t.Fatal("expected error for missing skills/, got nil")
	}
	if !strings.Contains(err.Error(), "malformed repository: skills/ directory is missing") {
		t.Errorf("unexpected error: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "run aiman init or restore scaffold") {
		t.Errorf("expected hint in error, got: %q", err.Error())
	}
}

// --- Issue #74 / #76: push guard uses git IsAncestor, not stored hashes ---

// TestRunPush_AfterInitExisting_NoStoredHashes reproduces the root cause of issue #74:
// aiman init (existing) creates aim.local.yaml without synced_hash or published_hash.
// Push must succeed if remoteHash is reachable from HEAD (just cloned = identical state).
func TestRunPush_AfterInitExisting_NoStoredHashes(t *testing.T) {
	dir := setupPushDir(t)
	writeValidSkill(t, dir, "example")
	remoteHash := "deadbeef1234567890123456789012345678abcd"
	// aim.local.yaml has no stored hashes (as written by aim init existing)
	if err := localconfig.Save(dir, localconfig.Config{}); err != nil {
		t.Fatal(err)
	}
	fake := &fakeGitOps{
		lsRemoteResult:   remoteHash,
		headHashResult:   remoteHash, // local HEAD == remote HEAD (just cloned)
		isAncestorResult: true,       // remoteHash IS reachable from HEAD
	}
	var out, errOut bytes.Buffer
	err := runPush(false, dir, fake, &out, &errOut)
	if err != nil {
		t.Fatalf("expected push to succeed after init existing (no stored hashes), got: %v", err)
	}
	if !strings.Contains(out.String(), "published:") {
		t.Errorf("expected published message, got: %q", out.String())
	}
}

// TestRunPush_RemoteAhead_BlockedByIsAncestor verifies that push is blocked when
// remoteHash is NOT reachable from HEAD (remote has commits local doesn't have).
func TestRunPush_RemoteAhead_BlockedByIsAncestor(t *testing.T) {
	dir := setupPushDir(t)
	writeValidSkill(t, dir, "example")
	remoteHash := "newremotehash12345678901234567890123456ab"
	fake := &fakeGitOps{
		lsRemoteResult:   remoteHash,
		isAncestorResult: false, // remoteHash NOT reachable from HEAD → remote is ahead
	}
	var out, errOut bytes.Buffer
	err := runPush(false, dir, fake, &out, &errOut)
	if err == nil {
		t.Fatal("expected error when remote is ahead (IsAncestor=false)")
	}
	if !strings.Contains(err.Error(), "remote is ahead of local") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestRunPush_IsAncestorError_ReturnsError verifies that an IsAncestor git error
// surfaces as a push error rather than being silently ignored.
func TestRunPush_IsAncestorError_ReturnsError(t *testing.T) {
	dir := setupPushDir(t)
	writeValidSkill(t, dir, "example")
	fake := &fakeGitOps{
		lsRemoteResult: "somehash1234567890123456789012345678abcd",
		isAncestorErr:  errors.New("git broken"),
	}
	var out, errOut bytes.Buffer
	err := runPush(false, dir, fake, &out, &errOut)
	if err == nil {
		t.Fatal("expected error when IsAncestor fails")
	}
	if !strings.Contains(err.Error(), "cannot verify remote state") {
		t.Errorf("unexpected error: %v", err)
	}
}

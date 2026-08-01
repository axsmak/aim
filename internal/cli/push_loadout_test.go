package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- Issue #152: push validates loadouts/ (BFT 5.2) ---

func writeLoadout(t *testing.T, dir, fileName, content string) {
	t.Helper()
	loDir := filepath.Join(dir, "loadouts")
	if err := os.MkdirAll(loDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(loDir, fileName), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func writeFolderSkill(t *testing.T, dir, name string) {
	t.Helper()
	skillDir := filepath.Join(dir, "skills", name)
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: a folder skill\n---\n\nSkill body."
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// assertValidationBlocked checks the shared outcome of every error row of the
// BFT 5.2 matrix: push fails, nothing is committed, errOut carries the finding.
func assertValidationBlocked(t *testing.T, err error, fake *fakeGitOps, errOut *bytes.Buffer, wantInErr string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(err.Error(), "validation failed") {
		t.Errorf("unexpected error: %v", err)
	}
	if fake.commitCalled {
		t.Error("Commit should not be called when loadout validation fails")
	}
	if fake.pushCalled {
		t.Error("Push should not be called when loadout validation fails")
	}
	if !strings.Contains(errOut.String(), wantInErr) {
		t.Errorf("expected %q in errOut, got: %q", wantInErr, errOut.String())
	}
}

func TestRunPush_loadoutUnparseable_blocks(t *testing.T) {
	dir := setupPushDir(t)
	writeValidSkill(t, dir, "example")
	writeLoadout(t, dir, "broken.yaml", "name: [unclosed\n\titems:\n")
	fake := &fakeGitOps{lsRemoteResult: ""}
	var out, errOut bytes.Buffer
	err := runPush(false, dir, fake, &out, &errOut)
	assertValidationBlocked(t, err, fake, &errOut, "error:")
	if !strings.Contains(errOut.String(), "yaml") {
		t.Errorf("expected yaml parse finding, got: %q", errOut.String())
	}
}

func TestRunPush_loadoutMissingName_blocks(t *testing.T) {
	dir := setupPushDir(t)
	writeValidSkill(t, dir, "example")
	writeLoadout(t, dir, "no-name.yaml", "description: d\nitems:\n  - skill:example\n")
	fake := &fakeGitOps{lsRemoteResult: ""}
	var out, errOut bytes.Buffer
	err := runPush(false, dir, fake, &out, &errOut)
	assertValidationBlocked(t, err, fake, &errOut, "name: required")
}

func TestRunPush_loadoutEmptyItems_blocks(t *testing.T) {
	dir := setupPushDir(t)
	writeValidSkill(t, dir, "example")
	writeLoadout(t, dir, "empty.yaml", "name: empty\ndescription: d\nitems: []\n")
	fake := &fakeGitOps{lsRemoteResult: ""}
	var out, errOut bytes.Buffer
	err := runPush(false, dir, fake, &out, &errOut)
	assertValidationBlocked(t, err, fake, &errOut, "items: cannot be empty")
}

func TestRunPush_loadoutUnknownSkill_blocksWithHint(t *testing.T) {
	dir := setupPushDir(t)
	writeValidSkill(t, dir, "example")
	writeLoadout(t, dir, "documentation-work.yaml",
		"name: Documentation Work\ndescription: d\nitems:\n  - skill:missing-skill\n")
	fake := &fakeGitOps{lsRemoteResult: ""}
	var out, errOut bytes.Buffer
	err := runPush(false, dir, fake, &out, &errOut)
	assertValidationBlocked(t, err, fake, &errOut,
		`error: loadout "Documentation Work" references unknown skill "missing-skill"`)
	if !strings.Contains(errOut.String(), "hint: check loadouts/documentation-work.yaml → items") {
		t.Errorf("expected hint line, got: %q", errOut.String())
	}
}

func TestRunPush_loadoutUnknownMCP_blocksWithHint(t *testing.T) {
	dir := setupPushDir(t)
	writeValidSkill(t, dir, "example")
	writeValidMCP(t, dir, "real-server")
	writeLoadout(t, dir, "ops.yaml",
		"name: ops\ndescription: d\nitems:\n  - mcp:ghost-server\n")
	fake := &fakeGitOps{lsRemoteResult: ""}
	var out, errOut bytes.Buffer
	err := runPush(false, dir, fake, &out, &errOut)
	assertValidationBlocked(t, err, fake, &errOut,
		`error: loadout "ops" references unknown mcp "ghost-server"`)
	if !strings.Contains(errOut.String(), "hint: check loadouts/ops.yaml → items") {
		t.Errorf("expected hint line, got: %q", errOut.String())
	}
}

func TestRunPush_loadoutAllErrorsReported(t *testing.T) {
	dir := setupPushDir(t)
	writeValidSkill(t, dir, "example")
	// Two broken refs in one loadout plus a second invalid loadout file:
	// every finding must be reported at once (US-L04).
	writeLoadout(t, dir, "dev.yaml",
		"name: dev\ndescription: d\nitems:\n  - skill:ghost-one\n  - mcp:ghost-two\n")
	writeLoadout(t, dir, "no-items.yaml", "name: no-items\ndescription: d\n")
	fake := &fakeGitOps{lsRemoteResult: ""}
	var out, errOut bytes.Buffer
	err := runPush(false, dir, fake, &out, &errOut)
	assertValidationBlocked(t, err, fake, &errOut, "items: cannot be empty")
	errOutput := errOut.String()
	for _, want := range []string{
		`references unknown skill "ghost-one"`,
		`references unknown mcp "ghost-two"`,
	} {
		if !strings.Contains(errOutput, want) {
			t.Errorf("expected %q in errOut (all errors at once), got: %q", want, errOutput)
		}
	}
}

func TestRunPush_loadoutMissingDescription_warnsOnly(t *testing.T) {
	dir := setupPushDir(t)
	writeValidSkill(t, dir, "example")
	writeLoadout(t, dir, "dev.yaml", "name: dev\nitems:\n  - skill:example\n")
	fake := &fakeGitOps{
		lsRemoteResult: "",
		headHashResult: "abc1234567890123456789012345678901234abc0",
	}
	var out, errOut bytes.Buffer
	err := runPush(false, dir, fake, &out, &errOut)
	if err != nil {
		t.Fatalf("warning must not block push, got: %v", err)
	}
	if !strings.Contains(errOut.String(), "warning:") || !strings.Contains(errOut.String(), "description") {
		t.Errorf("expected description warning, got: %q", errOut.String())
	}
	if !fake.pushCalled {
		t.Error("expected Push to be called despite warning")
	}
}

func TestRunPush_loadoutFileNameMismatch_warnsOnly(t *testing.T) {
	dir := setupPushDir(t)
	writeValidSkill(t, dir, "example")
	writeLoadout(t, dir, "other-file.yaml",
		"name: Documentation Work\ndescription: d\nitems:\n  - skill:example\n")
	fake := &fakeGitOps{
		lsRemoteResult: "",
		headHashResult: "abc1234567890123456789012345678901234abc0",
	}
	var out, errOut bytes.Buffer
	err := runPush(false, dir, fake, &out, &errOut)
	if err != nil {
		t.Fatalf("warning must not block push, got: %v", err)
	}
	if !strings.Contains(errOut.String(), "does not match normalized name") {
		t.Errorf("expected file-name mismatch warning, got: %q", errOut.String())
	}
	if !fake.pushCalled {
		t.Error("expected Push to be called despite warning")
	}
}

func TestRunPush_validLoadout_flatAndFolderSkills_succeeds(t *testing.T) {
	dir := setupPushDir(t)
	writeValidSkill(t, dir, "flat-skill")
	writeFolderSkill(t, dir, "folder-skill")
	writeValidMCP(t, dir, "my-server")
	writeLoadout(t, dir, "dev.yaml",
		"name: dev\ndescription: d\nitems:\n  - skill:flat-skill\n  - skill:folder-skill\n  - mcp:my-server\n")
	fake := &fakeGitOps{
		lsRemoteResult: "",
		headHashResult: "abc1234567890123456789012345678901234abc0",
	}
	var out, errOut bytes.Buffer
	err := runPush(false, dir, fake, &out, &errOut)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(errOut.String(), "error:") {
		t.Errorf("expected no errors, got: %q", errOut.String())
	}
	if !fake.pushCalled {
		t.Error("expected Push to be called")
	}
}

func TestRunPush_noLoadoutsDir_behavesAsBefore(t *testing.T) {
	dir := setupPushDir(t)
	writeValidSkill(t, dir, "example")
	// No loadouts/ directory at all — validation is silently skipped.
	fake := &fakeGitOps{
		lsRemoteResult: "",
		headHashResult: "abc1234567890123456789012345678901234abc0",
	}
	var out, errOut bytes.Buffer
	err := runPush(false, dir, fake, &out, &errOut)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(errOut.String(), "loadout") {
		t.Errorf("expected no loadout output without loadouts/, got: %q", errOut.String())
	}
	if !strings.Contains(out.String(), "published:") {
		t.Errorf("expected published message, got: %q", out.String())
	}
}

func TestRunPush_dryRun_validatesLoadouts(t *testing.T) {
	dir := setupPushDir(t)
	writeValidSkill(t, dir, "example")
	writeLoadout(t, dir, "dev.yaml",
		"name: dev\ndescription: d\nitems:\n  - skill:missing-skill\n")
	fake := &fakeGitOps{lsRemoteResult: ""}
	var out, errOut bytes.Buffer
	err := runPush(true, dir, fake, &out, &errOut)
	assertValidationBlocked(t, err, fake, &errOut,
		`error: loadout "dev" references unknown skill "missing-skill"`)
	// Validation happens before any dry-run reporting.
	if strings.Contains(out.String(), "[dry-run]") {
		t.Errorf("expected no dry-run output on validation failure, got: %q", out.String())
	}
}

func TestRunPush_dryRun_validLoadout_succeeds(t *testing.T) {
	dir := setupPushDir(t)
	writeValidSkill(t, dir, "example")
	writeLoadout(t, dir, "dev.yaml",
		"name: dev\ndescription: d\nitems:\n  - skill:example\n")
	fake := &fakeGitOps{lsRemoteResult: ""}
	var out, errOut bytes.Buffer
	err := runPush(true, dir, fake, &out, &errOut)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "[dry-run]") {
		t.Errorf("expected dry-run output, got: %q", out.String())
	}
}

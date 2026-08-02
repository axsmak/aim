package cli_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- Issue #151: apply --loadout CLI surface (ADR-0004, BFT 5.1) ---
//
// Helpers reused across the cli_test package:
//   runApplyCmd, setupApplyWorkDir       — apply_test.go
//   writeInventorySkill, installEnvSkill,
//   reconcileSkillContent, writeInventoryMCP,
//   writeMCPServersJSON, mcpServerKeys    — reconcile_test.go

// writeApplyLoadout writes loadouts/<fileName> in dir (the internal-test
// twin writeLoadout in push_loadout_test.go is not visible from cli_test).
func writeApplyLoadout(t *testing.T, dir, fileName, content string) {
	t.Helper()
	loDir := filepath.Join(dir, "loadouts")
	if err := os.MkdirAll(loDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(loDir, fileName), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// setupLoadoutWorkDir extends setupApplyWorkDir with an mcp/ directory so
// reconcile inventory loading sees the full working-tree layout.
func setupLoadoutWorkDir(t *testing.T, fakeHome string) (workDir string) {
	t.Helper()
	workDir = setupApplyWorkDir(t, fakeHome)
	if err := os.MkdirAll(filepath.Join(workDir, "mcp"), 0755); err != nil {
		t.Fatalf("mkdir mcp: %v", err)
	}
	return workDir
}

// setupAMDScenario builds the canonical A/M/D fixture: loadout "Test" wants
// keep+update+add; env has keep (identical), update (stale), extra (inventory
// item outside the loadout).
func setupAMDScenario(t *testing.T, fakeHome, workDir string) {
	t.Helper()
	claudeDir := filepath.Join(fakeHome, ".claude")

	writeInventorySkill(t, workDir, "keep", "Keep body.")
	writeInventorySkill(t, workDir, "update", "New body.")
	writeInventorySkill(t, workDir, "add", "Add body.")
	writeInventorySkill(t, workDir, "extra", "Extra body.")

	installEnvSkill(t, claudeDir, "keep", reconcileSkillContent("keep", "Keep body."))
	installEnvSkill(t, claudeDir, "update", reconcileSkillContent("update", "Old body."))
	installEnvSkill(t, claudeDir, "extra", reconcileSkillContent("extra", "Extra body."))

	writeApplyLoadout(t, workDir, "test.yaml",
		"name: Test\ndescription: d\nitems:\n  - skill:keep\n  - skill:update\n  - skill:add\n")
}

func TestApplyLoadout_HappyPath_AMD(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupLoadoutWorkDir(t, fakeHome)
	setupAMDScenario(t, fakeHome, workDir)
	claudeDir := filepath.Join(fakeHome, ".claude")

	stdout, _, err := runApplyCmd(t, fakeHome, workDir, "--loadout", "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Success line: loadout named, counters = operation volume (3 desired skills).
	if !strings.Contains(stdout, `applied loadout "Test": 3 skills → 1 environment`) {
		t.Errorf("expected loadout success line, got: %q", stdout)
	}
	// Delta block: composition A/M/D with real-run qualifiers.
	for _, want := range []string{
		"  A skills/add.md   (new in all environments)",
		"  M skills/update.md   (updated in all environments)",
		"  D skills/extra.md   (removed from all environments)",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("expected %q in delta block, got: %q", want, stdout)
		}
	}

	// Environment reconciled exactly to the loadout set.
	if _, err := os.Stat(filepath.Join(claudeDir, "skills", "add", "SKILL.md")); err != nil {
		t.Errorf("add not installed: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(claudeDir, "skills", "update", "SKILL.md"))
	if err != nil || string(got) != reconcileSkillContent("update", "New body.") {
		t.Errorf("update not refreshed: err=%v content=%q", err, got)
	}
	if _, err := os.Stat(filepath.Join(claudeDir, "skills", "extra")); !os.IsNotExist(err) {
		t.Error("extra must be removed from env")
	}
	if _, err := os.Stat(filepath.Join(claudeDir, "skills", "keep", "SKILL.md")); err != nil {
		t.Error("keep must remain installed")
	}
}

func TestApplyLoadout_NameNormalization(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupLoadoutWorkDir(t, fakeHome)
	writeInventorySkill(t, workDir, "solo", "Solo body.")
	writeApplyLoadout(t, workDir, "documentation-work.yaml",
		"name: Documentation Work\ndescription: d\nitems:\n  - skill:solo\n")

	// Human-readable name resolves via normalization (spaces → hyphens, lowercase).
	stdout, _, err := runApplyCmd(t, fakeHome, workDir, "--loadout", "Documentation Work")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, `applied loadout "Documentation Work"`) {
		t.Errorf("expected success line with resolved name, got: %q", stdout)
	}
	if _, err := os.Stat(filepath.Join(fakeHome, ".claude", "skills", "solo", "SKILL.md")); err != nil {
		t.Errorf("solo not installed: %v", err)
	}
}

func TestApplyLoadout_EmptyPlan_SuccessLineWithoutBlock(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupLoadoutWorkDir(t, fakeHome)
	writeInventorySkill(t, workDir, "solo", "Solo body.")
	installEnvSkill(t, filepath.Join(fakeHome, ".claude"), "solo",
		reconcileSkillContent("solo", "Solo body."))
	writeApplyLoadout(t, workDir, "idem.yaml", "name: idem\ndescription: d\nitems:\n  - skill:solo\n")

	stdout, _, err := runApplyCmd(t, fakeHome, workDir, "--loadout", "idem")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, `applied loadout "idem": 1 skill → 1 environment`) {
		t.Errorf("expected success line, got: %q", stdout)
	}
	for _, marker := range []string{"  A ", "  M ", "  D "} {
		if strings.Contains(stdout, marker) {
			t.Errorf("empty plan must not print a delta block, got: %q", stdout)
		}
	}
}

func TestApplyLoadout_DryRun_FullPlanNoChanges(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupLoadoutWorkDir(t, fakeHome)
	setupAMDScenario(t, fakeHome, workDir)
	claudeDir := filepath.Join(fakeHome, ".claude")

	cfgBefore, err := os.ReadFile(filepath.Join(workDir, "aim.local.yaml"))
	if err != nil {
		t.Fatalf("read aim.local.yaml: %v", err)
	}

	stdout, _, err := runApplyCmd(t, fakeHome, workDir, "--loadout", "test", "--dry-run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(stdout, `[dry-run] would apply loadout "Test" — 3 changes to 1 environment (claude-code):`) {
		t.Errorf("expected dry-run header, got: %q", stdout)
	}
	for _, want := range []string{
		"  A skills/add.md   (new in all environments)",
		"  M skills/update.md   (differs in all environments)",
		"  D skills/extra.md   (would remove from all environments)",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("expected %q in dry-run plan, got: %q", want, stdout)
		}
	}
	// Dry-run must not use real-run qualifiers.
	if strings.Contains(stdout, "updated in") || strings.Contains(stdout, "(removed from") {
		t.Errorf("dry-run must not use real-run qualifiers, got: %q", stdout)
	}

	// No changes in the environment.
	if _, err := os.Stat(filepath.Join(claudeDir, "skills", "add")); !os.IsNotExist(err) {
		t.Error("dry-run must not install skills")
	}
	if _, err := os.Stat(filepath.Join(claudeDir, "skills", "extra", "SKILL.md")); err != nil {
		t.Error("dry-run must not remove skills")
	}
	got, err := os.ReadFile(filepath.Join(claudeDir, "skills", "update", "SKILL.md"))
	if err != nil || string(got) != reconcileSkillContent("update", "Old body.") {
		t.Errorf("dry-run must not refresh skills: err=%v content=%q", err, got)
	}

	// No config save on the dry-run path.
	cfgAfter, err := os.ReadFile(filepath.Join(workDir, "aim.local.yaml"))
	if err != nil {
		t.Fatalf("read aim.local.yaml after dry-run: %v", err)
	}
	if string(cfgBefore) != string(cfgAfter) {
		t.Errorf("aim.local.yaml changed during dry-run:\nbefore: %q\nafter:  %q", cfgBefore, cfgAfter)
	}
}

func TestApplyLoadout_DryRun_EmptyPlan(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupLoadoutWorkDir(t, fakeHome)
	writeInventorySkill(t, workDir, "solo", "Solo body.")
	installEnvSkill(t, filepath.Join(fakeHome, ".claude"), "solo",
		reconcileSkillContent("solo", "Solo body."))
	writeApplyLoadout(t, workDir, "idem.yaml", "name: idem\ndescription: d\nitems:\n  - skill:solo\n")

	stdout, _, err := runApplyCmd(t, fakeHome, workDir, "--loadout", "idem", "--dry-run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, `[dry-run] nothing to apply — environments match loadout "idem"`) {
		t.Errorf("expected empty-plan dry-run line, got: %q", stdout)
	}
}

func TestApplyLoadout_NotFound_WithHint(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupLoadoutWorkDir(t, fakeHome)
	writeInventorySkill(t, workDir, "solo", "Solo body.")
	writeApplyLoadout(t, workDir, "dev.yaml", "name: dev\ndescription: d\nitems:\n  - skill:solo\n")
	writeApplyLoadout(t, workDir, "docs.yaml", "name: docs\ndescription: d\nitems:\n  - skill:solo\n")

	_, stderr, err := runApplyCmd(t, fakeHome, workDir, "--loadout", "ghost")
	if err == nil {
		t.Fatal("expected not-found error, got nil")
	}
	if got := err.Error(); got != `loadout "ghost" not found in loadouts/` {
		t.Errorf("unexpected error message: %q", got)
	}
	if !strings.Contains(stderr, "hint: available loadouts: dev, docs") {
		t.Errorf("expected hint with available loadouts, got: %q", stderr)
	}
	// No changes in the environment.
	if _, statErr := os.Stat(filepath.Join(fakeHome, ".claude", "skills", "solo")); !os.IsNotExist(statErr) {
		t.Error("not-found must leave environments untouched")
	}
}

func TestApplyLoadout_NotFound_NoLoadoutsDir(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupLoadoutWorkDir(t, fakeHome)
	writeInventorySkill(t, workDir, "solo", "Solo body.")
	// No loadouts/ directory at all — same not-found class, no panic.

	_, stderr, err := runApplyCmd(t, fakeHome, workDir, "--loadout", "anything")
	if err == nil {
		t.Fatal("expected not-found error, got nil")
	}
	if got := err.Error(); got != `loadout "anything" not found in loadouts/` {
		t.Errorf("unexpected error message: %q", got)
	}
	if strings.Contains(stderr, "hint:") {
		t.Errorf("no hint expected without any loadouts, got: %q", stderr)
	}
	if _, statErr := os.Stat(filepath.Join(fakeHome, ".claude", "skills", "solo")); !os.IsNotExist(statErr) {
		t.Error("not-found must leave environments untouched")
	}
}

func TestApplyLoadout_NotFound_HintSuppressedAboveFive(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupLoadoutWorkDir(t, fakeHome)
	writeInventorySkill(t, workDir, "solo", "Solo body.")
	for i := 1; i <= 6; i++ {
		writeApplyLoadout(t, workDir, fmt.Sprintf("lo%d.yaml", i),
			fmt.Sprintf("name: lo%d\ndescription: d\nitems:\n  - skill:solo\n", i))
	}

	_, stderr, err := runApplyCmd(t, fakeHome, workDir, "--loadout", "ghost")
	if err == nil {
		t.Fatal("expected not-found error, got nil")
	}
	if strings.Contains(stderr, "hint: available loadouts:") {
		t.Errorf("hint must be suppressed with more than 5 loadouts, got: %q", stderr)
	}
}

func TestApplyLoadout_Invalid_EmptyItems(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupLoadoutWorkDir(t, fakeHome)
	writeInventorySkill(t, workDir, "solo", "Solo body.")
	writeApplyLoadout(t, workDir, "empty.yaml", "name: empty\ndescription: d\nitems: []\n")

	_, _, err := runApplyCmd(t, fakeHome, workDir, "--loadout", "empty")
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if got := err.Error(); got != `loadout "empty": items: cannot be empty` {
		t.Errorf("unexpected error message: %q", got)
	}
	if _, statErr := os.Stat(filepath.Join(fakeHome, ".claude", "skills", "solo")); !os.IsNotExist(statErr) {
		t.Error("invalid loadout must leave environments untouched")
	}
}

func TestApplyLoadout_Invalid_Unparseable(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupLoadoutWorkDir(t, fakeHome)
	writeApplyLoadout(t, workDir, "broken.yaml", "name: [unclosed\n\titems:\n")

	_, _, err := runApplyCmd(t, fakeHome, workDir, "--loadout", "broken")
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.HasPrefix(err.Error(), `loadout "broken": yaml:`) {
		t.Errorf("expected yaml parse reason, got: %q", err.Error())
	}
}

func TestApplyLoadout_MissingRef_WarnsAndProceeds(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupLoadoutWorkDir(t, fakeHome)
	writeInventorySkill(t, workDir, "real", "Real body.")
	writeApplyLoadout(t, workDir, "dev.yaml",
		"name: dev\ndescription: d\nitems:\n  - skill:real\n  - skill:ghost\n")

	stdout, stderr, err := runApplyCmd(t, fakeHome, workDir, "--loadout", "dev")
	if err != nil {
		t.Fatalf("missing ref must warn, not fail: %v", err)
	}
	if !strings.Contains(stderr, `warning: loadout "dev": no valid inventory item for skill:ghost (skipped)`) {
		t.Errorf("expected missing-ref warning, got: %q", stderr)
	}
	if !strings.Contains(stdout, `applied loadout "dev"`) {
		t.Errorf("expected success despite missing ref, got: %q", stdout)
	}
	if _, statErr := os.Stat(filepath.Join(fakeHome, ".claude", "skills", "real", "SKILL.md")); statErr != nil {
		t.Errorf("real skill must still be installed: %v", statErr)
	}
}

func TestApplyLoadout_RemovesMCPOutsideLoadout(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupLoadoutWorkDir(t, fakeHome)
	writeInventorySkill(t, workDir, "solo", "Solo body.")
	writeInventoryMCP(t, workDir, "inv-server", "claude-code")
	writeApplyLoadout(t, workDir, "skills-only.yaml",
		"name: skills-only\ndescription: d\nitems:\n  - skill:solo\n")

	claudeJSON := filepath.Join(fakeHome, ".claude.json")
	writeMCPServersJSON(t, claudeJSON, "inv-server", "foreign-server")

	stdout, _, err := runApplyCmd(t, fakeHome, workDir, "--loadout", "skills-only")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "  D mcp/inv-server.yaml   (removed from all environments)") {
		t.Errorf("expected D line for inv-server, got: %q", stdout)
	}
	keys := mcpServerKeys(t, claudeJSON)
	if keys["inv-server"] {
		t.Error("inv-server must be removed from mcpServers")
	}
	if !keys["foreign-server"] {
		t.Error("foreign-server (outside inventory) must survive")
	}
}

// TestApply_WithoutLoadout_StaysAdditive_Regression guards ADR-0004 decision 10:
// plain apply keeps its additive semantics (A/M, never D) even when loadouts/
// exists in the working tree.
func TestApply_WithoutLoadout_StaysAdditive_Regression(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupLoadoutWorkDir(t, fakeHome)
	claudeDir := filepath.Join(fakeHome, ".claude")

	writeInventorySkill(t, workDir, "alpha", "Alpha body.")
	writeInventorySkill(t, workDir, "beta", "Beta body.")
	installEnvSkill(t, claudeDir, "beta", reconcileSkillContent("beta", "Beta body."))
	// A loadout excluding beta exists — plain apply must ignore it entirely.
	writeApplyLoadout(t, workDir, "narrow.yaml", "name: narrow\ndescription: d\nitems:\n  - skill:alpha\n")

	stdout, _, err := runApplyCmd(t, fakeHome, workDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "applied: 2 skills → 1 environment") {
		t.Errorf("expected plain additive success line, got: %q", stdout)
	}
	if strings.Contains(stdout, "  D ") || strings.Contains(stdout, "removed from") {
		t.Errorf("plain apply must never plan deletions, got: %q", stdout)
	}
	// Both inventory skills present after plain apply — nothing was deleted.
	for _, name := range []string{"alpha", "beta"} {
		if _, statErr := os.Stat(filepath.Join(claudeDir, "skills", name, "SKILL.md")); statErr != nil {
			t.Errorf("%s must be installed after plain apply: %v", name, statErr)
		}
	}
}

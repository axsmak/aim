package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/axsmak/aim/internal/globalconfig"
)

// --- Issue #159: apply --pin / --default / --unpin (ADR-0006 decisions 3-4) ---
//
// Helpers reused across the cli_test package:
//   runApplyCmd, setupApplyWorkDir             — apply_test.go
//   setupLoadoutWorkDir, writeApplyLoadout     — apply_loadout_test.go
//   writeInventorySkill, installEnvSkill,
//   reconcileSkillContent                      — reconcile_test.go

// --- flag conflict matrix (decision 4) ---

func TestApplyFlags_PinWithoutLoadout_Error(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupApplyWorkDir(t, fakeHome)

	_, _, err := runApplyCmd(t, fakeHome, workDir, "--pin")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); got != "--pin requires --loadout <name>" {
		t.Errorf("unexpected error message: %q", got)
	}
}

func TestApplyFlags_DefaultWithLoadout_Error(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupApplyWorkDir(t, fakeHome)

	_, _, err := runApplyCmd(t, fakeHome, workDir, "--default", "--loadout", "x")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); got != "--default and --loadout are mutually exclusive" {
		t.Errorf("unexpected error message: %q", got)
	}
}

func TestApplyFlags_DefaultWithPin_Error(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupApplyWorkDir(t, fakeHome)

	_, _, err := runApplyCmd(t, fakeHome, workDir, "--default", "--pin")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); got != "--pin is redundant with --default" {
		t.Errorf("unexpected error message: %q", got)
	}
}

func TestApplyFlags_UnpinCombined_Error(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"with --loadout", []string{"--unpin", "--loadout", "x"}},
		{"with --pin", []string{"--unpin", "--loadout", "x", "--pin"}},
		{"with --default", []string{"--unpin", "--default"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fakeHome := t.TempDir()
			workDir := setupApplyWorkDir(t, fakeHome)

			_, _, err := runApplyCmd(t, fakeHome, workDir, tc.args...)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if got := err.Error(); got != "--unpin does not apply — combine only alone" {
				t.Errorf("unexpected error message: %q", got)
			}
		})
	}
}

func TestApplyFlags_ConflictsCheckedBeforeInventoryRead(t *testing.T) {
	// No skills/ dir, no aim.local.yaml at all: if validation ran after inventory
	// access this would fail with an unrelated I/O error instead of the flag error.
	fakeHome := t.TempDir()
	workDir := t.TempDir()

	_, _, err := runApplyCmd(t, fakeHome, workDir, "--pin")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); got != "--pin requires --loadout <name>" {
		t.Errorf("expected flag-conflict error even without a working tree, got: %q", got)
	}
}

// --- --loadout X --pin: persistence on success/failure ---

func TestApply_LoadoutPin_SavesPinOnSuccess(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupLoadoutWorkDir(t, fakeHome)
	writeInventorySkill(t, workDir, "solo", "Solo body.")
	writeApplyLoadout(t, workDir, "dev.yaml", "name: dev\ndescription: d\nitems:\n  - skill:solo\n")

	_, _, err := runApplyCmd(t, fakeHome, workDir, "--loadout", "dev", "--pin")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gcfg, err := globalconfig.Load(fakeHome)
	if err != nil {
		t.Fatalf("load global config: %v", err)
	}
	if gcfg.Loadout != "dev" {
		t.Errorf("expected pinned loadout %q, got %q", "dev", gcfg.Loadout)
	}
}

func TestApply_LoadoutPin_NoSaveOnApplyError(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupLoadoutWorkDir(t, fakeHome)
	writeInventorySkill(t, workDir, "solo", "Solo body.")
	// No loadouts/*.yaml written — "ghost" cannot resolve, runApplyLoadout fails.

	_, _, err := runApplyCmd(t, fakeHome, workDir, "--loadout", "ghost", "--pin")
	if err == nil {
		t.Fatal("expected not-found error, got nil")
	}

	configPath := filepath.Join(fakeHome, ".config", "aim", "config.yaml")
	if _, statErr := os.Stat(configPath); !os.IsNotExist(statErr) {
		t.Errorf("global config must not be written when apply fails, stat err = %v", statErr)
	}
}

func TestApply_LoadoutDryRunPin_DoesNotSaveConfig(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupLoadoutWorkDir(t, fakeHome)
	writeInventorySkill(t, workDir, "solo", "Solo body.")
	writeApplyLoadout(t, workDir, "dev.yaml", "name: dev\ndescription: d\nitems:\n  - skill:solo\n")

	stdout, _, err := runApplyCmd(t, fakeHome, workDir, "--loadout", "dev", "--pin", "--dry-run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "[dry-run]") {
		t.Errorf("expected dry-run plan output, got: %q", stdout)
	}

	configPath := filepath.Join(fakeHome, ".config", "aim", "config.yaml")
	if _, statErr := os.Stat(configPath); !os.IsNotExist(statErr) {
		t.Errorf("--dry-run --pin must never write global config, stat err = %v", statErr)
	}
}

// --- plain --loadout X (no --pin): unchanged v0.8.0 behavior ---

func TestApply_LoadoutWithoutPin_DoesNotWriteConfig(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupLoadoutWorkDir(t, fakeHome)
	writeInventorySkill(t, workDir, "solo", "Solo body.")
	writeApplyLoadout(t, workDir, "dev.yaml", "name: dev\ndescription: d\nitems:\n  - skill:solo\n")

	_, _, err := runApplyCmd(t, fakeHome, workDir, "--loadout", "dev")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	configPath := filepath.Join(fakeHome, ".config", "aim", "config.yaml")
	if _, statErr := os.Stat(configPath); !os.IsNotExist(statErr) {
		t.Errorf("apply --loadout without --pin must not write global config, stat err = %v", statErr)
	}
}

// --- --default: full inventory + pin cleared ---

func TestApply_Default_AppliesFullInventoryAndClearsPin(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupApplyWorkDir(t, fakeHome)
	writeInventorySkill(t, workDir, "alpha", "Alpha body.")

	// Pre-seed an existing pin and a Repo pointer that must survive the save.
	// Repo must point at the real workDir: resolveWorkDir(homeDir) reads it
	// back to locate the working tree for this very apply call.
	if err := globalconfig.Save(fakeHome, globalconfig.Config{Repo: workDir, Loadout: "old-pin"}); err != nil {
		t.Fatalf("seed global config: %v", err)
	}

	stdout, _, err := runApplyCmd(t, fakeHome, workDir, "--default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "applied:") {
		t.Errorf("expected additive apply success line, got: %q", stdout)
	}
	if _, statErr := os.Stat(filepath.Join(fakeHome, ".claude", "skills", "alpha", "SKILL.md")); statErr != nil {
		t.Errorf("alpha not installed: %v", statErr)
	}

	gcfg, err := globalconfig.Load(fakeHome)
	if err != nil {
		t.Fatalf("load global config: %v", err)
	}
	if gcfg.Loadout != "" {
		t.Errorf("expected pin cleared, got %q", gcfg.Loadout)
	}
	if gcfg.Loadout == "Default" {
		t.Error("literal \"Default\" must never be written")
	}
	if gcfg.Repo != workDir {
		t.Errorf("expected Repo preserved, got %q", gcfg.Repo)
	}
}

func TestApply_Default_DryRun_DoesNotSaveConfig(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupApplyWorkDir(t, fakeHome)
	writeInventorySkill(t, workDir, "alpha", "Alpha body.")

	if err := globalconfig.Save(fakeHome, globalconfig.Config{Repo: workDir, Loadout: "old-pin"}); err != nil {
		t.Fatalf("seed global config: %v", err)
	}

	_, _, err := runApplyCmd(t, fakeHome, workDir, "--default", "--dry-run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gcfg, err := globalconfig.Load(fakeHome)
	if err != nil {
		t.Fatalf("load global config: %v", err)
	}
	if gcfg.Loadout != "old-pin" {
		t.Errorf("--default --dry-run must not touch global config, got Loadout=%q", gcfg.Loadout)
	}
}

// --- --unpin: cheap config-only clear, no environment access ---

func TestApply_Unpin_ClearsPinWithoutTouchingEnvironment(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupApplyWorkDir(t, fakeHome)
	writeInventorySkill(t, workDir, "alpha", "Alpha body.")

	if err := globalconfig.Save(fakeHome, globalconfig.Config{Repo: "/some/repo", Loadout: "old-pin"}); err != nil {
		t.Fatalf("seed global config: %v", err)
	}

	_, _, err := runApplyCmd(t, fakeHome, workDir, "--unpin")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gcfg, err := globalconfig.Load(fakeHome)
	if err != nil {
		t.Fatalf("load global config: %v", err)
	}
	if gcfg.Loadout != "" {
		t.Errorf("expected pin cleared, got %q", gcfg.Loadout)
	}
	if gcfg.Repo != "/some/repo" {
		t.Errorf("expected Repo preserved, got %q", gcfg.Repo)
	}

	// No environment access: the inventory skill must not have been installed.
	if _, statErr := os.Stat(filepath.Join(fakeHome, ".claude", "skills", "alpha")); !os.IsNotExist(statErr) {
		t.Error("--unpin must not install anything into environments")
	}
}

func TestApply_Unpin_WorksWithoutLoadoutsDir(t *testing.T) {
	fakeHome := t.TempDir()
	// A working tree with no loadouts/ directory at all, and no aim.local.yaml.
	workDir := t.TempDir()

	if err := globalconfig.Save(fakeHome, globalconfig.Config{Loadout: "old-pin"}); err != nil {
		t.Fatalf("seed global config: %v", err)
	}

	_, _, err := runApplyCmd(t, fakeHome, workDir, "--unpin")
	if err != nil {
		t.Fatalf("--unpin must succeed even without loadouts/ or aim.local.yaml: %v", err)
	}

	gcfg, err := globalconfig.Load(fakeHome)
	if err != nil {
		t.Fatalf("load global config: %v", err)
	}
	if gcfg.Loadout != "" {
		t.Errorf("expected pin cleared, got %q", gcfg.Loadout)
	}
}

func TestApply_Unpin_NoOpWhenAlreadyUnpinned(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()

	_, _, err := runApplyCmd(t, fakeHome, workDir, "--unpin")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gcfg, err := globalconfig.Load(fakeHome)
	if err != nil {
		t.Fatalf("load global config: %v", err)
	}
	if gcfg.Loadout != "" {
		t.Errorf("expected no pin, got %q", gcfg.Loadout)
	}
}

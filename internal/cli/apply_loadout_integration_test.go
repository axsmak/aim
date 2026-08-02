package cli_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/axsmak/aim/internal/cli"
)

// --- Issue #153: end-to-end verification of the v0.8.0 readiness criteria ---
//
// Fake-HOME integration scenarios from BFT section 6 and the readiness
// criteria: narrowing from the full inventory, switching between loadouts,
// foreign-file preservation, folder skills with nested references, dry-run
// leaving the filesystem byte-identical, loadout-level targets, and the
// additive regression for apply/sync without --loadout (ADR-0004 decision 10).
//
// Helpers reused across the cli_test package:
//   runApplyCmd, setupApplyWorkDir          — apply_test.go
//   runSyncCmd                              — sync_integration_test.go
//   setupLoadoutWorkDir, writeApplyLoadout  — apply_loadout_test.go
//   writeInventorySkill, installEnvSkill,
//   reconcileSkillContent, writeInventoryMCP,
//   writeMCPServersJSON, mcpServerKeys      — reconcile_test.go

// snapshotFS records every directory and file (content sha256) under each
// root. Two equal snapshots mean the trees are byte-identical.
func snapshotFS(t *testing.T, roots ...string) map[string]string {
	t.Helper()
	snap := make(map[string]string)
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			key := root + "::" + rel
			if d.IsDir() {
				snap[key] = "dir"
				return nil
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			snap[key] = fmt.Sprintf("%x", sha256.Sum256(data))
			return nil
		})
		if err != nil {
			t.Fatalf("snapshot %s: %v", root, err)
		}
	}
	return snap
}

// diffSnapshots reports every key whose presence or hash differs.
func diffSnapshots(before, after map[string]string) []string {
	var diffs []string
	for k, v := range before {
		got, ok := after[k]
		switch {
		case !ok:
			diffs = append(diffs, "removed: "+k)
		case got != v:
			diffs = append(diffs, "changed: "+k)
		}
	}
	for k := range after {
		if _, ok := before[k]; !ok {
			diffs = append(diffs, "added: "+k)
		}
	}
	return diffs
}

// writeFolderInventorySkill creates skills/<name>/SKILL.md plus reference
// files (relative paths, possibly nested) in the inventory working tree.
func writeFolderInventorySkill(t *testing.T, workDir, name, body string, refs map[string]string) {
	t.Helper()
	dir := filepath.Join(workDir, "skills", name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir folder skill %s: %v", name, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"),
		[]byte(reconcileSkillContent(name, body)), 0644); err != nil {
		t.Fatalf("write folder skill %s: %v", name, err)
	}
	for rel, content := range refs {
		dest := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			t.Fatalf("mkdir ref dir for %s: %v", rel, err)
		}
		if err := os.WriteFile(dest, []byte(content), 0644); err != nil {
			t.Fatalf("write ref %s: %v", rel, err)
		}
	}
}

// mustExist / mustNotExist keep the scenario assertions readable.
func mustExist(t *testing.T, path, why string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Errorf("%s: expected %s to exist: %v", why, path, err)
	}
}

func mustNotExist(t *testing.T, path, why string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("%s: expected %s to be absent (stat err = %v)", why, path, err)
	}
}

// --- Scenario: narrowing — full inventory applied, then apply --loadout ---
//
// BFT section 6: "Применён полный инвентарь → apply --loadout "Docs" (A, B)
// → A, B — остальные инвентарные элементы удалены (среда сузилась)".
// Then a plain apply returns the environment to the full set additively —
// the round trip the manual smoke checklist walks through.
func TestApplyLoadoutIntegration_NarrowingFromFullInventory(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupLoadoutWorkDir(t, fakeHome)
	claudeDir := filepath.Join(fakeHome, ".claude")
	claudeJSON := filepath.Join(fakeHome, ".claude.json")

	writeInventorySkill(t, workDir, "docs-a", "Docs A body.")
	writeInventorySkill(t, workDir, "docs-b", "Docs B body.")
	writeInventorySkill(t, workDir, "arch-c", "Arch C body.")
	writeInventoryMCP(t, workDir, "docs-mcp", "claude-code")
	writeInventoryMCP(t, workDir, "arch-mcp", "claude-code")
	writeApplyLoadout(t, workDir, "docs.yaml",
		"name: Docs\ndescription: d\nitems:\n  - skill:docs-a\n  - skill:docs-b\n  - mcp:docs-mcp\n")

	// Foreign state seeded before anything runs: a manual skill outside the
	// inventory and a foreign mcpServers key. Both must survive every step.
	installEnvSkill(t, claudeDir, "manual-note", "# handmade, not in inventory\n")
	writeMCPServersJSON(t, claudeJSON, "foreign-server")

	// Step 1: plain apply materializes the full inventory (Default).
	if _, _, err := runApplyCmd(t, fakeHome, workDir); err != nil {
		t.Fatalf("full apply failed: %v", err)
	}
	for _, name := range []string{"docs-a", "docs-b", "arch-c"} {
		mustExist(t, filepath.Join(claudeDir, "skills", name, "SKILL.md"), "after full apply")
	}
	keys := mcpServerKeys(t, claudeJSON)
	for _, key := range []string{"docs-mcp", "arch-mcp", "foreign-server"} {
		if !keys[key] {
			t.Errorf("after full apply: expected %s in mcpServers, got %v", key, keys)
		}
	}

	// Step 2: apply --loadout Docs narrows the environment to the loadout set.
	stdout, _, err := runApplyCmd(t, fakeHome, workDir, "--loadout", "Docs")
	if err != nil {
		t.Fatalf("apply --loadout Docs failed: %v", err)
	}
	if !strings.Contains(stdout, `applied loadout "Docs"`) {
		t.Errorf("expected loadout success line, got: %q", stdout)
	}
	for _, want := range []string{
		"  D skills/arch-c.md   (removed from all environments)",
		"  D mcp/arch-mcp.yaml   (removed from all environments)",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("expected %q in delta block, got: %q", want, stdout)
		}
	}
	mustExist(t, filepath.Join(claudeDir, "skills", "docs-a", "SKILL.md"), "after narrowing")
	mustExist(t, filepath.Join(claudeDir, "skills", "docs-b", "SKILL.md"), "after narrowing")
	mustNotExist(t, filepath.Join(claudeDir, "skills", "arch-c"), "after narrowing")
	keys = mcpServerKeys(t, claudeJSON)
	if keys["arch-mcp"] {
		t.Error("after narrowing: arch-mcp (managed, outside loadout) must be removed from mcpServers")
	}
	if !keys["docs-mcp"] {
		t.Error("after narrowing: docs-mcp must remain in mcpServers")
	}
	// Foreign state untouched by the declarative path.
	mustExist(t, filepath.Join(claudeDir, "skills", "manual-note", "SKILL.md"), "after narrowing")
	if !keys["foreign-server"] {
		t.Error("after narrowing: foreign-server must survive")
	}

	// Step 3: plain apply returns to the full set additively — nothing removed.
	stdout, _, err = runApplyCmd(t, fakeHome, workDir)
	if err != nil {
		t.Fatalf("return-to-full apply failed: %v", err)
	}
	if strings.Contains(stdout, "  D ") || strings.Contains(stdout, "removed from") {
		t.Errorf("plain apply must never plan deletions, got: %q", stdout)
	}
	for _, name := range []string{"docs-a", "docs-b", "arch-c", "manual-note"} {
		mustExist(t, filepath.Join(claudeDir, "skills", name, "SKILL.md"), "after return to full")
	}
	keys = mcpServerKeys(t, claudeJSON)
	for _, key := range []string{"docs-mcp", "arch-mcp", "foreign-server"} {
		if !keys[key] {
			t.Errorf("after return to full: expected %s in mcpServers, got %v", key, keys)
		}
	}
}

// --- Scenario: switching — loadout "Docs" applied, then loadout "Arch" ---
//
// BFT section 6: "Применён loadout "Docs" (A, B) → apply --loadout "Arch"
// (C, D) → C, D — A и B удалены (в инвентаре, но не в "Arch")".
func TestApplyLoadoutIntegration_SwitchingBetweenLoadouts(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupLoadoutWorkDir(t, fakeHome)
	claudeDir := filepath.Join(fakeHome, ".claude")
	claudeJSON := filepath.Join(fakeHome, ".claude.json")

	writeInventorySkill(t, workDir, "docs-a", "Docs A body.")
	writeInventorySkill(t, workDir, "docs-b", "Docs B body.")
	writeInventorySkill(t, workDir, "arch-c", "Arch C body.")
	writeInventorySkill(t, workDir, "arch-d", "Arch D body.")
	writeInventoryMCP(t, workDir, "docs-mcp", "claude-code")
	writeInventoryMCP(t, workDir, "arch-mcp", "claude-code")
	writeApplyLoadout(t, workDir, "docs.yaml",
		"name: Docs\ndescription: d\nitems:\n  - skill:docs-a\n  - skill:docs-b\n  - mcp:docs-mcp\n")
	writeApplyLoadout(t, workDir, "arch.yaml",
		"name: Arch\ndescription: d\nitems:\n  - skill:arch-c\n  - skill:arch-d\n  - mcp:arch-mcp\n")

	installEnvSkill(t, claudeDir, "manual-note", "# handmade, not in inventory\n")
	writeMCPServersJSON(t, claudeJSON, "foreign-server")

	if _, _, err := runApplyCmd(t, fakeHome, workDir, "--loadout", "Docs"); err != nil {
		t.Fatalf("apply --loadout Docs failed: %v", err)
	}
	mustExist(t, filepath.Join(claudeDir, "skills", "docs-a", "SKILL.md"), "after Docs")
	mustExist(t, filepath.Join(claudeDir, "skills", "docs-b", "SKILL.md"), "after Docs")
	if keys := mcpServerKeys(t, claudeJSON); !keys["docs-mcp"] {
		t.Error("after Docs: docs-mcp must be installed")
	}

	stdout, _, err := runApplyCmd(t, fakeHome, workDir, "--loadout", "Arch")
	if err != nil {
		t.Fatalf("apply --loadout Arch failed: %v", err)
	}
	if !strings.Contains(stdout, `applied loadout "Arch"`) {
		t.Errorf("expected Arch success line, got: %q", stdout)
	}

	// Docs-only items removed, Arch set installed.
	mustNotExist(t, filepath.Join(claudeDir, "skills", "docs-a"), "after switch to Arch")
	mustNotExist(t, filepath.Join(claudeDir, "skills", "docs-b"), "after switch to Arch")
	mustExist(t, filepath.Join(claudeDir, "skills", "arch-c", "SKILL.md"), "after switch to Arch")
	mustExist(t, filepath.Join(claudeDir, "skills", "arch-d", "SKILL.md"), "after switch to Arch")
	keys := mcpServerKeys(t, claudeJSON)
	if keys["docs-mcp"] {
		t.Error("after switch to Arch: docs-mcp must be removed")
	}
	if !keys["arch-mcp"] {
		t.Error("after switch to Arch: arch-mcp must be installed")
	}

	// Foreign state survives the switch.
	mustExist(t, filepath.Join(claudeDir, "skills", "manual-note", "SKILL.md"), "after switch to Arch")
	if !keys["foreign-server"] {
		t.Error("after switch to Arch: foreign-server must survive")
	}
}

// --- Scenario: folder skill with nested references installs and removes whole ---
func TestApplyLoadoutIntegration_FolderSkillReferences_WholeUnit(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupLoadoutWorkDir(t, fakeHome)
	claudeDir := filepath.Join(fakeHome, ".claude")

	writeInventorySkill(t, workDir, "other", "Other body.")
	writeFolderInventorySkill(t, workDir, "folderly", "Folder body.", map[string]string{
		filepath.Join("references", "template.md"):           "template content\n",
		filepath.Join("references", "nested", "deep.tpl.md"): "deep template content\n",
	})
	writeApplyLoadout(t, workDir, "with-folder.yaml",
		"name: with-folder\ndescription: d\nitems:\n  - skill:folderly\n")
	writeApplyLoadout(t, workDir, "without-folder.yaml",
		"name: without-folder\ndescription: d\nitems:\n  - skill:other\n")

	// Install: the folder skill lands as a whole, including nested references.
	if _, _, err := runApplyCmd(t, fakeHome, workDir, "--loadout", "with-folder"); err != nil {
		t.Fatalf("apply --loadout with-folder failed: %v", err)
	}
	installedDir := filepath.Join(claudeDir, "skills", "folderly")
	mustExist(t, filepath.Join(installedDir, "SKILL.md"), "folder skill install")
	mustExist(t, filepath.Join(installedDir, "references", "template.md"), "folder skill install")
	mustExist(t, filepath.Join(installedDir, "references", "nested", "deep.tpl.md"), "folder skill install")

	// Remove: switching to a loadout without it deletes the whole directory,
	// nested references included, leaving no residue.
	if _, _, err := runApplyCmd(t, fakeHome, workDir, "--loadout", "without-folder"); err != nil {
		t.Fatalf("apply --loadout without-folder failed: %v", err)
	}
	mustNotExist(t, installedDir, "folder skill removal")
	mustExist(t, filepath.Join(claudeDir, "skills", "other", "SKILL.md"), "folder skill removal")
}

// --- Scenario: --dry-run leaves the filesystem byte-identical ---
func TestApplyLoadoutIntegration_DryRunFilesystemByteIdentical(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupLoadoutWorkDir(t, fakeHome)
	claudeDir := filepath.Join(fakeHome, ".claude")

	// Rich pending plan: A (add), M (update), D (extra skill + extra MCP key),
	// plus foreign state that must also stay untouched.
	writeInventorySkill(t, workDir, "add", "Add body.")
	writeInventorySkill(t, workDir, "update", "New body.")
	writeInventorySkill(t, workDir, "extra", "Extra body.")
	writeInventoryMCP(t, workDir, "wanted-mcp", "claude-code")
	writeInventoryMCP(t, workDir, "extra-mcp", "claude-code")
	installEnvSkill(t, claudeDir, "update", reconcileSkillContent("update", "Old body."))
	installEnvSkill(t, claudeDir, "extra", reconcileSkillContent("extra", "Extra body."))
	installEnvSkill(t, claudeDir, "manual-note", "# handmade, not in inventory\n")
	writeMCPServersJSON(t, filepath.Join(fakeHome, ".claude.json"), "extra-mcp", "foreign-server")
	writeApplyLoadout(t, workDir, "preview.yaml",
		"name: preview\ndescription: d\nitems:\n  - skill:add\n  - skill:update\n  - mcp:wanted-mcp\n")

	before := snapshotFS(t, fakeHome, workDir)

	stdout, _, err := runApplyCmd(t, fakeHome, workDir, "--loadout", "preview", "--dry-run")
	if err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}
	// The full A/M/D plan is shown...
	for _, want := range []string{
		"  A skills/add.md   (new in all environments)",
		"  M skills/update.md   (differs in all environments)",
		"  D skills/extra.md   (would remove from all environments)",
		"  D mcp/extra-mcp.yaml   (would remove from all environments)",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("expected %q in dry-run plan, got: %q", want, stdout)
		}
	}

	// ...and not a single byte on disk changed — home and working tree alike.
	after := snapshotFS(t, fakeHome, workDir)
	if !reflect.DeepEqual(before, after) {
		t.Errorf("dry-run modified the filesystem:\n%s", strings.Join(diffSnapshots(before, after), "\n"))
	}
}

// --- Scenario: loadout targets — excluded environments are never touched ---
func TestApplyLoadoutIntegration_TargetsExcludedEnvUntouched(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupLoadoutWorkDir(t, fakeHome)
	claudeDir := filepath.Join(fakeHome, ".claude")
	cursorDir := filepath.Join(fakeHome, ".cursor")
	if err := os.MkdirAll(cursorDir, 0755); err != nil {
		t.Fatalf("mkdir .cursor: %v", err)
	}

	writeInventorySkill(t, workDir, "solo", "Solo body.")
	writeInventorySkill(t, workDir, "extra", "Extra body.")
	writeInventoryMCP(t, workDir, "ctx", "cursor")

	// cursor holds a managed skill outside the loadout and a managed MCP key —
	// prime deletion candidates, were cursor not excluded by loadout targets.
	installEnvSkill(t, claudeDir, "extra", reconcileSkillContent("extra", "Extra body."))
	installEnvSkill(t, cursorDir, "extra", reconcileSkillContent("extra", "Extra body."))
	writeMCPServersJSON(t, filepath.Join(cursorDir, "mcp.json"), "ctx")

	writeApplyLoadout(t, workDir, "scoped.yaml",
		"name: scoped\ndescription: d\ntargets:\n  - claude-code\nitems:\n  - skill:solo\n")

	cursorBefore := snapshotFS(t, cursorDir)

	stdout, _, err := runApplyCmd(t, fakeHome, workDir, "--loadout", "scoped")
	if err != nil {
		t.Fatalf("apply --loadout scoped failed: %v", err)
	}
	if strings.Contains(stdout, "cursor") {
		t.Errorf("excluded environment must not appear in the output, got: %q", stdout)
	}

	// claude-code reconciled: solo installed, extra removed.
	mustExist(t, filepath.Join(claudeDir, "skills", "solo", "SKILL.md"), "targeted env")
	mustNotExist(t, filepath.Join(claudeDir, "skills", "extra"), "targeted env")

	// cursor byte-identical: no installs, no removals, MCP key intact.
	cursorAfter := snapshotFS(t, cursorDir)
	if !reflect.DeepEqual(cursorBefore, cursorAfter) {
		t.Errorf("excluded environment was modified:\n%s", strings.Join(diffSnapshots(cursorBefore, cursorAfter), "\n"))
	}
}

// --- Regression: sync without --loadout stays additive with loadouts/ present ---
//
// BFT criterion 6 / ADR-0004 decision 10: sync is the transport of the full
// inventory and never removes anything from an environment, even when the
// working tree contains loadouts that exclude installed items. Complements
// TestApply_WithoutLoadout_StaysAdditive_Regression (apply_loadout_test.go).
func TestSyncIntegration_WithoutLoadout_StaysAdditive_Regression(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := setupLoadoutWorkDir(t, fakeHome)
	claudeDir := filepath.Join(fakeHome, ".claude")
	claudeJSON := filepath.Join(fakeHome, ".claude.json")

	writeInventorySkill(t, workDir, "alpha", "Alpha body.")
	writeInventorySkill(t, workDir, "beta", "Beta body.")
	writeInventoryMCP(t, workDir, "inv-server", "claude-code")
	// A loadout excluding beta and inv-server exists — sync must ignore it.
	writeApplyLoadout(t, workDir, "narrow.yaml",
		"name: narrow\ndescription: d\nitems:\n  - skill:alpha\n")

	// Pre-state: managed skill outside the narrow loadout already installed,
	// plus foreign skill and foreign MCP key.
	installEnvSkill(t, claudeDir, "beta", reconcileSkillContent("beta", "Beta body."))
	installEnvSkill(t, claudeDir, "manual-note", "# handmade, not in inventory\n")
	writeMCPServersJSON(t, claudeJSON, "foreign-server")

	stdout, _, err := runSyncCmd(t, fakeHome, workDir)
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	if strings.Contains(stdout, "  D ") || strings.Contains(stdout, "removed from") {
		t.Errorf("sync without --loadout must never report deletions, got: %q", stdout)
	}

	// Everything present before is still present; the full inventory is in.
	for _, name := range []string{"alpha", "beta", "manual-note"} {
		mustExist(t, filepath.Join(claudeDir, "skills", name, "SKILL.md"), "after additive sync")
	}
	keys := mcpServerKeys(t, claudeJSON)
	for _, key := range []string{"inv-server", "foreign-server"} {
		if !keys[key] {
			t.Errorf("after additive sync: expected %s in mcpServers, got %v", key, keys)
		}
	}
}

// --- CLI surface: sync --update-only does not exist (cancelled by ADR-0006) ---
func TestCLISurface_NoUpdateOnlyFlag(t *testing.T) {
	// Flag definitions: neither sync nor apply declares --update-only.
	root := cli.NewRootCmd("test")
	for _, name := range []string{"sync", "apply"} {
		cmd, _, err := root.Find([]string{name})
		if err != nil || cmd == nil || cmd.Name() != name {
			t.Fatalf("cannot find %q command: %v", name, err)
		}
		if f := cmd.Flags().Lookup("update-only"); f != nil {
			t.Errorf("%s must not define --update-only (cancelled by ADR-0006), found %q", name, f.Name)
		}
	}

	// End to end: cobra rejects the flag before any work happens.
	_, _, err := runSyncCmd(t, t.TempDir(), t.TempDir(), "--update-only")
	if err == nil {
		t.Fatal("sync --update-only must fail, got nil error")
	}
	if !strings.Contains(err.Error(), "unknown flag: --update-only") {
		t.Errorf("expected unknown-flag error, got: %q", err.Error())
	}
}

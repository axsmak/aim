package cli_test

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/axsmak/aim/internal/cli"
	"github.com/axsmak/aim/internal/loadout"
	"github.com/axsmak/aim/internal/localconfig"
)

// --- helpers ---

func reconcileSkillContent(name, body string) string {
	return fmt.Sprintf("---\nname: %s\ndescription: Test skill %s\n---\n\n# Role\n%s\n", name, name, body)
}

// reconcileSkillContentWithTargets renders a skill with an item-level
// targets list in its frontmatter (ADR-0007), mirroring reconcileMCPContent.
func reconcileSkillContentWithTargets(name, body string, targets ...string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "---\nname: %s\ndescription: Test skill %s\ntargets:\n", name, name)
	for _, tgt := range targets {
		fmt.Fprintf(&sb, "  - %s\n", tgt)
	}
	fmt.Fprintf(&sb, "---\n\n# Role\n%s\n", body)
	return sb.String()
}

func reconcileMCPContent(name string, targets ...string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "name: %s\ndescription: Test MCP %s\ncommand: npx\nargs: []\ntargets:\n", name, name)
	for _, tgt := range targets {
		fmt.Fprintf(&sb, "  - %s\n", tgt)
	}
	sb.WriteString("env: []\n")
	return sb.String()
}

func writeInventorySkill(t *testing.T, workDir, name, body string) {
	t.Helper()
	if err := os.WriteFile(
		filepath.Join(workDir, "skills", name+".md"),
		[]byte(reconcileSkillContent(name, body)), 0644,
	); err != nil {
		t.Fatalf("write inventory skill %s: %v", name, err)
	}
}

func writeInventorySkillWithTargets(t *testing.T, workDir, name, body string, targets ...string) {
	t.Helper()
	if err := os.WriteFile(
		filepath.Join(workDir, "skills", name+".md"),
		[]byte(reconcileSkillContentWithTargets(name, body, targets...)), 0644,
	); err != nil {
		t.Fatalf("write inventory skill %s: %v", name, err)
	}
}

func writeInventoryMCP(t *testing.T, workDir, name string, targets ...string) {
	t.Helper()
	if err := os.WriteFile(
		filepath.Join(workDir, "mcp", name+".yaml"),
		[]byte(reconcileMCPContent(name, targets...)), 0644,
	); err != nil {
		t.Fatalf("write inventory mcp %s: %v", name, err)
	}
}

// installEnvSkill materializes a skill in an env the way AIM does:
// <baseDir>/skills/<name>/SKILL.md.
func installEnvSkill(t *testing.T, baseDir, name, content string) {
	t.Helper()
	dir := filepath.Join(baseDir, "skills", name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir env skill %s: %v", name, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0644); err != nil {
		t.Fatalf("write env skill %s: %v", name, err)
	}
}

func writeMCPServersJSON(t *testing.T, path string, keys ...string) {
	t.Helper()
	servers := map[string]interface{}{}
	for _, k := range keys {
		servers[k] = map[string]interface{}{"command": "npx", "args": []string{}}
	}
	data, err := json.MarshalIndent(map[string]interface{}{"mcpServers": servers}, "", "  ")
	if err != nil {
		t.Fatalf("marshal mcp config: %v", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mcpServerKeys(t *testing.T, path string) map[string]bool {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("read %s: %v", path, err)
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
	servers, _ := cfg["mcpServers"].(map[string]interface{})
	keys := make(map[string]bool, len(servers))
	for k := range servers {
		keys[k] = true
	}
	return keys
}

func parseTestLoadout(t *testing.T, yamlDoc string) loadout.Loadout {
	t.Helper()
	lo, errs, _ := loadout.Parse([]byte(yamlDoc), "")
	if len(errs) > 0 {
		t.Fatalf("test loadout invalid: %v", errs)
	}
	return lo
}

// setupReconcileDirs creates a workdir with skills/ and mcp/ plus a fake home
// with the requested env dirs (".claude", ".cursor", ".codex").
func setupReconcileDirs(t *testing.T, envDirs ...string) (workDir, fakeHome string) {
	t.Helper()
	workDir = t.TempDir()
	fakeHome = t.TempDir()
	for _, sub := range []string{"skills", "mcp"} {
		if err := os.MkdirAll(filepath.Join(workDir, sub), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", sub, err)
		}
	}
	for _, d := range envDirs {
		if err := os.MkdirAll(filepath.Join(fakeHome, d), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	return workDir, fakeHome
}

func buildTestPlan(t *testing.T, workDir, fakeHome string, lo loadout.Loadout) *cli.ReconcilePlan {
	t.Helper()
	inv, warnings, err := cli.LoadReconcileInventory(
		filepath.Join(workDir, "skills"), filepath.Join(workDir, "mcp"))
	if err != nil {
		t.Fatalf("load inventory: %v", err)
	}
	_ = warnings
	envs := cli.DetectReconcileEnvs(localconfig.Config{}, fakeHome)
	plan, err := cli.BuildReconcilePlan(lo, inv, envs)
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	return plan
}

func executeTestPlan(t *testing.T, plan *cli.ReconcilePlan) cli.ReconcileResult {
	t.Helper()
	cfg := localconfig.Config{}
	res, err := cli.ExecuteReconcilePlan(plan, &cfg, strings.NewReader(""), io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("execute plan: %v", err)
	}
	return res
}

func findAction(plan *cli.ReconcilePlan, kind loadout.ItemKind, name string) *cli.ReconcileAction {
	for i := range plan.Actions {
		if plan.Actions[i].Kind == kind && plan.Actions[i].Name == name {
			return &plan.Actions[i]
		}
	}
	return nil
}

// --- three reconciliation branches (ADR-0004 decision 3) ---

func TestReconcile_ThreeBranches_Skills(t *testing.T) {
	workDir, fakeHome := setupReconcileDirs(t, ".claude")
	claudeDir := filepath.Join(fakeHome, ".claude")

	// Inventory: keep, update, remove, add.
	writeInventorySkill(t, workDir, "keep", "Keep body.")
	writeInventorySkill(t, workDir, "update", "New body.")
	writeInventorySkill(t, workDir, "remove", "Remove body.")
	writeInventorySkill(t, workDir, "add", "Add body.")

	// Env: keep (identical), update (stale), remove (inventory, not in loadout),
	// foreign (not in inventory at all).
	installEnvSkill(t, claudeDir, "keep", reconcileSkillContent("keep", "Keep body."))
	installEnvSkill(t, claudeDir, "update", reconcileSkillContent("update", "Old body."))
	installEnvSkill(t, claudeDir, "remove", reconcileSkillContent("remove", "Remove body."))
	installEnvSkill(t, claudeDir, "foreign", "# handmade, not in inventory\n")

	lo := parseTestLoadout(t, "name: Test\ndescription: d\nitems:\n  - skill:keep\n  - skill:update\n  - skill:add\n")
	plan := buildTestPlan(t, workDir, fakeHome, lo)

	// Branch 1: loadout items → A when absent, M when differing, nothing when identical.
	if a := findAction(plan, loadout.KindSkill, "add"); a == nil || a.Category != "A" {
		t.Errorf("expected A for add, got %+v", a)
	}
	if a := findAction(plan, loadout.KindSkill, "update"); a == nil || a.Category != "M" {
		t.Errorf("expected M for update, got %+v", a)
	}
	if a := findAction(plan, loadout.KindSkill, "keep"); a != nil {
		t.Errorf("identical skill must produce no action, got %+v", a)
	}
	// Branch 2: inventory item outside the loadout, present in env → D.
	if a := findAction(plan, loadout.KindSkill, "remove"); a == nil || a.Category != "D" {
		t.Errorf("expected D for remove, got %+v", a)
	}
	// Branch 3: file outside the inventory namespace → invisible to the plan.
	if a := findAction(plan, loadout.KindSkill, "foreign"); a != nil {
		t.Errorf("foreign skill must never enter the plan, got %+v", a)
	}

	// Plan-only mode performed no writes.
	if _, err := os.Stat(filepath.Join(claudeDir, "skills", "add")); !os.IsNotExist(err) {
		t.Error("plan building must not install skills")
	}
	if _, err := os.Stat(filepath.Join(claudeDir, "skills", "remove", "SKILL.md")); err != nil {
		t.Error("plan building must not remove skills")
	}

	res := executeTestPlan(t, plan)

	// Execution brings the env exactly to the loadout set within the namespace.
	if _, err := os.Stat(filepath.Join(claudeDir, "skills", "add", "SKILL.md")); err != nil {
		t.Errorf("add not installed: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(claudeDir, "skills", "update", "SKILL.md"))
	if err != nil || string(got) != reconcileSkillContent("update", "New body.") {
		t.Errorf("update not refreshed: err=%v content=%q", err, got)
	}
	if _, err := os.Stat(filepath.Join(claudeDir, "skills", "remove")); !os.IsNotExist(err) {
		t.Error("remove must be deleted from env")
	}
	if _, err := os.Stat(filepath.Join(claudeDir, "skills", "foreign", "SKILL.md")); err != nil {
		t.Error("foreign skill must survive reconciliation")
	}
	if _, err := os.Stat(filepath.Join(claudeDir, "skills", "keep", "SKILL.md")); err != nil {
		t.Error("keep must remain installed")
	}

	if res.SkillCount != 3 || res.EnvCount != 1 {
		t.Errorf("unexpected result counts: %+v", res)
	}
	joined := strings.Join(res.DeltaLines, "\n")
	if !strings.Contains(joined, "D skills/remove.md   (removed from all environments)") {
		t.Errorf("expected real-run D line, got:\n%s", joined)
	}
}

// --- targets: loadout-level exclusion leaves the env completely untouched ---

func TestReconcile_LoadoutTargetsExcludeEnv(t *testing.T) {
	workDir, fakeHome := setupReconcileDirs(t, ".claude", ".cursor")
	claudeDir := filepath.Join(fakeHome, ".claude")
	cursorDir := filepath.Join(fakeHome, ".cursor")

	writeInventorySkill(t, workDir, "solo", "Solo body.")
	writeInventorySkill(t, workDir, "extra", "Extra body.")

	// "extra" is installed in both envs and is outside the loadout.
	installEnvSkill(t, claudeDir, "extra", reconcileSkillContent("extra", "Extra body."))
	installEnvSkill(t, cursorDir, "extra", reconcileSkillContent("extra", "Extra body."))

	lo := parseTestLoadout(t, "name: Scoped\ndescription: d\ntargets:\n  - claude-code\nitems:\n  - skill:solo\n")
	plan := buildTestPlan(t, workDir, fakeHome, lo)

	if names := plan.EnvNames(); len(names) != 1 || names[0] != "claude-code" {
		t.Fatalf("expected plan to cover only claude-code, got %v", names)
	}
	for _, a := range plan.Actions {
		for _, env := range a.Envs {
			if env == "cursor" {
				t.Errorf("cursor excluded by loadout targets must not appear in plan: %+v", a)
			}
		}
	}

	executeTestPlan(t, plan)

	// claude-code reconciled: extra removed, solo installed.
	if _, err := os.Stat(filepath.Join(claudeDir, "skills", "extra")); !os.IsNotExist(err) {
		t.Error("extra must be removed from claude-code")
	}
	if _, err := os.Stat(filepath.Join(claudeDir, "skills", "solo", "SKILL.md")); err != nil {
		t.Error("solo must be installed in claude-code")
	}
	// cursor untouched entirely: extra stays, solo not installed.
	if _, err := os.Stat(filepath.Join(cursorDir, "skills", "extra", "SKILL.md")); err != nil {
		t.Error("cursor excluded by loadout targets must keep its skills")
	}
	if _, err := os.Stat(filepath.Join(cursorDir, "skills", "solo")); !os.IsNotExist(err) {
		t.Error("cursor excluded by loadout targets must not receive installs")
	}
}

// --- targets: intersection of loadout-level and item-level (MCP) ---

func TestReconcile_MCPTargetsIntersection(t *testing.T) {
	workDir, fakeHome := setupReconcileDirs(t, ".claude", ".cursor")
	cursorDir := filepath.Join(fakeHome, ".cursor")

	// Inventory MCP targets only cursor; loadout has no targets of its own.
	writeInventoryMCP(t, workDir, "ctx", "cursor")

	// The key is nevertheless present in claude-code's config (e.g. installed
	// before targets were narrowed) — desired set for claude-code excludes it
	// by the intersection, so it must be planned D there.
	writeMCPServersJSON(t, filepath.Join(fakeHome, ".claude.json"), "ctx")

	lo := parseTestLoadout(t, "name: MCPTest\ndescription: d\nitems:\n  - mcp:ctx\n")
	plan := buildTestPlan(t, workDir, fakeHome, lo)

	a := findAction(plan, loadout.KindMCP, "ctx")
	if a == nil {
		t.Fatalf("expected actions for mcp ctx, got none: %+v", plan.Actions)
	}
	// Aggregated entry: A in cursor (absent there, desired), D handled per env.
	var sawA, sawD bool
	for _, act := range plan.Actions {
		if act.Kind != loadout.KindMCP || act.Name != "ctx" {
			continue
		}
		switch act.Category {
		case "A":
			sawA = true
			if len(act.Envs) != 1 || act.Envs[0] != "cursor" {
				t.Errorf("A for ctx must target cursor only, got %v", act.Envs)
			}
		case "D":
			sawD = true
			if len(act.Envs) != 1 || act.Envs[0] != "claude-code" {
				t.Errorf("D for ctx must target claude-code only, got %v", act.Envs)
			}
		}
	}
	if !sawA || !sawD {
		t.Fatalf("expected both A (cursor) and D (claude-code) for ctx, actions: %+v", plan.Actions)
	}

	executeTestPlan(t, plan)

	if keys := mcpServerKeys(t, filepath.Join(fakeHome, ".claude.json")); keys["ctx"] {
		t.Error("ctx must be removed from claude-code config (item targets exclude it)")
	}
	if keys := mcpServerKeys(t, filepath.Join(cursorDir, "mcp.json")); !keys["ctx"] {
		t.Error("ctx must be installed in cursor config")
	}
}

// --- targets: intersection of loadout-level and item-level (skills, ADR-0007) ---

func TestReconcile_SkillTargetsIntersection(t *testing.T) {
	workDir, fakeHome := setupReconcileDirs(t, ".claude", ".cursor")
	cursorDir := filepath.Join(fakeHome, ".cursor")

	// Inventory skill "solo" targets only cursor; loadout has no targets of
	// its own, and "everywhere" carries no targets at all (admits every env).
	writeInventorySkillWithTargets(t, workDir, "solo", "Solo body.", "cursor")
	writeInventorySkill(t, workDir, "everywhere", "Everywhere body.")

	// "solo" is nevertheless installed in claude-code (e.g. from before
	// targets were narrowed) — the intersection excludes claude-code from
	// its desired set there, so it must be planned D in that environment.
	installEnvSkill(t, filepath.Join(fakeHome, ".claude"), "solo", reconcileSkillContentWithTargets("solo", "Solo body.", "cursor"))

	lo := parseTestLoadout(t, "name: SkillTest\ndescription: d\nitems:\n  - skill:solo\n  - skill:everywhere\n")
	plan := buildTestPlan(t, workDir, fakeHome, lo)

	var sawA, sawD bool
	for _, act := range plan.Actions {
		if act.Kind != loadout.KindSkill || act.Name != "solo" {
			continue
		}
		switch act.Category {
		case "A":
			sawA = true
			if len(act.Envs) != 1 || act.Envs[0] != "cursor" {
				t.Errorf("A for solo must target cursor only, got %v", act.Envs)
			}
		case "D":
			sawD = true
			if len(act.Envs) != 1 || act.Envs[0] != "claude-code" {
				t.Errorf("D for solo must target claude-code only, got %v", act.Envs)
			}
		}
	}
	if !sawA || !sawD {
		t.Fatalf("expected both A (cursor) and D (claude-code) for solo, actions: %+v", plan.Actions)
	}

	// Empty targets ("everywhere") is desired in both admissible environments.
	if a := findAction(plan, loadout.KindSkill, "everywhere"); a == nil || len(a.Envs) != 2 {
		t.Errorf("everywhere must be A in both environments, got %+v", a)
	}

	// The D line must not claim "all environments" — the plan covers two
	// environments but the action touches only one.
	dry := strings.Join(plan.DeltaLines(true), "\n")
	if !strings.Contains(dry, "D skills/solo.md   (would remove from claude-code)") {
		t.Errorf("expected scoped D line for solo, got:\n%s", dry)
	}
	if strings.Contains(dry, "solo.md   (would remove from all environments)") {
		t.Errorf("D line for solo must not claim all environments, got:\n%s", dry)
	}

	executeTestPlan(t, plan)

	if _, err := os.Stat(filepath.Join(fakeHome, ".claude", "skills", "solo")); !os.IsNotExist(err) {
		t.Error("solo must be removed from claude-code (item targets exclude it)")
	}
	if _, err := os.Stat(filepath.Join(cursorDir, "skills", "solo", "SKILL.md")); err != nil {
		t.Error("solo must be installed in cursor")
	}
	if _, err := os.Stat(filepath.Join(cursorDir, "skills", "everywhere", "SKILL.md")); err != nil {
		t.Error("everywhere must be installed in cursor")
	}
	if _, err := os.Stat(filepath.Join(fakeHome, ".claude", "skills", "everywhere", "SKILL.md")); err != nil {
		t.Error("everywhere must be installed in claude-code")
	}
}

// --- targets: guard holds across every environment, not just one (ADR-0007 decision 5) ---

func TestReconcile_SkillTargetsGuardInvalidFileAcrossEnvs(t *testing.T) {
	workDir, fakeHome := setupReconcileDirs(t, ".claude", ".cursor")
	claudeDir := filepath.Join(fakeHome, ".claude")
	cursorDir := filepath.Join(fakeHome, ".cursor")

	// "broken" exists in the inventory but fails validation, so its targets
	// are unknowable. The loadout asks for it explicitly and it is installed
	// in both environments; neither must plan it for deletion.
	if err := os.WriteFile(filepath.Join(workDir, "skills", "broken.md"),
		[]byte("---\nno-name: x\n---\n\nBody.\n"), 0644); err != nil {
		t.Fatalf("write invalid skill: %v", err)
	}
	installEnvSkill(t, claudeDir, "broken", "installed earlier\n")
	installEnvSkill(t, cursorDir, "broken", "installed earlier\n")

	lo := parseTestLoadout(t, "name: Guard2\ndescription: d\nitems:\n  - skill:broken\n")
	plan := buildTestPlan(t, workDir, fakeHome, lo)

	if a := findAction(plan, loadout.KindSkill, "broken"); a != nil {
		t.Errorf("loadout-referenced invalid skill must not get any action in any env, got %+v", a)
	}

	executeTestPlan(t, plan)
	if _, err := os.Stat(filepath.Join(claudeDir, "skills", "broken", "SKILL.md")); err != nil {
		t.Error("broken must survive in claude-code")
	}
	if _, err := os.Stat(filepath.Join(cursorDir, "skills", "broken", "SKILL.md")); err != nil {
		t.Error("broken must survive in cursor")
	}
}

// --- MCP: foreign keys stay untouched (ADR-0004 decision 6) ---

func TestReconcile_MCPForeignKeysUntouched(t *testing.T) {
	workDir, fakeHome := setupReconcileDirs(t, ".claude")

	writeInventoryMCP(t, workDir, "inv-server", "claude-code")
	writeInventorySkill(t, workDir, "some-skill", "Body.")

	// Env config holds an inventory key (outside the loadout) and a foreign key.
	claudeJSON := filepath.Join(fakeHome, ".claude.json")
	writeMCPServersJSON(t, claudeJSON, "inv-server", "foreign-server")

	lo := parseTestLoadout(t, "name: SkillsOnly\ndescription: d\nitems:\n  - skill:some-skill\n")
	plan := buildTestPlan(t, workDir, fakeHome, lo)

	if a := findAction(plan, loadout.KindMCP, "inv-server"); a == nil || a.Category != "D" {
		t.Errorf("expected D for inv-server, got %+v", a)
	}
	if a := findAction(plan, loadout.KindMCP, "foreign-server"); a != nil {
		t.Errorf("foreign MCP key must never enter the plan, got %+v", a)
	}

	executeTestPlan(t, plan)

	keys := mcpServerKeys(t, claudeJSON)
	if keys["inv-server"] {
		t.Error("inv-server must be removed from mcpServers")
	}
	if !keys["foreign-server"] {
		t.Error("foreign-server must survive reconciliation")
	}
}

// --- idempotency: second run of the same loadout yields an empty plan ---

func TestReconcile_Idempotent(t *testing.T) {
	workDir, fakeHome := setupReconcileDirs(t, ".claude")
	claudeDir := filepath.Join(fakeHome, ".claude")

	writeInventorySkill(t, workDir, "alpha", "Alpha body.")
	writeInventorySkill(t, workDir, "beta", "Beta body.")
	writeInventoryMCP(t, workDir, "gamma", "claude-code")
	writeInventoryMCP(t, workDir, "delta", "claude-code")

	// Pre-state: beta installed (will be D), delta present (will be D).
	installEnvSkill(t, claudeDir, "beta", reconcileSkillContent("beta", "Beta body."))
	writeMCPServersJSON(t, filepath.Join(fakeHome, ".claude.json"), "delta")

	lo := parseTestLoadout(t, "name: Idem\ndescription: d\nitems:\n  - skill:alpha\n  - mcp:gamma\n")

	first := buildTestPlan(t, workDir, fakeHome, lo)
	if first.Empty() {
		t.Fatal("first plan must not be empty")
	}
	executeTestPlan(t, first)

	second := buildTestPlan(t, workDir, fakeHome, lo)
	if !second.Empty() {
		t.Errorf("second plan must be empty, got actions: %+v", second.Actions)
	}
	if lines := second.DeltaLines(false); len(lines) != 0 {
		t.Errorf("empty plan must produce no delta lines, got %v", lines)
	}
}

// --- plan-only mode performs no writes at all ---

func TestReconcile_PlanOnlyWritesNothing(t *testing.T) {
	workDir, fakeHome := setupReconcileDirs(t, ".claude")
	claudeDir := filepath.Join(fakeHome, ".claude")

	writeInventorySkill(t, workDir, "alpha", "Alpha body.")
	writeInventoryMCP(t, workDir, "gamma", "claude-code")
	writeInventorySkill(t, workDir, "stale", "Stale body.")
	installEnvSkill(t, claudeDir, "stale", reconcileSkillContent("stale", "Stale body."))

	lo := parseTestLoadout(t, "name: DryOnly\ndescription: d\nitems:\n  - skill:alpha\n  - mcp:gamma\n")
	plan := buildTestPlan(t, workDir, fakeHome, lo)
	if plan.Empty() {
		t.Fatal("plan must contain A and D entries")
	}

	if _, err := os.Stat(filepath.Join(claudeDir, "skills", "alpha")); !os.IsNotExist(err) {
		t.Error("plan-only must not install skills")
	}
	if _, err := os.Stat(filepath.Join(claudeDir, "skills", "stale", "SKILL.md")); err != nil {
		t.Error("plan-only must not remove skills")
	}
	if _, err := os.Stat(filepath.Join(fakeHome, ".claude.json")); !os.IsNotExist(err) {
		t.Error("plan-only must not create MCP configs")
	}
}

// --- delta line formats (ADR-0003 markers, consumed by PrintDeltaBlock) ---

func TestReconcile_DeltaLineFormats(t *testing.T) {
	workDir, fakeHome := setupReconcileDirs(t, ".claude")
	claudeDir := filepath.Join(fakeHome, ".claude")

	writeInventorySkill(t, workDir, "add", "Add body.")
	writeInventorySkill(t, workDir, "update", "New body.")
	writeInventorySkill(t, workDir, "remove", "Remove body.")
	writeInventoryMCP(t, workDir, "gone", "claude-code")

	installEnvSkill(t, claudeDir, "update", reconcileSkillContent("update", "Old body."))
	installEnvSkill(t, claudeDir, "remove", reconcileSkillContent("remove", "Remove body."))
	writeMCPServersJSON(t, filepath.Join(fakeHome, ".claude.json"), "gone")

	lo := parseTestLoadout(t, "name: Fmt\ndescription: d\nitems:\n  - skill:add\n  - skill:update\n")
	plan := buildTestPlan(t, workDir, fakeHome, lo)

	run := strings.Join(plan.DeltaLines(false), "\n")
	for _, want := range []string{
		"A skills/add.md   (new in all environments)",
		"M skills/update.md   (updated in all environments)",
		"D skills/remove.md   (removed from all environments)",
		"D mcp/gone.yaml   (removed from all environments)",
	} {
		if !strings.Contains(run, want) {
			t.Errorf("real-run delta missing %q, got:\n%s", want, run)
		}
	}

	dry := strings.Join(plan.DeltaLines(true), "\n")
	for _, want := range []string{
		"A skills/add.md   (new in all environments)",
		"M skills/update.md   (differs in all environments)",
		"D skills/remove.md   (would remove from all environments)",
		"D mcp/gone.yaml   (would remove from all environments)",
	} {
		if !strings.Contains(dry, want) {
			t.Errorf("dry-run delta missing %q, got:\n%s", want, dry)
		}
	}
	if strings.Contains(dry, "updated in") || strings.Contains(dry, "removed from all") {
		t.Errorf("dry-run must not use real-run qualifiers, got:\n%s", dry)
	}
}

// --- namespace: folder and invalid inventory skills are still AIM-managed ---

func TestReconcile_NamespaceCoversFolderAndInvalidSkills(t *testing.T) {
	workDir, fakeHome := setupReconcileDirs(t, ".claude")
	claudeDir := filepath.Join(fakeHome, ".claude")

	writeInventorySkill(t, workDir, "wanted", "Wanted body.")

	// Folder-format inventory skill: skills/folderly/SKILL.md.
	folderDir := filepath.Join(workDir, "skills", "folderly")
	if err := os.MkdirAll(folderDir, 0755); err != nil {
		t.Fatalf("mkdir folder skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(folderDir, "SKILL.md"),
		[]byte(reconcileSkillContent("folderly", "Folder body.")), 0644); err != nil {
		t.Fatalf("write folder skill: %v", err)
	}

	// Invalid inventory skill: exists as skills/<name>.md → still in namespace.
	if err := os.WriteFile(filepath.Join(workDir, "skills", "broken.md"),
		[]byte("---\nno-name: x\n---\n\nBody.\n"), 0644); err != nil {
		t.Fatalf("write invalid skill: %v", err)
	}

	installEnvSkill(t, claudeDir, "folderly", reconcileSkillContent("folderly", "Folder body."))
	installEnvSkill(t, claudeDir, "broken", "whatever\n")

	lo := parseTestLoadout(t, "name: NS\ndescription: d\nitems:\n  - skill:wanted\n")
	plan := buildTestPlan(t, workDir, fakeHome, lo)

	if a := findAction(plan, loadout.KindSkill, "folderly"); a == nil || a.Category != "D" {
		t.Errorf("folder-format inventory skill outside loadout must be D, got %+v", a)
	}
	if a := findAction(plan, loadout.KindSkill, "broken"); a == nil || a.Category != "D" {
		t.Errorf("invalid inventory skill outside loadout must still be D (namespace is about existence), got %+v", a)
	}

	executeTestPlan(t, plan)
	if _, err := os.Stat(filepath.Join(claudeDir, "skills", "folderly")); !os.IsNotExist(err) {
		t.Error("folderly must be removed")
	}
	if _, err := os.Stat(filepath.Join(claudeDir, "skills", "broken")); !os.IsNotExist(err) {
		t.Error("broken must be removed")
	}
}

// --- loadout-referenced names are never deleted, even when currently invalid ---

func TestReconcile_LoadoutRefGuardsInvalidSkillFromDeletion(t *testing.T) {
	workDir, fakeHome := setupReconcileDirs(t, ".claude")
	claudeDir := filepath.Join(fakeHome, ".claude")

	// "broken" exists in the inventory but fails validation; the loadout asks
	// for it explicitly. It cannot be installed, but it must not be deleted.
	if err := os.WriteFile(filepath.Join(workDir, "skills", "broken.md"),
		[]byte("---\nno-name: x\n---\n\nBody.\n"), 0644); err != nil {
		t.Fatalf("write invalid skill: %v", err)
	}
	installEnvSkill(t, claudeDir, "broken", "installed earlier\n")

	lo := parseTestLoadout(t, "name: Guard\ndescription: d\nitems:\n  - skill:broken\n  - skill:ghost\n")
	plan := buildTestPlan(t, workDir, fakeHome, lo)

	if a := findAction(plan, loadout.KindSkill, "broken"); a != nil {
		t.Errorf("loadout-referenced invalid skill must not get any action, got %+v", a)
	}

	// Both unresolvable refs are surfaced for the caller.
	missing := make(map[string]bool)
	for _, ref := range plan.MissingRefs {
		missing[ref.String()] = true
	}
	if !missing["skill:broken"] || !missing["skill:ghost"] {
		t.Errorf("expected skill:broken and skill:ghost in MissingRefs, got %v", plan.MissingRefs)
	}

	executeTestPlan(t, plan)
	if _, err := os.Stat(filepath.Join(claudeDir, "skills", "broken", "SKILL.md")); err != nil {
		t.Error("loadout-referenced skill must survive even when its inventory file is invalid")
	}
}

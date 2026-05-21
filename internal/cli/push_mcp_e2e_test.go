package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/axsmak/aim/internal/localconfig"
)

// TestPushSync_E2E_SkillsAndMCP simulates the full A→B publish→sync cycle for
// both skills AND MCP. Machine A creates skill + MCP, pushes to a bare remote;
// Machine B clones from the same remote and syncs, verifying that the skill and
// MCP server appear in Machine B's fake HOME.
func TestPushSync_E2E_SkillsAndMCP(t *testing.T) {
	// --- Step 1: Create a bare git repo as the fake remote ---
	bareURL := setupEmptyBareRepo(t)

	// --- Step 2: Machine A ---
	fakeHomeA := t.TempDir()
	if err := os.MkdirAll(filepath.Join(fakeHomeA, ".claude"), 0755); err != nil {
		t.Fatalf("mkdir .claude A: %v", err)
	}

	// aiman init clones the (empty) bare repo into workDirA
	workDirA := initWorkDirForPush(t, fakeHomeA, bareURL)

	// Create skills/ with a test skill
	if err := os.MkdirAll(filepath.Join(workDirA, "skills"), 0755); err != nil {
		t.Fatalf("mkdir skills A: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(workDirA, "skills", "test-skill.md"),
		[]byte(validSkillContent),
		0644,
	); err != nil {
		t.Fatalf("write test-skill.md A: %v", err)
	}

	// Create mcp/ with a test server (no required env — won't block on stdin)
	if err := os.MkdirAll(filepath.Join(workDirA, "mcp"), 0755); err != nil {
		t.Fatalf("mkdir mcp A: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(workDirA, "mcp", "test-server.yaml"),
		[]byte(validMCPContent),
		0644,
	); err != nil {
		t.Fatalf("write test-server.yaml A: %v", err)
	}

	// Add .gitignore so aim.local.yaml is not accidentally staged
	if err := os.WriteFile(
		filepath.Join(workDirA, ".gitignore"),
		[]byte("aim.local.yaml\n"),
		0644,
	); err != nil {
		t.Fatalf("write .gitignore A: %v", err)
	}

	// aiman push from Machine A — stages skills/ + mcp/, commits, pushes
	stdout, _, err := runAimCmd(t, fakeHomeA, workDirA, "push")
	if err != nil {
		t.Fatalf("aiman push A: %v (stdout: %q)", err, stdout)
	}

	cfgA, err := localconfig.Load(workDirA)
	if err != nil {
		t.Fatalf("load config A after push: %v", err)
	}
	if cfgA.PublishedHash == "" {
		t.Fatal("published_hash empty after successful aiman push on Machine A")
	}

	// --- Step 3: Machine B ---
	fakeHomeB := t.TempDir()
	if err := os.MkdirAll(filepath.Join(fakeHomeB, ".claude"), 0755); err != nil {
		t.Fatalf("mkdir .claude B: %v", err)
	}
	workDirB := t.TempDir()

	// aiman init clones from the bare repo (now contains A's commit)
	if _, _, err := runAimCmd(t, fakeHomeB, workDirB, "init", "--path", workDirB, bareURL); err != nil {
		t.Fatalf("aiman init B: %v", err)
	}

	// aiman sync fetches + applies skills and MCP into fakeHomeB
	if _, _, err := runAimCmd(t, fakeHomeB, workDirB, "sync"); err != nil {
		t.Fatalf("aiman sync B: %v", err)
	}

	// --- Step 4: Assertions on Machine B ---

	// Skill installed: homeB/.claude/skills/test-skill/SKILL.md
	skillPath := filepath.Join(fakeHomeB, ".claude", "skills", "test-skill", "SKILL.md")
	if _, err := os.Stat(skillPath); err != nil {
		t.Errorf("skill not installed on Machine B at %s: %v", skillPath, err)
	}

	// MCP installed: homeB/.claude/settings.json contains "test-server" in mcpServers
	settingsPath := filepath.Join(fakeHomeB, ".claude", "settings.json")
	settingsData, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("settings.json not created on Machine B: %v", err)
	}
	var settings map[string]interface{}
	if err := json.Unmarshal(settingsData, &settings); err != nil {
		t.Fatalf("invalid JSON in settings.json on Machine B: %v", err)
	}
	mcpServers, ok := settings["mcpServers"].(map[string]interface{})
	if !ok {
		t.Fatal("mcpServers missing from settings.json on Machine B")
	}
	if _, found := mcpServers["test-server"]; !found {
		t.Errorf("test-server not found in mcpServers on Machine B; got keys: %v", mcpServersKeys(mcpServers))
	}

	// synced_hash in workDirB/aim.local.yaml is set and non-empty
	cfgB, err := localconfig.Load(workDirB)
	if err != nil {
		t.Fatalf("load config B after sync: %v", err)
	}
	if cfgB.SyncedHash == "" {
		t.Fatal("synced_hash empty on Machine B after successful aiman sync")
	}

	// synced_hash on B must equal published_hash on A (same commit)
	if cfgA.PublishedHash != cfgB.SyncedHash {
		t.Errorf("published_hash A (%q) != synced_hash B (%q)", cfgA.PublishedHash, cfgB.SyncedHash)
	}
}

// mcpServersKeys returns the keys of the mcpServers map for error messages.
func mcpServersKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

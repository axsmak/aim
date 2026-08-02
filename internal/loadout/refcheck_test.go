package loadout

import (
	"os"
	"path/filepath"
	"testing"
)

// setupInventory creates a minimal inventory layout:
// skills/flat.md, skills/folder/SKILL.md, mcp/server.yaml.
func setupInventory(t *testing.T) (skillsDir, mcpDir string) {
	t.Helper()
	dir := t.TempDir()
	skillsDir = filepath.Join(dir, "skills")
	mcpDir = filepath.Join(dir, "mcp")
	if err := os.MkdirAll(filepath.Join(skillsDir, "folder"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(mcpDir, 0755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		filepath.Join(skillsDir, "flat.md"):            "---\nname: flat\ndescription: d\n---\nbody",
		filepath.Join(skillsDir, "folder", "SKILL.md"): "---\nname: folder\ndescription: d\n---\nbody",
		filepath.Join(mcpDir, "server.yaml"):           "name: server\n",
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return skillsDir, mcpDir
}

func loadoutWithRefs(refs ...ItemRef) Loadout {
	return Loadout{Name: "Test Loadout", FilePath: "loadouts/test-loadout.yaml", Refs: refs}
}

func TestCheckRefs_allResolve(t *testing.T) {
	skillsDir, mcpDir := setupInventory(t)
	l := loadoutWithRefs(
		ItemRef{Kind: KindSkill, Name: "flat"},
		ItemRef{Kind: KindSkill, Name: "folder"},
		ItemRef{Kind: KindMCP, Name: "server"},
	)
	if broken := CheckRefs(l, skillsDir, mcpDir); len(broken) != 0 {
		t.Errorf("expected no broken refs, got %v", broken)
	}
}

func TestCheckRefs_unknownSkill(t *testing.T) {
	skillsDir, mcpDir := setupInventory(t)
	l := loadoutWithRefs(ItemRef{Kind: KindSkill, Name: "missing-skill"})
	broken := CheckRefs(l, skillsDir, mcpDir)
	if len(broken) != 1 {
		t.Fatalf("expected 1 broken ref, got %v", broken)
	}
	want := `loadout "Test Loadout" references unknown skill "missing-skill"`
	if got := broken[0].Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	if broken[0].FilePath != l.FilePath {
		t.Errorf("FilePath = %q, want %q", broken[0].FilePath, l.FilePath)
	}
}

func TestCheckRefs_unknownMCP(t *testing.T) {
	skillsDir, mcpDir := setupInventory(t)
	l := loadoutWithRefs(ItemRef{Kind: KindMCP, Name: "missing-server"})
	broken := CheckRefs(l, skillsDir, mcpDir)
	if len(broken) != 1 {
		t.Fatalf("expected 1 broken ref, got %v", broken)
	}
	want := `loadout "Test Loadout" references unknown mcp "missing-server"`
	if got := broken[0].Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestCheckRefs_returnsAllBrokenRefs(t *testing.T) {
	skillsDir, mcpDir := setupInventory(t)
	l := loadoutWithRefs(
		ItemRef{Kind: KindSkill, Name: "flat"},
		ItemRef{Kind: KindSkill, Name: "ghost-one"},
		ItemRef{Kind: KindMCP, Name: "ghost-two"},
	)
	broken := CheckRefs(l, skillsDir, mcpDir)
	if len(broken) != 2 {
		t.Fatalf("expected 2 broken refs (US-L04: all at once), got %v", broken)
	}
	if broken[0].Ref.Name != "ghost-one" || broken[1].Ref.Name != "ghost-two" {
		t.Errorf("unexpected broken refs: %v", broken)
	}
}

func TestCheckRefs_skillDirWithoutSkillMD_isUnknown(t *testing.T) {
	skillsDir, mcpDir := setupInventory(t)
	// A bare directory without SKILL.md is not a skill.
	if err := os.MkdirAll(filepath.Join(skillsDir, "empty-dir"), 0755); err != nil {
		t.Fatal(err)
	}
	l := loadoutWithRefs(ItemRef{Kind: KindSkill, Name: "empty-dir"})
	if broken := CheckRefs(l, skillsDir, mcpDir); len(broken) != 1 {
		t.Errorf("expected directory without SKILL.md to be unknown, got %v", broken)
	}
}

func TestCheckRefs_pathSeparatorNeverResolves(t *testing.T) {
	skillsDir, mcpDir := setupInventory(t)
	l := loadoutWithRefs(
		ItemRef{Kind: KindSkill, Name: "../skills/flat"},
		ItemRef{Kind: KindMCP, Name: `..\mcp\server`},
	)
	if broken := CheckRefs(l, skillsDir, mcpDir); len(broken) != 2 {
		t.Errorf("expected path-separator names to be rejected, got %v", broken)
	}
}

func TestCheckRefs_missingInventoryDirs(t *testing.T) {
	// Loadout referencing anything while skills/ and mcp/ do not exist.
	dir := t.TempDir()
	l := loadoutWithRefs(
		ItemRef{Kind: KindSkill, Name: "flat"},
		ItemRef{Kind: KindMCP, Name: "server"},
	)
	broken := CheckRefs(l, filepath.Join(dir, "skills"), filepath.Join(dir, "mcp"))
	if len(broken) != 2 {
		t.Errorf("expected all refs broken when inventory dirs are missing, got %v", broken)
	}
}

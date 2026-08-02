package adapter_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/axsmak/aim/internal/adapter"
	"github.com/axsmak/aim/internal/mcp"
	"github.com/axsmak/aim/internal/skill"
)

// allAdapters returns one instance of each adapter for table-driven tests:
// RemoveSkill shares one helper, so behavior must be identical everywhere.
func allAdapters() []adapter.Adapter {
	return []adapter.Adapter{
		adapter.NewClaudeCodeAdapter(""),
		adapter.NewCursorAdapter(""),
		adapter.NewCodexAdapter(""),
	}
}

func TestRemoveSkill_FlatSkill_DeletesDir(t *testing.T) {
	for _, a := range allAdapters() {
		t.Run(a.Name(), func(t *testing.T) {
			baseDir := filepath.Join(t.TempDir(), "env")
			s := skill.Skill{Name: "flat-skill", Raw: []byte("---\nname: flat-skill\n---\nbody")}
			if err := a.InstallSkill(s, baseDir); err != nil {
				t.Fatalf("InstallSkill() error = %v", err)
			}

			if err := a.RemoveSkill("flat-skill", baseDir); err != nil {
				t.Fatalf("RemoveSkill() error = %v", err)
			}

			if _, err := os.Stat(filepath.Join(baseDir, "skills", "flat-skill")); !os.IsNotExist(err) {
				t.Errorf("skill dir still exists after RemoveSkill, stat err = %v", err)
			}
		})
	}
}

func TestRemoveSkill_FolderSkill_DeletesNestedRefs(t *testing.T) {
	// Folder skill with a nested reference subdirectory (post-#145 install layout).
	sourceDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(sourceDir, "references"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "references", "backend.tpl.md"), []byte("# tpl"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "patterns.md"), []byte("# patterns"), 0644); err != nil {
		t.Fatal(err)
	}

	s := skill.Skill{
		Name:      "folder-skill",
		Raw:       []byte("---\nname: folder-skill\n---\nbody"),
		SourceDir: sourceDir,
		RefFiles:  []string{"patterns.md", filepath.Join("references", "backend.tpl.md")},
	}

	a := adapter.NewClaudeCodeAdapter("")
	baseDir := filepath.Join(t.TempDir(), ".claude")
	if err := a.InstallSkill(s, baseDir); err != nil {
		t.Fatalf("InstallSkill() error = %v", err)
	}
	// Sanity: nested ref actually installed.
	if _, err := os.Stat(filepath.Join(baseDir, "skills", "folder-skill", "references", "backend.tpl.md")); err != nil {
		t.Fatalf("nested ref not installed: %v", err)
	}

	if err := a.RemoveSkill("folder-skill", baseDir); err != nil {
		t.Fatalf("RemoveSkill() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(baseDir, "skills", "folder-skill")); !os.IsNotExist(err) {
		t.Errorf("folder skill dir still exists after RemoveSkill, stat err = %v", err)
	}
}

func TestRemoveSkill_NonExistent_NoOp(t *testing.T) {
	for _, a := range allAdapters() {
		t.Run(a.Name(), func(t *testing.T) {
			baseDir := filepath.Join(t.TempDir(), "env")
			if err := a.RemoveSkill("never-installed", baseDir); err != nil {
				t.Errorf("RemoveSkill() on missing skill = %v, want nil", err)
			}
		})
	}
}

func TestRemoveSkill_LeavesSiblingsUntouched(t *testing.T) {
	a := adapter.NewClaudeCodeAdapter("")
	baseDir := filepath.Join(t.TempDir(), ".claude")
	for _, name := range []string{"keep-me", "remove-me"} {
		s := skill.Skill{Name: name, Raw: []byte("---\nname: " + name + "\n---\nbody")}
		if err := a.InstallSkill(s, baseDir); err != nil {
			t.Fatal(err)
		}
	}

	if err := a.RemoveSkill("remove-me", baseDir); err != nil {
		t.Fatalf("RemoveSkill() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(baseDir, "skills", "keep-me", "SKILL.md")); err != nil {
		t.Errorf("sibling skill lost after RemoveSkill: %v", err)
	}
}

func TestRemoveSkill_PathTraversal_Rejected(t *testing.T) {
	a := adapter.NewClaudeCodeAdapter("")
	home := t.TempDir()
	baseDir := filepath.Join(home, ".claude")
	// A file outside skills/ that a traversal could reach.
	victim := filepath.Join(home, "victim.txt")
	if err := os.MkdirAll(filepath.Join(baseDir, "skills"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(victim, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"", ".", "..", "../..", "a/b", `a\b`, "../victim.txt"} {
		if err := a.RemoveSkill(name, baseDir); err == nil {
			t.Errorf("RemoveSkill(%q) = nil, want error", name)
		}
	}

	if _, err := os.Stat(victim); err != nil {
		t.Errorf("file outside skills dir was affected: %v", err)
	}
	if _, err := os.Stat(filepath.Join(baseDir, "skills")); err != nil {
		t.Errorf("skills dir itself was affected: %v", err)
	}
}

func TestClaudeCodeAdapter_RemoveMCP_DeletesOnlyTargetKey(t *testing.T) {
	home := t.TempDir()
	baseDir := filepath.Join(home, ".claude")
	if err := os.Mkdir(baseDir, 0755); err != nil {
		t.Fatal(err)
	}
	existing := `{"existingKey":"value","mcpServers":{"other-server":{"command":"node","args":[]}}}`
	if err := os.WriteFile(filepath.Join(home, ".claude.json"), []byte(existing), 0644); err != nil {
		t.Fatal(err)
	}

	a := adapter.NewClaudeCodeAdapter("")
	if err := a.InstallMCP(testMCP, baseDir, nil); err != nil {
		t.Fatal(err)
	}

	if err := a.RemoveMCP("test-server", baseDir); err != nil {
		t.Fatalf("RemoveMCP() error = %v", err)
	}

	cfg := readJSONFile(t, filepath.Join(home, ".claude.json"))
	if cfg["existingKey"] != "value" {
		t.Error("existingKey lost after RemoveMCP")
	}
	servers, ok := cfg["mcpServers"].(map[string]interface{})
	if !ok {
		t.Fatal("mcpServers missing after RemoveMCP")
	}
	if _, ok := servers["other-server"]; !ok {
		t.Error("other-server lost after RemoveMCP")
	}
	if _, ok := servers["test-server"]; ok {
		t.Error("test-server still present after RemoveMCP")
	}
}

func TestClaudeCodeAdapter_RemoveMCP_MissingKey_NoOp(t *testing.T) {
	home := t.TempDir()
	baseDir := filepath.Join(home, ".claude")
	if err := os.Mkdir(baseDir, 0755); err != nil {
		t.Fatal(err)
	}
	existing := `{"mcpServers":{"other-server":{"command":"node","args":[]}}}`
	configPath := filepath.Join(home, ".claude.json")
	if err := os.WriteFile(configPath, []byte(existing), 0644); err != nil {
		t.Fatal(err)
	}

	a := adapter.NewClaudeCodeAdapter("")
	if err := a.RemoveMCP("never-added", baseDir); err != nil {
		t.Fatalf("RemoveMCP() on missing key = %v, want nil", err)
	}

	// File must not be rewritten: content stays byte-identical.
	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != existing {
		t.Errorf("config rewritten on no-op removal:\ngot  %s\nwant %s", got, existing)
	}
}

func TestClaudeCodeAdapter_RemoveMCP_MissingFile_NoOp(t *testing.T) {
	home := t.TempDir()
	baseDir := filepath.Join(home, ".claude")
	if err := os.Mkdir(baseDir, 0755); err != nil {
		t.Fatal(err)
	}

	a := adapter.NewClaudeCodeAdapter("")
	if err := a.RemoveMCP("anything", baseDir); err != nil {
		t.Fatalf("RemoveMCP() with no config file = %v, want nil", err)
	}

	if _, err := os.Stat(filepath.Join(home, ".claude.json")); !os.IsNotExist(err) {
		t.Errorf(".claude.json created as a side effect of RemoveMCP, stat err = %v", err)
	}
}

func TestCursorAdapter_RemoveMCP_DeletesOnlyTargetKey(t *testing.T) {
	home := t.TempDir()
	baseDir := filepath.Join(home, ".cursor")
	if err := os.Mkdir(baseDir, 0755); err != nil {
		t.Fatal(err)
	}

	a := adapter.NewCursorAdapter("")
	for _, name := range []string{"keep-server", "drop-server"} {
		m := mcp.MCP{Name: name, Command: "node", Args: []string{"srv.js"}, Targets: []string{"cursor"}}
		if err := a.InstallMCP(m, baseDir, nil); err != nil {
			t.Fatal(err)
		}
	}

	if err := a.RemoveMCP("drop-server", baseDir); err != nil {
		t.Fatalf("RemoveMCP() error = %v", err)
	}

	cfg := readJSONFile(t, filepath.Join(baseDir, "mcp.json"))
	servers := cfg["mcpServers"].(map[string]interface{})
	if _, ok := servers["keep-server"]; !ok {
		t.Error("keep-server lost after RemoveMCP")
	}
	if _, ok := servers["drop-server"]; ok {
		t.Error("drop-server still present after RemoveMCP")
	}
}

func TestCursorAdapter_RemoveMCP_MissingFile_NoOp(t *testing.T) {
	home := t.TempDir()
	baseDir := filepath.Join(home, ".cursor")
	if err := os.Mkdir(baseDir, 0755); err != nil {
		t.Fatal(err)
	}

	a := adapter.NewCursorAdapter("")
	if err := a.RemoveMCP("anything", baseDir); err != nil {
		t.Fatalf("RemoveMCP() with no mcp.json = %v, want nil", err)
	}
	if _, err := os.Stat(filepath.Join(baseDir, "mcp.json")); !os.IsNotExist(err) {
		t.Errorf("mcp.json created as a side effect of RemoveMCP, stat err = %v", err)
	}
}

func TestCodexAdapter_RemoveMCP_DeletesOnlyTargetKey(t *testing.T) {
	home := t.TempDir()
	baseDir := filepath.Join(home, ".codex")
	if err := os.Mkdir(baseDir, 0755); err != nil {
		t.Fatal(err)
	}

	a := adapter.NewCodexAdapter("")
	for _, name := range []string{"keep-server", "drop-server"} {
		m := mcp.MCP{Name: name, Command: "node", Args: []string{"srv.js"}, Targets: []string{"codex"}}
		if err := a.InstallMCP(m, baseDir, map[string]string{"KEY": "v"}); err != nil {
			t.Fatal(err)
		}
	}

	if err := a.RemoveMCP("drop-server", baseDir); err != nil {
		t.Fatalf("RemoveMCP() error = %v", err)
	}

	got, err := a.ScanMCP(baseDir)
	if err != nil {
		t.Fatalf("ScanMCP() error = %v", err)
	}
	if len(got) != 1 || got[0].ServerName != "keep-server" {
		t.Errorf("ScanMCP() after RemoveMCP = %v, want only keep-server", got)
	}
	// Env of the surviving entry must be preserved.
	if got[0].Env["KEY"] != "v" {
		t.Errorf("surviving server env = %v, want KEY=v", got[0].Env)
	}
}

func TestCodexAdapter_RemoveMCP_MissingFile_NoOp(t *testing.T) {
	home := t.TempDir()
	baseDir := filepath.Join(home, ".codex")
	if err := os.Mkdir(baseDir, 0755); err != nil {
		t.Fatal(err)
	}

	a := adapter.NewCodexAdapter("")
	if err := a.RemoveMCP("anything", baseDir); err != nil {
		t.Fatalf("RemoveMCP() with no config.toml = %v, want nil", err)
	}
	if _, err := os.Stat(filepath.Join(baseDir, "config.toml")); !os.IsNotExist(err) {
		t.Errorf("config.toml created as a side effect of RemoveMCP, stat err = %v", err)
	}
}

func TestCodexAdapter_RemoveMCP_MissingKey_PreservesFile(t *testing.T) {
	home := t.TempDir()
	baseDir := filepath.Join(home, ".codex")
	if err := os.Mkdir(baseDir, 0755); err != nil {
		t.Fatal(err)
	}
	existing := "model = \"o3\"\n\n[mcp_servers.other]\ncommand = \"node\"\nargs = [\"srv.js\"]\n"
	configPath := filepath.Join(baseDir, "config.toml")
	if err := os.WriteFile(configPath, []byte(existing), 0644); err != nil {
		t.Fatal(err)
	}

	a := adapter.NewCodexAdapter("")
	if err := a.RemoveMCP("never-added", baseDir); err != nil {
		t.Fatalf("RemoveMCP() on missing key = %v, want nil", err)
	}

	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != existing {
		t.Errorf("config.toml rewritten on no-op removal:\ngot  %s\nwant %s", got, existing)
	}
}

func TestClaudeCodeAdapter_RemoveMCP_RoundTripsThroughScanMCP(t *testing.T) {
	home := t.TempDir()
	baseDir := filepath.Join(home, ".claude")
	if err := os.Mkdir(baseDir, 0755); err != nil {
		t.Fatal(err)
	}

	a := adapter.NewClaudeCodeAdapter("")
	if err := a.InstallMCP(testMCP, baseDir, nil); err != nil {
		t.Fatal(err)
	}
	if err := a.RemoveMCP(testMCP.Name, baseDir); err != nil {
		t.Fatalf("RemoveMCP() error = %v", err)
	}

	got, err := a.ScanMCP(baseDir)
	if err != nil {
		t.Fatalf("ScanMCP() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ScanMCP() after RemoveMCP = %v, want empty", got)
	}
}

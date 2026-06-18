package adapter_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/axsmak/aim/internal/adapter"
	"github.com/axsmak/aim/internal/mcp"
	"github.com/axsmak/aim/internal/skill"
)

var testMCP = mcp.MCP{
	Name:    "test-server",
	Command: "npx",
	Args:    []string{"-y", "test-pkg"},
	Targets: []string{"claude-code"},
	Env:     []mcp.EnvVar{},
}

func TestClaudeCodeAdapter_Name(t *testing.T) {
	a := adapter.ClaudeCodeAdapter{}
	if got := a.Name(); got != "claude-code" {
		t.Errorf("Name() = %q, want %q", got, "claude-code")
	}
}

func TestClaudeCodeAdapter_Detect_Found(t *testing.T) {
	home := t.TempDir()
	claudeDir := filepath.Join(home, ".claude")
	if err := os.Mkdir(claudeDir, 0755); err != nil {
		t.Fatal(err)
	}

	a := adapter.ClaudeCodeAdapter{}
	got, ok := a.Detect(home)
	if !ok {
		t.Fatal("Detect() ok = false, want true")
	}
	if got != claudeDir {
		t.Errorf("Detect() path = %q, want %q", got, claudeDir)
	}
}

func TestClaudeCodeAdapter_Detect_NotFound(t *testing.T) {
	home := t.TempDir()

	a := adapter.ClaudeCodeAdapter{}
	got, ok := a.Detect(home)
	if ok {
		t.Fatal("Detect() ok = true, want false")
	}
	if got != "" {
		t.Errorf("Detect() path = %q, want empty string", got)
	}
}

func TestClaudeCodeAdapter_InstallSkill_WritesCorrectPath(t *testing.T) {
	home := t.TempDir()
	baseDir := filepath.Join(home, ".claude")
	if err := os.Mkdir(baseDir, 0755); err != nil {
		t.Fatal(err)
	}

	s := skill.Skill{
		Name: "my-skill",
		Raw:  []byte("---\nname: my-skill\n---\n# Body"),
	}

	a := adapter.ClaudeCodeAdapter{}
	if err := a.InstallSkill(s, baseDir); err != nil {
		t.Fatalf("InstallSkill() error = %v", err)
	}

	want := filepath.Join(baseDir, "skills", "my-skill", "SKILL.md")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("expected file not found at %s: %v", want, err)
	}
}

func TestClaudeCodeAdapter_InstallSkill_ContentMatchesRaw(t *testing.T) {
	home := t.TempDir()
	baseDir := filepath.Join(home, ".claude")
	if err := os.Mkdir(baseDir, 0755); err != nil {
		t.Fatal(err)
	}

	raw := []byte("---\nname: my-skill\ndescription: does something\n---\n# Role\nAn agent.")
	s := skill.Skill{Name: "my-skill", Raw: raw}

	a := adapter.ClaudeCodeAdapter{}
	if err := a.InstallSkill(s, baseDir); err != nil {
		t.Fatalf("InstallSkill() error = %v", err)
	}

	dest := filepath.Join(baseDir, "skills", "my-skill", "SKILL.md")
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != string(raw) {
		t.Errorf("file content = %q, want %q", got, raw)
	}
}

func TestClaudeCodeAdapter_InstallSkill_Idempotent(t *testing.T) {
	home := t.TempDir()
	baseDir := filepath.Join(home, ".claude")
	if err := os.Mkdir(baseDir, 0755); err != nil {
		t.Fatal(err)
	}

	s := skill.Skill{
		Name: "my-skill",
		Raw:  []byte("---\nname: my-skill\n---\noriginal"),
	}

	a := adapter.ClaudeCodeAdapter{}
	if err := a.InstallSkill(s, baseDir); err != nil {
		t.Fatalf("first InstallSkill() error = %v", err)
	}

	updated := skill.Skill{
		Name: "my-skill",
		Raw:  []byte("---\nname: my-skill\n---\nupdated"),
	}
	if err := a.InstallSkill(updated, baseDir); err != nil {
		t.Fatalf("second InstallSkill() error = %v", err)
	}

	dest := filepath.Join(baseDir, "skills", "my-skill", "SKILL.md")
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != string(updated.Raw) {
		t.Errorf("after overwrite content = %q, want %q", got, updated.Raw)
	}
}

func TestClaudeCodeAdapter_InstallSkill_CreatesNestedDirectories(t *testing.T) {
	home := t.TempDir()
	// baseDir does NOT exist yet — InstallSkill must create the full nested path
	baseDir := filepath.Join(home, ".claude")

	s := skill.Skill{
		Name: "nested-skill",
		Raw:  []byte("---\nname: nested-skill\n---\nbody"),
	}

	a := adapter.ClaudeCodeAdapter{}
	if err := a.InstallSkill(s, baseDir); err != nil {
		t.Fatalf("InstallSkill() error = %v", err)
	}

	want := filepath.Join(baseDir, "skills", "nested-skill", "SKILL.md")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("expected file not found at %s: %v", want, err)
	}
}

func TestClaudeCodeAdapter_InstallSkill_FolderSkill_CopiesRefs(t *testing.T) {
	// Set up a source dir with reference files
	sourceDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(sourceDir, "agent-patterns.md"), []byte("# Patterns\nContent."), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "delegation.md"), []byte("# Delegation\nNotes."), 0644); err != nil {
		t.Fatal(err)
	}

	s := skill.Skill{
		Name:      "write-agent",
		Raw:       []byte("---\nname: write-agent\ndescription: Write agent definitions\n---\n\n# Role\nHelps write agents.\n"),
		SourceDir: sourceDir,
		RefFiles:  []string{"agent-patterns.md", "delegation.md"},
	}

	home := t.TempDir()
	baseDir := filepath.Join(home, ".claude")

	a := adapter.ClaudeCodeAdapter{}
	if err := a.InstallSkill(s, baseDir); err != nil {
		t.Fatalf("InstallSkill() error = %v", err)
	}

	destDir := filepath.Join(baseDir, "skills", "write-agent")

	// SKILL.md must exist
	if _, err := os.Stat(filepath.Join(destDir, "SKILL.md")); err != nil {
		t.Errorf("SKILL.md not found: %v", err)
	}
	// Reference files must be copied
	for _, ref := range []string{"agent-patterns.md", "delegation.md"} {
		dest := filepath.Join(destDir, ref)
		got, err := os.ReadFile(dest)
		if err != nil {
			t.Errorf("ref file %s not found: %v", ref, err)
			continue
		}
		src, _ := os.ReadFile(filepath.Join(sourceDir, ref))
		if string(got) != string(src) {
			t.Errorf("ref file %s content mismatch: got %q, want %q", ref, got, src)
		}
	}
}

func TestClaudeCodeAdapter_InstallSkill_FlatSkill_NoRefFilesDir(t *testing.T) {
	s := skill.Skill{
		Name: "flat-skill",
		Raw:  []byte("---\nname: flat-skill\ndescription: A flat skill\n---\n\n# Role\nDoes something.\n"),
		// SourceDir and RefFiles are zero values (flat skill)
	}

	home := t.TempDir()
	baseDir := filepath.Join(home, ".claude")

	a := adapter.ClaudeCodeAdapter{}
	if err := a.InstallSkill(s, baseDir); err != nil {
		t.Fatalf("InstallSkill() error = %v", err)
	}

	destDir := filepath.Join(baseDir, "skills", "flat-skill")

	// SKILL.md must exist
	if _, err := os.Stat(filepath.Join(destDir, "SKILL.md")); err != nil {
		t.Errorf("SKILL.md not found: %v", err)
	}

	// No extra files should exist — only SKILL.md
	entries, err := os.ReadDir(destDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("expected only SKILL.md, got: %v", names)
	}
}

func readJSONFile(t *testing.T, path string) map[string]interface{} {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Unmarshal(%s): %v", path, err)
	}
	return result
}

func TestClaudeCodeAdapter_InstallMCP_CreatesClaudeJSON(t *testing.T) {
	home := t.TempDir()
	baseDir := filepath.Join(home, ".claude")
	if err := os.Mkdir(baseDir, 0755); err != nil {
		t.Fatal(err)
	}

	a := adapter.NewClaudeCodeAdapter("")
	if err := a.InstallMCP(testMCP, baseDir, map[string]string{"API_KEY": "secret"}); err != nil {
		t.Fatalf("InstallMCP() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(baseDir, "settings.json")); !os.IsNotExist(err) {
		t.Errorf("settings.json should not be written by InstallMCP, stat err = %v", err)
	}

	cfg := readJSONFile(t, filepath.Join(home, ".claude.json"))
	servers, ok := cfg["mcpServers"].(map[string]interface{})
	if !ok {
		t.Fatal("mcpServers missing or wrong type")
	}
	entry, ok := servers["test-server"].(map[string]interface{})
	if !ok {
		t.Fatal("test-server entry missing")
	}
	if entry["command"] != "npx" {
		t.Errorf("command = %v, want npx", entry["command"])
	}
	env, _ := entry["env"].(map[string]interface{})
	if env["API_KEY"] != "secret" {
		t.Errorf("env.API_KEY = %v, want secret", env["API_KEY"])
	}
}

func TestClaudeCodeAdapter_InstallMCP_MergesExistingKeys(t *testing.T) {
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
		t.Fatalf("InstallMCP() error = %v", err)
	}

	cfg := readJSONFile(t, filepath.Join(home, ".claude.json"))
	if cfg["existingKey"] != "value" {
		t.Error("existingKey lost after InstallMCP")
	}
	servers := cfg["mcpServers"].(map[string]interface{})
	if _, ok := servers["other-server"]; !ok {
		t.Error("other-server lost after InstallMCP")
	}
	if _, ok := servers["test-server"]; !ok {
		t.Error("test-server not added")
	}
}

func TestClaudeCodeAdapter_InstallMCP_OverwritesSameServer(t *testing.T) {
	home := t.TempDir()
	baseDir := filepath.Join(home, ".claude")
	if err := os.Mkdir(baseDir, 0755); err != nil {
		t.Fatal(err)
	}

	a := adapter.NewClaudeCodeAdapter("")
	if err := a.InstallMCP(testMCP, baseDir, map[string]string{"API_KEY": "old"}); err != nil {
		t.Fatal(err)
	}
	if err := a.InstallMCP(testMCP, baseDir, map[string]string{"API_KEY": "new"}); err != nil {
		t.Fatal(err)
	}

	cfg := readJSONFile(t, filepath.Join(home, ".claude.json"))
	servers := cfg["mcpServers"].(map[string]interface{})
	entry := servers["test-server"].(map[string]interface{})
	env := entry["env"].(map[string]interface{})
	if env["API_KEY"] != "new" {
		t.Errorf("API_KEY = %v, want new", env["API_KEY"])
	}
}

func TestClaudeCodeAdapter_InstallMCP_NoEnvSection(t *testing.T) {
	home := t.TempDir()
	baseDir := filepath.Join(home, ".claude")
	if err := os.Mkdir(baseDir, 0755); err != nil {
		t.Fatal(err)
	}

	a := adapter.NewClaudeCodeAdapter("")
	if err := a.InstallMCP(testMCP, baseDir, nil); err != nil {
		t.Fatalf("InstallMCP() error = %v", err)
	}

	cfg := readJSONFile(t, filepath.Join(home, ".claude.json"))
	servers := cfg["mcpServers"].(map[string]interface{})
	entry := servers["test-server"].(map[string]interface{})
	if _, ok := entry["env"]; ok {
		t.Error("env section should be absent when envValues is nil")
	}
}

func TestClaudeCodeAdapter_InstallMCP_RoundTripsThroughScanMCP(t *testing.T) {
	home := t.TempDir()
	baseDir := filepath.Join(home, ".claude")
	if err := os.Mkdir(baseDir, 0755); err != nil {
		t.Fatal(err)
	}

	a := adapter.NewClaudeCodeAdapter("")
	if err := a.InstallMCP(testMCP, baseDir, map[string]string{"API_KEY": "secret"}); err != nil {
		t.Fatalf("InstallMCP() error = %v", err)
	}

	got, err := a.ScanMCP(baseDir)
	if err != nil {
		t.Fatalf("ScanMCP() error = %v", err)
	}
	if len(got) != 1 || got[0].ServerName != "test-server" {
		t.Fatalf("ScanMCP() after InstallMCP = %v, want one entry named test-server", got)
	}
}

func TestCursorAdapter_InstallMCP_CreatesMCPJSON(t *testing.T) {
	home := t.TempDir()
	baseDir := filepath.Join(home, ".cursor")
	if err := os.Mkdir(baseDir, 0755); err != nil {
		t.Fatal(err)
	}

	a := adapter.NewCursorAdapter("")
	mcpItem := mcp.MCP{
		Name:    "cursor-server",
		Command: "node",
		Args:    []string{"server.js"},
		Targets: []string{"cursor"},
		Env:     []mcp.EnvVar{},
	}
	if err := a.InstallMCP(mcpItem, baseDir, nil); err != nil {
		t.Fatalf("InstallMCP() error = %v", err)
	}

	cfg := readJSONFile(t, filepath.Join(baseDir, "mcp.json"))
	servers := cfg["mcpServers"].(map[string]interface{})
	entry := servers["cursor-server"].(map[string]interface{})
	if entry["command"] != "node" {
		t.Errorf("command = %v, want node", entry["command"])
	}
}

func TestCursorAdapter_InstallMCP_EmptyMCPJSON(t *testing.T) {
	// Cursor creates mcp.json as an empty file before the user configures anything.
	// installMCPJSON must tolerate this instead of failing with "unexpected end of JSON input".
	home := t.TempDir()
	baseDir := filepath.Join(home, ".cursor")
	if err := os.Mkdir(baseDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "mcp.json"), []byte{}, 0644); err != nil {
		t.Fatal(err)
	}

	a := adapter.NewCursorAdapter("")
	mcpItem := mcp.MCP{
		Name:    "my-server",
		Command: "node",
		Args:    []string{"srv.js"},
		Targets: []string{"cursor"},
		Env:     []mcp.EnvVar{},
	}
	if err := a.InstallMCP(mcpItem, baseDir, nil); err != nil {
		t.Fatalf("InstallMCP() on empty mcp.json: %v", err)
	}

	cfg := readJSONFile(t, filepath.Join(baseDir, "mcp.json"))
	servers := cfg["mcpServers"].(map[string]interface{})
	if _, ok := servers["my-server"]; !ok {
		t.Error("my-server not written to mcp.json")
	}
}

func TestRegistry(t *testing.T) {
	adapters := adapter.Registry()
	if len(adapters) == 0 {
		t.Fatal("Registry() returned empty slice")
	}
	found := false
	for _, a := range adapters {
		if a.Name() == "claude-code" {
			found = true
		}
	}
	if !found {
		t.Error("Registry() does not include claude-code adapter")
	}
}

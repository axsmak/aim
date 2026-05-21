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

func TestClaudeCodeAdapter_InstallMCP_CreatesSettingsJSON(t *testing.T) {
	home := t.TempDir()
	baseDir := filepath.Join(home, ".claude")
	if err := os.Mkdir(baseDir, 0755); err != nil {
		t.Fatal(err)
	}

	a := adapter.NewClaudeCodeAdapter("")
	if err := a.InstallMCP(testMCP, baseDir, map[string]string{"API_KEY": "secret"}); err != nil {
		t.Fatalf("InstallMCP() error = %v", err)
	}

	cfg := readJSONFile(t, filepath.Join(baseDir, "settings.json"))
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
	if err := os.WriteFile(filepath.Join(baseDir, "settings.json"), []byte(existing), 0644); err != nil {
		t.Fatal(err)
	}

	a := adapter.NewClaudeCodeAdapter("")
	if err := a.InstallMCP(testMCP, baseDir, nil); err != nil {
		t.Fatalf("InstallMCP() error = %v", err)
	}

	cfg := readJSONFile(t, filepath.Join(baseDir, "settings.json"))
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

	cfg := readJSONFile(t, filepath.Join(baseDir, "settings.json"))
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

	cfg := readJSONFile(t, filepath.Join(baseDir, "settings.json"))
	servers := cfg["mcpServers"].(map[string]interface{})
	entry := servers["test-server"].(map[string]interface{})
	if _, ok := entry["env"]; ok {
		t.Error("env section should be absent when envValues is nil")
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

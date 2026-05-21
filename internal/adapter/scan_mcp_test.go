package adapter_test

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/axsmak/aim/internal/adapter"
)

// helper: sort DiscoveredMCP slice by ServerName for deterministic assertions.
func sortByName(s []adapter.DiscoveredMCP) {
	sort.Slice(s, func(i, j int) bool { return s[i].ServerName < s[j].ServerName })
}

// ─── ClaudeCodeAdapter.ScanMCP ───────────────────────────────────────────────

func TestClaudeCodeAdapter_ScanMCP_TwoServers(t *testing.T) {
	baseDir := t.TempDir()
	configPath := filepath.Join(baseDir, "settings.json")
	content := `{
		"mcpServers": {
			"alpha": {"command": "node", "args": ["alpha.js"], "env": {"KEY": "val"}},
			"beta":  {"command": "python", "args": ["-m", "beta"]}
		}
	}`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	a := adapter.NewClaudeCodeAdapter("")
	got, err := a.ScanMCP(baseDir)
	if err != nil {
		t.Fatalf("ScanMCP() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ScanMCP() returned %d entries, want 2", len(got))
	}
	sortByName(got)

	if got[0].ServerName != "alpha" {
		t.Errorf("got[0].ServerName = %q, want alpha", got[0].ServerName)
	}
	if got[0].Source != "claude-code" {
		t.Errorf("got[0].Source = %q, want claude-code", got[0].Source)
	}
	if got[0].Command != "node" {
		t.Errorf("got[0].Command = %q, want node", got[0].Command)
	}
	if len(got[0].Args) != 1 || got[0].Args[0] != "alpha.js" {
		t.Errorf("got[0].Args = %v, want [alpha.js]", got[0].Args)
	}
	if got[0].Env["KEY"] != "val" {
		t.Errorf("got[0].Env[KEY] = %q, want val", got[0].Env["KEY"])
	}

	if got[1].ServerName != "beta" {
		t.Errorf("got[1].ServerName = %q, want beta", got[1].ServerName)
	}
	if got[1].Env != nil {
		t.Errorf("got[1].Env = %v, want nil", got[1].Env)
	}
}

func TestClaudeCodeAdapter_ScanMCP_NoMCPServersKey(t *testing.T) {
	baseDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(baseDir, "settings.json"), []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}

	a := adapter.NewClaudeCodeAdapter("")
	got, err := a.ScanMCP(baseDir)
	if err != nil {
		t.Fatalf("ScanMCP() error = %v", err)
	}
	if got != nil {
		t.Errorf("ScanMCP() = %v, want nil", got)
	}
}

func TestClaudeCodeAdapter_ScanMCP_MissingFile(t *testing.T) {
	baseDir := t.TempDir()

	a := adapter.NewClaudeCodeAdapter("")
	got, err := a.ScanMCP(baseDir)
	if err != nil {
		t.Fatalf("ScanMCP() error = %v, want nil", err)
	}
	if got != nil {
		t.Errorf("ScanMCP() = %v, want nil", got)
	}
}

func TestClaudeCodeAdapter_ScanMCP_MalformedJSON(t *testing.T) {
	baseDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(baseDir, "settings.json"), []byte(`{bad json`), 0644); err != nil {
		t.Fatal(err)
	}

	a := adapter.NewClaudeCodeAdapter("")
	_, err := a.ScanMCP(baseDir)
	if err == nil {
		t.Fatal("ScanMCP() error = nil, want non-nil for malformed JSON")
	}
}

// ─── CursorAdapter.ScanMCP ───────────────────────────────────────────────────

func TestCursorAdapter_ScanMCP_TwoServers(t *testing.T) {
	baseDir := t.TempDir()
	configPath := filepath.Join(baseDir, "mcp.json")
	content := `{
		"mcpServers": {
			"srv1": {"command": "go", "args": ["run", "main.go"], "env": {"TOKEN": "abc"}},
			"srv2": {"command": "deno"}
		}
	}`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	a := adapter.NewCursorAdapter("")
	got, err := a.ScanMCP(baseDir)
	if err != nil {
		t.Fatalf("ScanMCP() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ScanMCP() returned %d entries, want 2", len(got))
	}
	sortByName(got)

	if got[0].ServerName != "srv1" {
		t.Errorf("got[0].ServerName = %q, want srv1", got[0].ServerName)
	}
	if got[0].Source != "cursor" {
		t.Errorf("got[0].Source = %q, want cursor", got[0].Source)
	}
	if got[0].Command != "go" {
		t.Errorf("got[0].Command = %q, want go", got[0].Command)
	}
	if len(got[0].Args) != 2 {
		t.Errorf("got[0].Args = %v, want [run main.go]", got[0].Args)
	}
	if got[0].Env["TOKEN"] != "abc" {
		t.Errorf("got[0].Env[TOKEN] = %q, want abc", got[0].Env["TOKEN"])
	}

	if got[1].ServerName != "srv2" {
		t.Errorf("got[1].ServerName = %q, want srv2", got[1].ServerName)
	}
}

func TestCursorAdapter_ScanMCP_NoMCPServersKey(t *testing.T) {
	baseDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(baseDir, "mcp.json"), []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}

	a := adapter.NewCursorAdapter("")
	got, err := a.ScanMCP(baseDir)
	if err != nil {
		t.Fatalf("ScanMCP() error = %v", err)
	}
	if got != nil {
		t.Errorf("ScanMCP() = %v, want nil", got)
	}
}

func TestCursorAdapter_ScanMCP_MissingFile(t *testing.T) {
	baseDir := t.TempDir()

	a := adapter.NewCursorAdapter("")
	got, err := a.ScanMCP(baseDir)
	if err != nil {
		t.Fatalf("ScanMCP() error = %v, want nil", err)
	}
	if got != nil {
		t.Errorf("ScanMCP() = %v, want nil", got)
	}
}

func TestCursorAdapter_ScanMCP_MalformedJSON(t *testing.T) {
	baseDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(baseDir, "mcp.json"), []byte(`not json`), 0644); err != nil {
		t.Fatal(err)
	}

	a := adapter.NewCursorAdapter("")
	_, err := a.ScanMCP(baseDir)
	if err == nil {
		t.Fatal("ScanMCP() error = nil, want non-nil for malformed JSON")
	}
}

// ─── CodexAdapter.ScanMCP ────────────────────────────────────────────────────

func TestCodexAdapter_ScanMCP_TwoServers(t *testing.T) {
	baseDir := t.TempDir()
	configPath := filepath.Join(baseDir, "config.toml")
	content := `
[mcp_servers.srvA]
command = "uvx"
args    = ["mcp-a"]
[mcp_servers.srvA.env]
API_KEY = "secret"

[mcp_servers.srvB]
command = "node"
args    = ["b.js"]
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	a := adapter.NewCodexAdapter("")
	got, err := a.ScanMCP(baseDir)
	if err != nil {
		t.Fatalf("ScanMCP() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ScanMCP() returned %d entries, want 2", len(got))
	}
	sortByName(got)

	if got[0].ServerName != "srvA" {
		t.Errorf("got[0].ServerName = %q, want srvA", got[0].ServerName)
	}
	if got[0].Source != "codex" {
		t.Errorf("got[0].Source = %q, want codex", got[0].Source)
	}
	if got[0].Command != "uvx" {
		t.Errorf("got[0].Command = %q, want uvx", got[0].Command)
	}
	if len(got[0].Args) != 1 || got[0].Args[0] != "mcp-a" {
		t.Errorf("got[0].Args = %v, want [mcp-a]", got[0].Args)
	}
	if got[0].Env["API_KEY"] != "secret" {
		t.Errorf("got[0].Env[API_KEY] = %q, want secret", got[0].Env["API_KEY"])
	}

	if got[1].ServerName != "srvB" {
		t.Errorf("got[1].ServerName = %q, want srvB", got[1].ServerName)
	}
	if got[1].Env != nil {
		t.Errorf("got[1].Env = %v, want nil", got[1].Env)
	}
}

func TestCodexAdapter_ScanMCP_NoMCPServersKey(t *testing.T) {
	baseDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(baseDir, "config.toml"), []byte(`title = "codex"`), 0644); err != nil {
		t.Fatal(err)
	}

	a := adapter.NewCodexAdapter("")
	got, err := a.ScanMCP(baseDir)
	if err != nil {
		t.Fatalf("ScanMCP() error = %v", err)
	}
	if got != nil {
		t.Errorf("ScanMCP() = %v, want nil", got)
	}
}

func TestCodexAdapter_ScanMCP_MissingFile(t *testing.T) {
	baseDir := t.TempDir()

	a := adapter.NewCodexAdapter("")
	got, err := a.ScanMCP(baseDir)
	if err != nil {
		t.Fatalf("ScanMCP() error = %v, want nil", err)
	}
	if got != nil {
		t.Errorf("ScanMCP() = %v, want nil", got)
	}
}

func TestCodexAdapter_ScanMCP_MalformedTOML(t *testing.T) {
	baseDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(baseDir, "config.toml"), []byte(`[bad toml`), 0644); err != nil {
		t.Fatal(err)
	}

	a := adapter.NewCodexAdapter("")
	_, err := a.ScanMCP(baseDir)
	if err == nil {
		t.Fatal("ScanMCP() error = nil, want non-nil for malformed TOML")
	}
}

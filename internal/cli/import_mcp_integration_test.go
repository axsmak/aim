package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/axsmak/aim/internal/localconfig"
	"gopkg.in/yaml.v3"
)

// writeCursorMCPConfig writes a Cursor mcp.json to <fakeHome>/.cursor/mcp.json.
func writeCursorMCPConfig(t *testing.T, fakeHome string, servers map[string]interface{}) {
	t.Helper()
	dir := filepath.Join(fakeHome, ".cursor")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir .cursor: %v", err)
	}
	data, err := json.MarshalIndent(map[string]interface{}{"mcpServers": servers}, "", "  ")
	if err != nil {
		t.Fatalf("marshal cursor mcp.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mcp.json"), data, 0644); err != nil {
		t.Fatalf("write mcp.json: %v", err)
	}
}

// writeClaudeCodeMCPConfig writes a Claude Code settings.json to <fakeHome>/.claude/settings.json.
func writeClaudeCodeMCPConfig(t *testing.T, fakeHome string, servers map[string]interface{}) {
	t.Helper()
	dir := filepath.Join(fakeHome, ".claude")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}
	data, err := json.MarshalIndent(map[string]interface{}{"mcpServers": servers}, "", "  ")
	if err != nil {
		t.Fatalf("marshal claude settings.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), data, 0644); err != nil {
		t.Fatalf("write settings.json: %v", err)
	}
}

// jiraServerEntry returns a standard jira MCP server entry for test fixtures.
func jiraServerEntry() map[string]interface{} {
	return map[string]interface{}{
		"command": "npx",
		"args":    []interface{}{"-y", "@modelcontextprotocol/server-jira"},
		"env": map[string]interface{}{
			"JIRA_API_TOKEN": "secret-token",
			"JIRA_BASE_URL":  "https://mycompany.atlassian.net",
		},
	}
}

// TestImportMCP_HappyPath verifies the full import flow for a cursor-sourced MCP server.
func TestImportMCP_HappyPath(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()

	writeCursorMCPConfig(t, fakeHome, map[string]interface{}{
		"jira": jiraServerEntry(),
	})

	_, _, err := runImportCmd(t, fakeHome, workDir, "mcp", "jira", "--from", "cursor")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// mcp/jira.yaml must exist.
	destPath := filepath.Join(workDir, "mcp", "jira.yaml")
	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("mcp/jira.yaml not written: %v", err)
	}

	// Env values must NOT be in the stored descriptor.
	if bytes.Contains(data, []byte("secret-token")) {
		t.Error("mcp/jira.yaml must not contain real env values (secret-token)")
	}
	if bytes.Contains(data, []byte("mycompany")) {
		t.Error("mcp/jira.yaml must not contain real env values (mycompany)")
	}

	// Env descriptors must have required:true and empty values.
	var stored struct {
		Env []struct {
			Name     string `yaml:"name"`
			Required bool   `yaml:"required"`
			Value    string `yaml:"value"`
		} `yaml:"env"`
	}
	if err := yaml.Unmarshal(data, &stored); err != nil {
		t.Fatalf("parse mcp/jira.yaml: %v", err)
	}
	if len(stored.Env) != 2 {
		t.Fatalf("expected 2 env entries, got %d", len(stored.Env))
	}
	for _, ev := range stored.Env {
		if !ev.Required {
			t.Errorf("env entry %q: required must be true", ev.Name)
		}
		if ev.Value != "" {
			t.Errorf("env entry %q: value must be empty in descriptor, got %q", ev.Name, ev.Value)
		}
	}

	// Real values must be in aim.local.yaml.
	cfg, err := localconfig.Load(workDir)
	if err != nil {
		t.Fatalf("load aim.local.yaml: %v", err)
	}
	if cfg.MCPEnv["jira.JIRA_API_TOKEN"] != "secret-token" {
		t.Errorf("aim.local.yaml: jira.JIRA_API_TOKEN = %q, want %q", cfg.MCPEnv["jira.JIRA_API_TOKEN"], "secret-token")
	}
	if cfg.MCPEnv["jira.JIRA_BASE_URL"] != "https://mycompany.atlassian.net" {
		t.Errorf("aim.local.yaml: jira.JIRA_BASE_URL = %q, want %q", cfg.MCPEnv["jira.JIRA_BASE_URL"], "https://mycompany.atlassian.net")
	}
}

// TestImportMCP_DryRun verifies that --print prints YAML to stdout without writing files.
func TestImportMCP_DryRun(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()

	writeCursorMCPConfig(t, fakeHome, map[string]interface{}{
		"jira": jiraServerEntry(),
	})

	stdout, _, err := runImportCmd(t, fakeHome, workDir, "mcp", "jira", "--from", "cursor", "--print")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// stdout must contain YAML content.
	if !strings.Contains(stdout, "jira") {
		t.Errorf("dry-run stdout must contain server name, got: %q", stdout)
	}
	if !strings.Contains(stdout, "npx") {
		t.Errorf("dry-run stdout must contain command, got: %q", stdout)
	}

	// mcp/jira.yaml must NOT be written.
	destPath := filepath.Join(workDir, "mcp", "jira.yaml")
	if _, statErr := os.Stat(destPath); !os.IsNotExist(statErr) {
		t.Error("dry-run must not write mcp/jira.yaml to disk")
	}

	// aim.local.yaml must NOT be written.
	localPath := filepath.Join(workDir, "aim.local.yaml")
	if _, statErr := os.Stat(localPath); !os.IsNotExist(statErr) {
		t.Error("dry-run must not write aim.local.yaml to disk")
	}
}

// TestImportMCP_TargetsAll verifies that --targets all sets all adapter names in the descriptor.
func TestImportMCP_TargetsAll(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()

	writeCursorMCPConfig(t, fakeHome, map[string]interface{}{
		"jira": jiraServerEntry(),
	})

	_, _, err := runImportCmd(t, fakeHome, workDir, "mcp", "jira", "--from", "cursor", "--targets", "all")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(workDir, "mcp", "jira.yaml"))
	if err != nil {
		t.Fatalf("mcp/jira.yaml not written: %v", err)
	}

	var stored struct {
		Targets []string `yaml:"targets"`
	}
	if err := yaml.Unmarshal(data, &stored); err != nil {
		t.Fatalf("parse mcp/jira.yaml: %v", err)
	}

	wantTargets := map[string]bool{"claude-code": true, "cursor": true, "codex": true}
	if len(stored.Targets) != 3 {
		t.Fatalf("--targets all: expected 3 targets, got %d: %v", len(stored.Targets), stored.Targets)
	}
	for _, tgt := range stored.Targets {
		if !wantTargets[tgt] {
			t.Errorf("--targets all: unexpected target %q", tgt)
		}
	}
}

// TestImportMCP_NotFound verifies an error when the server name does not exist in the adapter.
func TestImportMCP_NotFound(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()

	// Cursor config exists but has no "nonexistent" server.
	writeCursorMCPConfig(t, fakeHome, map[string]interface{}{
		"jira": jiraServerEntry(),
	})

	_, _, err := runImportCmd(t, fakeHome, workDir, "mcp", "nonexistent", "--from", "cursor")
	if err == nil {
		t.Fatal("expected error for unknown server name, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

// TestImportMCP_ConflictNoOverwrite verifies ConflictError when mcp/jira.yaml already exists with different content.
func TestImportMCP_ConflictNoOverwrite(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()

	writeCursorMCPConfig(t, fakeHome, map[string]interface{}{
		"jira": jiraServerEntry(),
	})

	// First import: write the file.
	if _, _, err := runImportCmd(t, fakeHome, workDir, "mcp", "jira", "--from", "cursor"); err != nil {
		t.Fatalf("first import: %v", err)
	}

	// Now write a different entry (different command) to force a conflict.
	writeCursorMCPConfig(t, fakeHome, map[string]interface{}{
		"jira": map[string]interface{}{
			"command": "uvx",
			"args":    []interface{}{"jira-mcp"},
		},
	})

	_, _, err := runImportCmd(t, fakeHome, workDir, "mcp", "jira", "--from", "cursor")
	if err == nil {
		t.Fatal("expected ConflictError, got nil")
	}
	if !strings.Contains(err.Error(), "--overwrite") {
		t.Errorf("conflict error must mention --overwrite, got: %v", err)
	}
}

// TestImportMCP_ConflictWithOverwrite verifies that --overwrite replaces the existing file.
func TestImportMCP_ConflictWithOverwrite(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()

	writeCursorMCPConfig(t, fakeHome, map[string]interface{}{
		"jira": jiraServerEntry(),
	})

	// First import.
	if _, _, err := runImportCmd(t, fakeHome, workDir, "mcp", "jira", "--from", "cursor"); err != nil {
		t.Fatalf("first import: %v", err)
	}

	// Change the entry.
	writeCursorMCPConfig(t, fakeHome, map[string]interface{}{
		"jira": map[string]interface{}{
			"command": "uvx",
			"args":    []interface{}{"jira-mcp"},
		},
	})

	_, _, err := runImportCmd(t, fakeHome, workDir, "mcp", "jira", "--from", "cursor", "--overwrite")
	if err != nil {
		t.Fatalf("--overwrite should succeed, got: %v", err)
	}

	// The stored file must reflect the new command.
	data, err := os.ReadFile(filepath.Join(workDir, "mcp", "jira.yaml"))
	if err != nil {
		t.Fatalf("read mcp/jira.yaml after overwrite: %v", err)
	}
	if !bytes.Contains(data, []byte("uvx")) {
		t.Error("mcp/jira.yaml after --overwrite must contain new command 'uvx'")
	}
}

// TestImportMCP_UnknownAdapter verifies an error when --from specifies an unknown environment.
func TestImportMCP_UnknownAdapter(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()

	_, _, err := runImportCmd(t, fakeHome, workDir, "mcp", "jira", "--from", "unknown-env")
	if err == nil {
		t.Fatal("expected error for unknown environment, got nil")
	}
	if !strings.Contains(err.Error(), "unknown environment") {
		t.Errorf("expected 'unknown environment' error, got: %v", err)
	}
}

// TestImportMCP_DefaultTarget verifies that without --targets all, the target is set to the source adapter name.
func TestImportMCP_DefaultTarget(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()

	writeCursorMCPConfig(t, fakeHome, map[string]interface{}{
		"jira": jiraServerEntry(),
	})

	_, _, err := runImportCmd(t, fakeHome, workDir, "mcp", "jira", "--from", "cursor")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(workDir, "mcp", "jira.yaml"))
	if err != nil {
		t.Fatalf("mcp/jira.yaml not written: %v", err)
	}

	var stored struct {
		Targets []string `yaml:"targets"`
	}
	if err := yaml.Unmarshal(data, &stored); err != nil {
		t.Fatalf("parse mcp/jira.yaml: %v", err)
	}
	if len(stored.Targets) != 1 || stored.Targets[0] != "cursor" {
		t.Errorf("expected targets=[cursor], got %v", stored.Targets)
	}
}

// TestImportMCP_ClaudeCodeAdapter verifies import from claude-code adapter reads settings.json.
func TestImportMCP_ClaudeCodeAdapter(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()

	writeClaudeCodeMCPConfig(t, fakeHome, map[string]interface{}{
		"jira": jiraServerEntry(),
	})

	_, _, err := runImportCmd(t, fakeHome, workDir, "mcp", "jira", "--from", "claude-code")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	destPath := filepath.Join(workDir, "mcp", "jira.yaml")
	if _, statErr := os.Stat(destPath); statErr != nil {
		t.Fatalf("mcp/jira.yaml not written for claude-code: %v", statErr)
	}
}

// TestImportMCP_NoEnvVars verifies that when MCP server has no env vars, aim.local.yaml is not created.
func TestImportMCP_NoEnvVars(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()

	writeCursorMCPConfig(t, fakeHome, map[string]interface{}{
		"simple-server": map[string]interface{}{
			"command": "node",
			"args":    []interface{}{"./server.js"},
		},
	})

	_, _, err := runImportCmd(t, fakeHome, workDir, "mcp", "simple-server", "--from", "cursor")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	localPath := filepath.Join(workDir, "aim.local.yaml")
	if _, statErr := os.Stat(localPath); !os.IsNotExist(statErr) {
		t.Error("aim.local.yaml must not be created when MCP server has no env vars")
	}
}

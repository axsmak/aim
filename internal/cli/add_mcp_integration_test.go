package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/axsmak/aim/internal/localconfig"
	"gopkg.in/yaml.v3"
)

const mcpFixtureNoEnv = `name: jira
description: Jira MCP server
command: npx
args:
    - -y
    - mcp-jira
targets:
    - claude_code
env: []
`

const mcpFixtureWithEnv = `name: jira
description: Jira MCP server
command: npx
args:
    - -y
    - mcp-jira
targets:
    - claude_code
env:
    - name: JIRA_API_KEY
      description: Jira API key
      required: true
      value: "secret123"
    - name: JIRA_URL
      description: Jira instance URL
      required: true
      value: "https://mycompany.atlassian.net"
`

func writeMCPFixture(t *testing.T, content string) string {
	t.Helper()
	f := filepath.Join(t.TempDir(), "jira.yaml")
	if err := os.WriteFile(f, []byte(content), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return f
}

// TestAddMCP_WritesDescriptorFile verifies that mcp/<name>.yaml is created.
func TestAddMCP_WritesDescriptorFile(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()
	src := writeMCPFixture(t, mcpFixtureNoEnv)

	_, _, err := runAddCmd(t, fakeHome, workDir, "mcp", src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	dest := filepath.Join(workDir, "mcp", "jira.yaml")
	if _, statErr := os.Stat(dest); statErr != nil {
		t.Fatalf("mcp/jira.yaml not written: %v", statErr)
	}
}

// TestAddMCP_EnvStrip_ValuesNotInYAML is the core security invariant test:
// real env values must not appear in the stored mcp/<name>.yaml.
func TestAddMCP_EnvStrip_ValuesNotInYAML(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()
	src := writeMCPFixture(t, mcpFixtureWithEnv)

	_, _, err := runAddCmd(t, fakeHome, workDir, "mcp", src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(workDir, "mcp", "jira.yaml"))
	if err != nil {
		t.Fatalf("read mcp/jira.yaml: %v", err)
	}
	if bytes.Contains(data, []byte("secret123")) {
		t.Fatal("env value 'secret123' must not appear in mcp/jira.yaml")
	}
	if bytes.Contains(data, []byte("mycompany")) {
		t.Fatal("env value 'mycompany.atlassian.net' must not appear in mcp/jira.yaml")
	}
}

// TestAddMCP_EnvStrip_ValuesInLocalConfig verifies real values are stored in aim.local.yaml.
func TestAddMCP_EnvStrip_ValuesInLocalConfig(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()
	src := writeMCPFixture(t, mcpFixtureWithEnv)

	_, _, err := runAddCmd(t, fakeHome, workDir, "mcp", src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg, err := localconfig.Load(workDir)
	if err != nil {
		t.Fatalf("load aim.local.yaml: %v", err)
	}
	if cfg.MCPEnv["jira.JIRA_API_KEY"] != "secret123" {
		t.Errorf("aim.local.yaml: jira.JIRA_API_KEY = %q, want %q", cfg.MCPEnv["jira.JIRA_API_KEY"], "secret123")
	}
	if cfg.MCPEnv["jira.JIRA_URL"] != "https://mycompany.atlassian.net" {
		t.Errorf("aim.local.yaml: jira.JIRA_URL = %q, want %q", cfg.MCPEnv["jira.JIRA_URL"], "https://mycompany.atlassian.net")
	}
}

// TestAddMCP_NoEnvValues_NoLocalConfigCreated verifies no aim.local.yaml is created
// when there are no env values to strip.
func TestAddMCP_NoEnvValues_NoLocalConfigCreated(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()
	src := writeMCPFixture(t, mcpFixtureNoEnv)

	_, _, err := runAddCmd(t, fakeHome, workDir, "mcp", src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	localPath := filepath.Join(workDir, "aim.local.yaml")
	if _, statErr := os.Stat(localPath); statErr == nil {
		t.Fatal("aim.local.yaml must not be created when there are no env values")
	}
}

// TestAddMCP_DescriptorFieldsPreserved verifies that name, description, required
// are kept in the stored file.
func TestAddMCP_DescriptorFieldsPreserved(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()
	src := writeMCPFixture(t, mcpFixtureWithEnv)

	_, _, err := runAddCmd(t, fakeHome, workDir, "mcp", src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(workDir, "mcp", "jira.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	var stored struct {
		Env []struct {
			Name     string `yaml:"name"`
			Required bool   `yaml:"required"`
			Value    string `yaml:"value"`
		} `yaml:"env"`
	}
	if err := yaml.Unmarshal(data, &stored); err != nil {
		t.Fatalf("parse stored yaml: %v", err)
	}

	if len(stored.Env) != 2 {
		t.Fatalf("expected 2 env entries, got %d", len(stored.Env))
	}
	if stored.Env[0].Name != "JIRA_API_KEY" {
		t.Errorf("env[0].name = %q, want JIRA_API_KEY", stored.Env[0].Name)
	}
	if !stored.Env[0].Required {
		t.Error("env[0].required must be true")
	}
	if stored.Env[0].Value != "" {
		t.Errorf("env[0].value must be empty, got %q", stored.Env[0].Value)
	}
}

// TestAddMCP_Duplicate_NoOp verifies that adding the exact same content twice is a no-op (no error).
func TestAddMCP_Duplicate_NoOp(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()
	src := writeMCPFixture(t, mcpFixtureNoEnv)

	if _, _, err := runAddCmd(t, fakeHome, workDir, "mcp", src); err != nil {
		t.Fatalf("first add: %v", err)
	}
	if _, _, err := runAddCmd(t, fakeHome, workDir, "mcp", src); err != nil {
		t.Fatalf("second add (duplicate): %v", err)
	}
}

// TestAddMCP_ConflictNoOverwrite_Error verifies ConflictError when content differs.
func TestAddMCP_ConflictNoOverwrite_Error(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()

	// Add first version.
	src1 := writeMCPFixture(t, mcpFixtureNoEnv)
	if _, _, err := runAddCmd(t, fakeHome, workDir, "mcp", src1); err != nil {
		t.Fatalf("first add: %v", err)
	}

	// Add modified version without --overwrite.
	modified := strings.ReplaceAll(mcpFixtureNoEnv, "Jira MCP server", "Modified description")
	src2 := filepath.Join(t.TempDir(), "jira.yaml")
	if err := os.WriteFile(src2, []byte(modified), 0644); err != nil {
		t.Fatal(err)
	}

	_, _, err := runAddCmd(t, fakeHome, workDir, "mcp", src2)
	if err == nil {
		t.Fatal("expected ConflictError, got nil")
	}
	if !strings.Contains(err.Error(), "--overwrite") {
		t.Errorf("error must mention --overwrite, got: %v", err)
	}
}

// TestAddMCP_ConflictWithOverwrite_Succeeds verifies --overwrite replaces the file.
func TestAddMCP_ConflictWithOverwrite_Succeeds(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()

	src1 := writeMCPFixture(t, mcpFixtureNoEnv)
	if _, _, err := runAddCmd(t, fakeHome, workDir, "mcp", src1); err != nil {
		t.Fatalf("first add: %v", err)
	}

	modified := strings.ReplaceAll(mcpFixtureNoEnv, "Jira MCP server", "Modified description")
	src2 := filepath.Join(t.TempDir(), "jira.yaml")
	if err := os.WriteFile(src2, []byte(modified), 0644); err != nil {
		t.Fatal(err)
	}

	_, _, err := runAddCmd(t, fakeHome, workDir, "mcp", src2, "--overwrite")
	if err != nil {
		t.Fatalf("unexpected error with --overwrite: %v", err)
	}
}

// TestAddMCP_NameFlag_UsesOverrideName verifies --name stores as mcp/<name>.yaml.
func TestAddMCP_NameFlag_UsesOverrideName(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()
	src := writeMCPFixture(t, mcpFixtureNoEnv)

	_, _, err := runAddCmd(t, fakeHome, workDir, "mcp", src, "--name", "my-jira")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, statErr := os.Stat(filepath.Join(workDir, "mcp", "my-jira.yaml")); statErr != nil {
		t.Fatalf("mcp/my-jira.yaml not written: %v", statErr)
	}
}

// TestAddMCP_Stdin reads from stdin ("-").
func TestAddMCP_Stdin(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()

	// Write fixture to a temp file, then use it as stdin substitute via a named pipe.
	// Since runAddCmd uses Cobra and our CLI opens os.Stdin for "-", we need to redirect.
	// We use a real file piped via os.Stdin replacement.
	stdinFile := filepath.Join(t.TempDir(), "stdin.yaml")
	if err := os.WriteFile(stdinFile, []byte(mcpFixtureNoEnv), 0644); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(stdinFile)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	oldStdin := os.Stdin
	os.Stdin = f
	t.Cleanup(func() { os.Stdin = oldStdin })

	_, _, err = runAddCmd(t, fakeHome, workDir, "mcp", "-")
	if err != nil {
		t.Fatalf("unexpected error reading from stdin: %v", err)
	}

	if _, statErr := os.Stat(filepath.Join(workDir, "mcp", "jira.yaml")); statErr != nil {
		t.Fatalf("mcp/jira.yaml not written from stdin: %v", statErr)
	}
}

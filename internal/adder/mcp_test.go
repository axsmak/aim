package adder

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/axsmak/aim/internal/importer"
	"gopkg.in/yaml.v3"
)

const validMCPNoEnvValues = `name: jira
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
`

const validMCPWithEnvValues = `name: jira
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

func TestAddMCP_NoEnvValues_WritesCleanYAML(t *testing.T) {
	dir := t.TempDir()
	_, err := addMCP([]byte(validMCPNoEnvValues), AddOptions{WorkDir: dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	dest := filepath.Join(dir, "mcp", "jira.yaml")
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("mcp/jira.yaml not written: %v", err)
	}
}

func TestAddMCP_WithEnvValues_StripFromYAML(t *testing.T) {
	dir := t.TempDir()
	_, err := addMCP([]byte(validMCPWithEnvValues), AddOptions{WorkDir: dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "mcp", "jira.yaml"))
	if err != nil {
		t.Fatalf("mcp/jira.yaml not written: %v", err)
	}

	// Value must not appear in the stored file.
	if bytes.Contains(data, []byte("secret123")) {
		t.Fatal("env value 'secret123' must not appear in mcp/jira.yaml")
	}
	if bytes.Contains(data, []byte("mycompany")) {
		t.Fatal("env value 'mycompany.atlassian.net' must not appear in mcp/jira.yaml")
	}
}

func TestAddMCP_WithEnvValues_StoredInLocalConfig(t *testing.T) {
	dir := t.TempDir()
	_, err := addMCP([]byte(validMCPWithEnvValues), AddOptions{WorkDir: dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	localData, err := os.ReadFile(filepath.Join(dir, "aim.local.yaml"))
	if err != nil {
		t.Fatalf("aim.local.yaml not written: %v", err)
	}
	if !bytes.Contains(localData, []byte("secret123")) {
		t.Fatal("aim.local.yaml must contain env value 'secret123'")
	}
	if !bytes.Contains(localData, []byte("jira.JIRA_API_KEY")) {
		t.Fatal("aim.local.yaml must use key 'jira.JIRA_API_KEY'")
	}
}

func TestAddMCP_DescriptorFieldsPreserved(t *testing.T) {
	dir := t.TempDir()
	_, err := addMCP([]byte(validMCPWithEnvValues), AddOptions{WorkDir: dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "mcp", "jira.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	var result struct {
		Env []struct {
			Name        string `yaml:"name"`
			Description string `yaml:"description"`
			Required    bool   `yaml:"required"`
			Value       string `yaml:"value"`
		} `yaml:"env"`
	}
	if err := yaml.Unmarshal(data, &result); err != nil {
		t.Fatalf("cannot parse written yaml: %v", err)
	}
	if len(result.Env) == 0 {
		t.Fatal("env section must be preserved")
	}
	if result.Env[0].Name != "JIRA_API_KEY" {
		t.Fatalf("env[0].name mismatch: got %q", result.Env[0].Name)
	}
	if !result.Env[0].Required {
		t.Fatal("env[0].required must be true")
	}
	if result.Env[0].Value != "" {
		t.Fatalf("env[0].value must be empty in stored file, got %q", result.Env[0].Value)
	}
}

func TestAddMCP_InvalidYAML_Error(t *testing.T) {
	_, err := addMCP([]byte("!!invalid: {"), AddOptions{WorkDir: t.TempDir()})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestAddMCP_ConflictNoOverwrite_ConflictError(t *testing.T) {
	dir := t.TempDir()
	mcpDir := filepath.Join(dir, "mcp")
	if err := os.MkdirAll(mcpDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mcpDir, "jira.yaml"), []byte("different content"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := addMCP([]byte(validMCPNoEnvValues), AddOptions{WorkDir: dir})
	if err == nil {
		t.Fatal("expected ConflictError, got nil")
	}
	var ce importer.ConflictError
	if !errors.As(err, &ce) {
		t.Fatalf("expected ConflictError, got %T: %v", err, err)
	}
}

func TestAddMCP_ConflictWithOverwrite_Succeeds(t *testing.T) {
	dir := t.TempDir()
	mcpDir := filepath.Join(dir, "mcp")
	if err := os.MkdirAll(mcpDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mcpDir, "jira.yaml"), []byte("different content"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := addMCP([]byte(validMCPNoEnvValues), AddOptions{WorkDir: dir, Overwrite: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAddMCP_NameOverride(t *testing.T) {
	dir := t.TempDir()
	_, err := addMCP([]byte(validMCPNoEnvValues), AddOptions{WorkDir: dir, Name: "custom-name"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "mcp", "custom-name.yaml")); err != nil {
		t.Fatalf("mcp/custom-name.yaml not written: %v", err)
	}
}

func TestAddMCP_HasSecrets_ResultFlag(t *testing.T) {
	dir := t.TempDir()
	result, err := addMCP([]byte(validMCPWithEnvValues), AddOptions{WorkDir: dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.HasSecrets {
		t.Fatal("expected HasSecrets=true when env values are present")
	}
}

func TestAddMCP_NoSecrets_ResultFlag(t *testing.T) {
	dir := t.TempDir()
	result, err := addMCP([]byte(validMCPNoEnvValues), AddOptions{WorkDir: dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.HasSecrets {
		t.Fatal("expected HasSecrets=false when no env values")
	}
}

func TestAddMCP_Identical_NoOp(t *testing.T) {
	dir := t.TempDir()
	// First write.
	result1, err := addMCP([]byte(validMCPNoEnvValues), AddOptions{WorkDir: dir})
	if err != nil {
		t.Fatalf("first add error: %v", err)
	}
	if result1.Identical {
		t.Fatal("first add must not be identical")
	}

	// Second write with same content.
	result2, err := addMCP([]byte(validMCPNoEnvValues), AddOptions{WorkDir: dir})
	if err != nil {
		t.Fatalf("second add error: %v", err)
	}
	if !result2.Identical {
		t.Fatal("second add with identical content must set Identical=true")
	}
}

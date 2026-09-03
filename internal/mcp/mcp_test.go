package mcp_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/axsmak/aim/internal/mcp"
)

var validYAML = []byte(`
name: test-server
description: A test MCP server
command: npx
args:
  - "-y"
  - test-mcp-pkg
targets:
  - claude-code
  - cursor
env:
  - name: API_KEY
    description: API key for the service
    required: true
    example: sk-xxxx
  - name: BASE_URL
    description: Base URL
    required: false
`)

func TestParse_Valid(t *testing.T) {
	m, err := mcp.Parse(validYAML)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Name != "test-server" {
		t.Errorf("Name = %q, want %q", m.Name, "test-server")
	}
	if m.Command != "npx" {
		t.Errorf("Command = %q, want %q", m.Command, "npx")
	}
	if len(m.Args) != 2 {
		t.Errorf("Args len = %d, want 2", len(m.Args))
	}
	if len(m.Targets) != 2 {
		t.Errorf("Targets len = %d, want 2", len(m.Targets))
	}
	if len(m.Env) != 2 {
		t.Errorf("Env len = %d, want 2", len(m.Env))
	}
	if !m.Env[0].Required {
		t.Error("Env[0].Required should be true")
	}
	if m.Env[1].Required {
		t.Error("Env[1].Required should be false")
	}
}

func TestParse_MissingName(t *testing.T) {
	data := []byte(`
description: A test MCP server
command: npx
args: []
targets:
  - claude-code
env: []
`)
	_, err := mcp.Parse(data)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestParse_MissingCommand(t *testing.T) {
	data := []byte(`
name: test-server
description: desc
args: []
targets:
  - claude-code
env: []
`)
	_, err := mcp.Parse(data)
	if err == nil {
		t.Fatal("expected error for missing command")
	}
}

func TestParse_EmptyTargets(t *testing.T) {
	data := []byte(`
name: test-server
description: desc
command: npx
args: []
targets: []
env: []
`)
	_, err := mcp.Parse(data)
	if err == nil {
		t.Fatal("expected error for empty targets")
	}
}

func TestParse_EnvMissingName(t *testing.T) {
	data := []byte(`
name: test-server
description: desc
command: npx
args: []
targets:
  - claude-code
env:
  - description: some env
    required: true
`)
	_, err := mcp.Parse(data)
	if err == nil {
		t.Fatal("expected error for env entry missing name")
	}
}

func TestParse_EmptyArgs(t *testing.T) {
	data := []byte(`
name: test-server
description: desc
command: npx
args: []
targets:
  - claude-code
env: []
`)
	m, err := mcp.Parse(data)
	if err != nil {
		t.Fatalf("empty args should be valid: %v", err)
	}
	if m.Args == nil {
		t.Error("Args should be empty slice, not nil")
	}
}

func TestParseDir_MultipleFiles(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "server1.yaml"), validYAML, 0644); err != nil {
		t.Fatal(err)
	}

	server2 := []byte(`
name: server2
description: Second server
command: node
args:
  - server2.js
targets:
  - cursor
env: []
`)
	if err := os.WriteFile(filepath.Join(dir, "server2.yaml"), server2, 0644); err != nil {
		t.Fatal(err)
	}

	items, errs := mcp.ParseDir(dir)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}
}

func TestParseDir_NonExistentDir(t *testing.T) {
	items, errs := mcp.ParseDir("/nonexistent/path/mcp")
	if errs != nil {
		t.Fatalf("expected nil errors for non-existent dir, got %v", errs)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
}

func TestParseDir_PartialSuccess(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "valid.yaml"), validYAML, 0644); err != nil {
		t.Fatal(err)
	}
	// invalid: missing name
	invalid := []byte(`
description: bad server
command: npx
args: []
targets:
  - claude-code
env: []
`)
	if err := os.WriteFile(filepath.Join(dir, "invalid.yaml"), invalid, 0644); err != nil {
		t.Fatal(err)
	}

	items, errs := mcp.ParseDir(dir)
	if len(items) != 1 {
		t.Errorf("expected 1 valid item, got %d", len(items))
	}
	if len(errs) != 1 {
		t.Errorf("expected 1 error, got %d", len(errs))
	}
}

func TestValidate_MissingDescriptionIsValid(t *testing.T) {
	m := mcp.MCP{
		Name:    "test",
		Command: "npx",
		Args:    []string{},
		Targets: []string{"claude-code"},
		Env: []mcp.EnvVar{
			{Name: "API_KEY"},
		},
	}
	if err := mcp.Validate(m); err != nil {
		t.Fatalf("expected no error for missing description, got %v", err)
	}
}

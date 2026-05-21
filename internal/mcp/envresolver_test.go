package mcp_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/axsmak/aim/internal/mcp"
)

var serverWithEnv = mcp.MCP{
	Name:    "test-server",
	Command: "npx",
	Args:    []string{},
	Targets: []string{"claude-code"},
	Env: []mcp.EnvVar{
		{Name: "API_KEY", Description: "API key", Required: true, Example: "sk-xxxx"},
		{Name: "BASE_URL", Description: "Base URL", Required: false},
	},
}

func TestResolveEnv_AllExisting(t *testing.T) {
	existing := map[string]string{
		"API_KEY":  "existing-key",
		"BASE_URL": "https://api.example.com",
	}
	r := strings.NewReader("")
	var w bytes.Buffer

	resolved, changed, err := mcp.ResolveEnv(serverWithEnv, existing, r, &w)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if changed {
		t.Error("changed should be false when all values already exist")
	}
	if resolved["API_KEY"] != "existing-key" {
		t.Errorf("API_KEY = %q", resolved["API_KEY"])
	}
}

func TestResolveEnv_PromptRequired(t *testing.T) {
	existing := map[string]string{}
	r := strings.NewReader("new-secret-key\n")
	var w bytes.Buffer

	resolved, changed, err := mcp.ResolveEnv(serverWithEnv, existing, r, &w)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Error("changed should be true after entering a new value")
	}
	if resolved["API_KEY"] != "new-secret-key" {
		t.Errorf("API_KEY = %q, want new-secret-key", resolved["API_KEY"])
	}
	// Optional BASE_URL not prompted, not in resolved
	if _, ok := resolved["BASE_URL"]; ok {
		t.Error("BASE_URL should not be in resolved when not provided")
	}
}

func TestResolveEnv_OptionalSkipped(t *testing.T) {
	existing := map[string]string{"API_KEY": "key"}
	r := strings.NewReader("")
	var w bytes.Buffer

	resolved, changed, err := mcp.ResolveEnv(serverWithEnv, existing, r, &w)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if changed {
		t.Error("changed should be false")
	}
	if _, ok := resolved["BASE_URL"]; ok {
		t.Error("optional BASE_URL with no value should not appear in resolved")
	}
}

func TestResolveEnv_EmptyInputForRequired(t *testing.T) {
	existing := map[string]string{}
	r := strings.NewReader("\n") // empty line
	var w bytes.Buffer

	resolved, changed, err := mcp.ResolveEnv(serverWithEnv, existing, r, &w)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if changed {
		t.Error("changed should be false when user enters empty string")
	}
	if _, ok := resolved["API_KEY"]; ok {
		t.Error("API_KEY should not be in resolved when user entered empty string")
	}
}

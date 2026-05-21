package importer_test

import (
	"testing"

	"github.com/axsmak/aim/internal/adapter"
	"github.com/axsmak/aim/internal/importer"
)

func TestNormalizeMCP_NoEnvVars(t *testing.T) {
	d := adapter.DiscoveredMCP{
		ServerName: "my-server",
		Source:     "cursor",
		Command:    "node",
		Args:       []string{"server.js"},
		Env:        nil,
	}

	m, secrets, err := importer.NormalizeMCP(d, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.Env) != 0 {
		t.Errorf("expected empty Env slice, got %v", m.Env)
	}
	if len(secrets) != 0 {
		t.Errorf("expected empty SecretBatch, got %v", secrets)
	}
}

func TestNormalizeMCP_TwoEnvVars(t *testing.T) {
	d := adapter.DiscoveredMCP{
		ServerName: "my-server",
		Source:     "cursor",
		Command:    "node",
		Args:       []string{"server.js"},
		Env: map[string]string{
			"API_KEY":  "real-key-value",
			"API_HOST": "https://example.com",
		},
	}

	m, secrets, err := importer.NormalizeMCP(d, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(m.Env) != 2 {
		t.Fatalf("expected 2 EnvVar descriptors, got %d", len(m.Env))
	}
	for _, ev := range m.Env {
		if !ev.Required {
			t.Errorf("expected Required=true for env var %q", ev.Name)
		}
		if ev.Value != "" {
			t.Errorf("expected Value to be empty for env var %q, got %q", ev.Name, ev.Value)
		}
	}

	if len(secrets) != 2 {
		t.Fatalf("expected 2 secrets, got %d", len(secrets))
	}
	if secrets["API_KEY"] != "real-key-value" {
		t.Errorf("expected secrets[API_KEY]=%q, got %q", "real-key-value", secrets["API_KEY"])
	}
	if secrets["API_HOST"] != "https://example.com" {
		t.Errorf("expected secrets[API_HOST]=%q, got %q", "https://example.com", secrets["API_HOST"])
	}
}

func TestNormalizeMCP_EmptyTargets(t *testing.T) {
	d := adapter.DiscoveredMCP{
		ServerName: "my-server",
		Command:    "node",
	}

	m, _, err := importer.NormalizeMCP(d, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.Targets) != 0 {
		t.Errorf("expected empty Targets, got %v", m.Targets)
	}
}

func TestNormalizeMCP_NonEmptyTargets(t *testing.T) {
	d := adapter.DiscoveredMCP{
		ServerName: "my-server",
		Command:    "node",
	}
	targets := []string{"cursor", "claude-code"}

	m, _, err := importer.NormalizeMCP(d, targets)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.Targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(m.Targets))
	}
	if m.Targets[0] != "cursor" || m.Targets[1] != "claude-code" {
		t.Errorf("unexpected targets: %v", m.Targets)
	}
}

func TestNormalizeMCP_FieldMapping(t *testing.T) {
	d := adapter.DiscoveredMCP{
		ServerName: "test-server",
		Source:     "cursor",
		Command:    "python",
		Args:       []string{"-m", "mcp_server", "--port", "8080"},
		Env:        nil,
	}

	m, _, err := importer.NormalizeMCP(d, []string{"cursor"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Name != d.ServerName {
		t.Errorf("Name: expected %q, got %q", d.ServerName, m.Name)
	}
	if m.Command != d.Command {
		t.Errorf("Command: expected %q, got %q", d.Command, m.Command)
	}
	if len(m.Args) != len(d.Args) {
		t.Fatalf("Args length: expected %d, got %d", len(d.Args), len(m.Args))
	}
	for i, arg := range d.Args {
		if m.Args[i] != arg {
			t.Errorf("Args[%d]: expected %q, got %q", i, arg, m.Args[i])
		}
	}
}

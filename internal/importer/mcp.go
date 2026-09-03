package importer

import (
	"sort"

	"github.com/axsmak/aim/internal/adapter"
	"github.com/axsmak/aim/internal/mcp"
)

// SecretBatch holds real env var values to be written to aim.local.yaml.
type SecretBatch map[string]string

// NormalizeMCP converts a DiscoveredMCP into a mcp.MCP (with env descriptors, no real values)
// and a SecretBatch (varName → real value) for writing to aim.local.yaml.
func NormalizeMCP(d adapter.DiscoveredMCP, targets []string) (mcp.MCP, SecretBatch, error) {
	secrets := make(SecretBatch)
	keys := make([]string, 0, len(d.Env))
	for k, v := range d.Env {
		secrets[k] = v
		keys = append(keys, k)
	}
	sort.Strings(keys)
	envVars := make([]mcp.EnvVar, 0, len(keys))
	for _, k := range keys {
		envVars = append(envVars, mcp.EnvVar{Name: k, Required: true})
	}
	m := mcp.MCP{
		Name:    d.ServerName,
		Command: d.Command,
		Args:    d.Args,
		Env:     envVars,
		Targets: targets,
	}
	return m, secrets, nil
}

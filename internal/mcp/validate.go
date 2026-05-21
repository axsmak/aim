package mcp

import "fmt"

// Validate checks that all required fields are present and non-empty.
func Validate(m MCP) error {
	if m.Name == "" {
		return fmt.Errorf("name: required")
	}
	if m.Description == "" {
		return fmt.Errorf("description: required")
	}
	if m.Command == "" {
		return fmt.Errorf("command: required")
	}
	if m.Args == nil {
		return fmt.Errorf("args: required (use [] for empty)")
	}
	if len(m.Targets) == 0 {
		return fmt.Errorf("targets: at least one target required")
	}
	if m.Env == nil {
		return fmt.Errorf("env: required (use [] for empty)")
	}
	for i, ev := range m.Env {
		if ev.Name == "" {
			return fmt.Errorf("env[%d].name: required", i)
		}
		if ev.Description == "" {
			return fmt.Errorf("env[%d].description: required", i)
		}
	}
	return nil
}

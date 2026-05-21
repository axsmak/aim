package mcp

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Parse parses and validates a single YAML file's bytes.
func Parse(data []byte) (MCP, error) {
	var m MCP
	if err := yaml.Unmarshal(data, &m); err != nil {
		return MCP{}, fmt.Errorf("yaml parse: %w", err)
	}
	if err := Validate(m); err != nil {
		return MCP{}, err
	}
	return m, nil
}

// ParseDir reads all *.yaml files from dir and parses each one.
// Successfully parsed items are returned alongside any per-file errors (partial success).
// If dir does not exist, returns an empty slice with no error.
func ParseDir(dir string) ([]MCP, []error) {
	if _, err := os.Stat(dir); err != nil {
		return nil, nil
	}

	matches, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil, []error{fmt.Errorf("glob %s: %w", dir, err)}
	}

	var items []MCP
	var errs []error
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", path, err))
			continue
		}
		m, err := Parse(data)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", path, err))
			continue
		}
		items = append(items, m)
	}
	return items, errs
}

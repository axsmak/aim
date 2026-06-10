package adder

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/axsmak/aim/internal/importer"
	"github.com/axsmak/aim/internal/localconfig"
	"github.com/axsmak/aim/internal/mcp"
	"gopkg.in/yaml.v3"
)

func addMCP(raw []byte, opts AddOptions) (AddResult, error) {
	m, err := mcp.Parse(raw)
	if err != nil {
		return AddResult{}, err
	}

	name := m.Name
	if opts.Name != "" {
		name = opts.Name
	}

	// Strip env values: extract real values to aim.local.yaml, leave only descriptors.
	var envValues []struct{ varName, value string }
	for i, ev := range m.Env {
		if ev.Value != "" {
			envValues = append(envValues, struct{ varName, value string }{ev.Name, ev.Value})
			m.Env[i].Value = ""
		}
	}

	stripped, err := yaml.Marshal(m)
	if err != nil {
		return AddResult{}, fmt.Errorf("marshal mcp: %w", err)
	}

	destPath := filepath.Join(opts.WorkDir, "mcp", name+".yaml")

	if err := importer.CheckConflict(destPath, stripped, opts.Overwrite); err != nil {
		if errors.Is(err, importer.ErrIdentical) {
			// File already exists with identical content — no-op.
			// HasSecrets is false: secrets were already there from the previous add.
			return AddResult{Name: name, Identical: true}, nil
		}
		return AddResult{}, err
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return AddResult{}, err
	}
	if err := os.WriteFile(destPath, stripped, 0644); err != nil {
		return AddResult{}, err
	}

	if len(envValues) > 0 {
		cfg, err := localconfig.Load(opts.WorkDir)
		if err != nil {
			return AddResult{}, fmt.Errorf("load aim.local.yaml: %w", err)
		}
		for _, ev := range envValues {
			cfg.SetMCPEnv(name, ev.varName, ev.value)
		}
		if err := localconfig.Save(opts.WorkDir, cfg); err != nil {
			return AddResult{}, fmt.Errorf("save aim.local.yaml: %w", err)
		}
	}

	return AddResult{Name: name, Identical: false, HasSecrets: len(envValues) > 0}, nil
}

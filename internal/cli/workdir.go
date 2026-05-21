package cli

import "github.com/axsmak/aim/internal/globalconfig"

// resolveWorkDir returns the active repository path from global config,
// falling back to "." if no global config is set.
func resolveWorkDir(homeDir string) string {
	cfg, err := globalconfig.Load(homeDir)
	if err != nil || cfg.Repo == "" {
		return "."
	}
	return cfg.Repo
}

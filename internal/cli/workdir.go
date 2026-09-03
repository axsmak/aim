package cli

import (
	"errors"

	"github.com/axsmak/aim/internal/globalconfig"
)

// resolveWorkDir returns the active repository path from global config,
// falling back to "." if no global config is set.
func resolveWorkDir(homeDir string) string {
	cfg, err := globalconfig.Load(homeDir)
	if err != nil || cfg.Repo == "" {
		return "."
	}
	return cfg.Repo
}

// errNoActiveRepo is returned by requireWorkDir when no repository is
// configured as active in the global config.
var errNoActiveRepo = errors.New("no active inventory repository; run 'aiman init' first")

// requireWorkDir returns the active repository path from global config, or
// an error if no repository is active. Unlike resolveWorkDir, it never
// falls back to the current working directory: commands that write to disk
// (e.g. import) must not silently create files outside the inventory repo.
func requireWorkDir(homeDir string) (string, error) {
	cfg, err := globalconfig.Load(homeDir)
	if err != nil {
		return "", err
	}
	if cfg.Repo == "" {
		return "", errNoActiveRepo
	}
	return cfg.Repo, nil
}

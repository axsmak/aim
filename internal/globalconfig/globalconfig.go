package globalconfig

import (
	"errors"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config holds the global AIM configuration.
type Config struct {
	Repo string `yaml:"repo"`
}

// Path returns the XDG-compliant path to the global config file.
// Path: ~/.config/aim/config.yaml
func Path(homeDir string) string {
	return filepath.Join(homeDir, ".config", "aim", "config.yaml")
}

// Load reads the global config. Returns empty Config (not error) if file doesn't exist.
func Load(homeDir string) (Config, error) {
	data, err := os.ReadFile(Path(homeDir))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, nil
		}
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Save writes the global config, creating ~/.config/aim/ directory if needed.
func Save(homeDir string, cfg Config) error {
	dir := filepath.Join(homeDir, ".config", "aim")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(Path(homeDir), data, 0644)
}

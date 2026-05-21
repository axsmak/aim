package repoconfig

import (
	"errors"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	SkillPaths map[string]string `yaml:"skill_paths"`
}

// Load reads aim.yaml from workDir. Returns empty Config if file does not exist.
func Load(workDir string) (Config, error) {
	path := filepath.Join(workDir, "aim.yaml")
	data, err := os.ReadFile(path)
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

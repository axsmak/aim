package localconfig

import (
	"errors"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Adapters      AdaptersConfig    `yaml:"adapters"`
	Repo          string            `yaml:"repo,omitempty"`
	SyncedHash    string            `yaml:"synced_hash,omitempty"`
	PublishedHash string            `yaml:"published_hash,omitempty"`
	MCPEnv        map[string]string `yaml:"mcp_env,omitempty"`
}

// GetMCPEnvForServer returns env values for a specific MCP server.
// Keys in MCPEnv are "serverName.VAR_NAME"; this returns only the VAR_NAME→value pairs.
func (c *Config) GetMCPEnvForServer(serverName string) map[string]string {
	prefix := serverName + "."
	result := make(map[string]string)
	for k, v := range c.MCPEnv {
		if len(k) > len(prefix) && k[:len(prefix)] == prefix {
			result[k[len(prefix):]] = v
		}
	}
	return result
}

// SetMCPEnv stores a single env value for an MCP server.
func (c *Config) SetMCPEnv(serverName, varName, value string) {
	if c.MCPEnv == nil {
		c.MCPEnv = make(map[string]string)
	}
	c.MCPEnv[serverName+"."+varName] = value
}

type AdaptersConfig struct {
	ClaudeCode AdapterConfig `yaml:"claude_code"`
	Cursor     AdapterConfig `yaml:"cursor"`
	Codex      AdapterConfig `yaml:"codex"`
}

type AdapterConfig struct {
	BaseDir string `yaml:"base_dir"`
}

func Save(workDir string, cfg Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(workDir, "aim.local.yaml"), data, 0644)
}

func Load(workDir string) (Config, error) {
	path := filepath.Join(workDir, "aim.local.yaml")
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

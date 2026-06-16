package adapter

import (
	"bytes"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"

	"github.com/axsmak/aim/internal/mcp"
	"github.com/axsmak/aim/internal/skill"
)

type CodexAdapter struct{ configBaseDir string }

func NewCodexAdapter(configBaseDir string) CodexAdapter {
	return CodexAdapter{configBaseDir: configBaseDir}
}

func (a CodexAdapter) Name() string { return "codex" }

func (a CodexAdapter) ScanSkills(baseDir string) ([]DiscoveredSkill, error) {
	dir := baseDir
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		dir = filepath.Join(home, ".codex")
	}
	skillsDir := filepath.Join(dir, "skills")
	if _, err := os.Stat(skillsDir); os.IsNotExist(err) {
		return nil, nil
	}
	// AIM installs skills as <name>/SKILL.md
	matches, err := filepath.Glob(filepath.Join(skillsDir, "*", "SKILL.md"))
	if err != nil {
		return nil, err
	}
	var out []DiscoveredSkill
	for _, path := range matches {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		name := filepath.Base(filepath.Dir(path))
		out = append(out, DiscoveredSkill{Name: name, Source: "codex", Raw: raw})
	}
	return out, nil
}

func (a CodexAdapter) ScanMCP(baseDir string) ([]DiscoveredMCP, error) {
	dir := baseDir
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		dir = filepath.Join(home, ".codex")
	}
	configPath := filepath.Join(dir, "config.toml")

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var config map[string]interface{}
	if _, err := toml.Decode(string(data), &config); err != nil {
		return nil, err
	}

	servers, _ := config["mcp_servers"].(map[string]interface{})
	if len(servers) == 0 {
		return nil, nil
	}

	out := make([]DiscoveredMCP, 0, len(servers))
	for name, raw := range servers {
		entry, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		cmd, _ := entry["command"].(string)

		var args []string
		if rawArgs, ok := entry["args"].([]interface{}); ok {
			for _, a := range rawArgs {
				if s, ok := a.(string); ok {
					args = append(args, s)
				}
			}
		}

		var env map[string]string
		if rawEnv, ok := entry["env"].(map[string]interface{}); ok && len(rawEnv) > 0 {
			env = make(map[string]string, len(rawEnv))
			for k, v := range rawEnv {
				if s, ok := v.(string); ok {
					env[k] = s
				}
			}
		}

		out = append(out, DiscoveredMCP{
			ServerName: name,
			Source:     "codex",
			Command:    cmd,
			Args:       args,
			Env:        env,
		})
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func (a CodexAdapter) Detect(homeDir string) (string, bool) {
	baseDir := a.configBaseDir
	if baseDir == "" {
		baseDir = filepath.Join(homeDir, ".codex")
	}
	if _, err := os.Stat(baseDir); err != nil {
		return "", false
	}
	return baseDir, true
}

func (a CodexAdapter) InstallSkill(s skill.Skill, baseDir string) error {
	return installSkillDir(s, baseDir)
}

func (a CodexAdapter) InstallMCP(m mcp.MCP, baseDir string, envValues map[string]string) error {
	configPath := filepath.Join(baseDir, "config.toml")
	return installMCPTOML(configPath, m, envValues)
}

func installMCPTOML(configPath string, m mcp.MCP, envValues map[string]string) error {
	var config map[string]interface{}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		config = make(map[string]interface{})
	} else {
		if _, err := toml.Decode(string(data), &config); err != nil {
			return err
		}
	}

	mcpServers, _ := config["mcp_servers"].(map[string]interface{})
	if mcpServers == nil {
		mcpServers = make(map[string]interface{})
	}

	entry := map[string]interface{}{
		"command": m.Command,
		"args":    m.Args,
	}
	if len(envValues) > 0 {
		entry["env"] = envValues
	}
	mcpServers[m.Name] = entry
	config["mcp_servers"] = mcpServers

	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(config); err != nil {
		return err
	}
	return os.WriteFile(configPath, buf.Bytes(), 0644)
}

package adapter

import (
	"bytes"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"

	"github.com/axsmak/aim/internal/fsutil"
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
		out = append(out, DiscoveredSkill{Name: name, Source: "codex", Raw: raw, IsFolder: true, SourceDir: filepath.Dir(path)})
	}
	return out, nil
}

func (a CodexAdapter) ScanMCP(baseDir string) ([]DiscoveredMCP, error) {
	servers, err := readMCPServersTOML(baseDir)
	if err != nil {
		return nil, err
	}
	out, _ := splitMCPServers(servers, "codex")
	return out, nil
}

// ScanUnsupportedMCP reports MCP server entries in config.toml whose
// transport isn't supported (see UnsupportedMCP).
func (a CodexAdapter) ScanUnsupportedMCP(baseDir string) ([]UnsupportedMCP, error) {
	servers, err := readMCPServersTOML(baseDir)
	if err != nil {
		return nil, err
	}
	_, unsupported := splitMCPServers(servers, "codex")
	return unsupported, nil
}

func readMCPServersTOML(baseDir string) (map[string]interface{}, error) {
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
	return servers, nil
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

func (a CodexAdapter) RemoveSkill(name string, baseDir string) error {
	return removeSkillDir(name, baseDir)
}

func (a CodexAdapter) RemoveMCP(name string, baseDir string) error {
	return removeMCPTOML(filepath.Join(baseDir, "config.toml"), name)
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
	return fsutil.WriteFile(configPath, buf.Bytes(), 0644)
}

// removeMCPTOML deletes the mcp_servers entry keyed by name from a TOML config
// file, preserving every other key. A missing file or key is a no-op: nothing
// is written and no config file is created as a side effect.
func removeMCPTOML(configPath, name string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var config map[string]interface{}
	if _, err := toml.Decode(string(data), &config); err != nil {
		return err
	}

	servers, _ := config["mcp_servers"].(map[string]interface{})
	if _, ok := servers[name]; !ok {
		return nil
	}
	delete(servers, name)
	config["mcp_servers"] = servers

	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(config); err != nil {
		return err
	}
	return fsutil.WriteFile(configPath, buf.Bytes(), 0644)
}

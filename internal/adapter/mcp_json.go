package adapter

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/axsmak/aim/internal/mcp"
)

// scanMCPFromJSON reads mcpServers from a JSON config file and returns DiscoveredMCP entries.
// Used by both ClaudeCode (settings.json) and Cursor (mcp.json) which share the same format.
func scanMCPFromJSON(configPath, source string) ([]DiscoveredMCP, error) {
	cfg, err := readJSONConfig(configPath)
	if err != nil {
		return nil, err
	}
	servers, _ := cfg["mcpServers"].(map[string]interface{})
	return parseMCPServers(servers, source), nil
}

func parseMCPServers(servers map[string]interface{}, source string) []DiscoveredMCP {
	if len(servers) == 0 {
		return nil
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
			Source:     source,
			Command:    cmd,
			Args:       args,
			Env:        env,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// installMCPJSON reads a JSON config file, merges the MCP server entry, and writes it back.
// Used by both ClaudeCode (settings.json) and Cursor (mcp.json) which share the same format.
func installMCPJSON(configPath string, m mcp.MCP, envValues map[string]string) error {
	config, err := readJSONConfig(configPath)
	if err != nil {
		return err
	}

	servers, _ := config["mcpServers"].(map[string]interface{})
	if servers == nil {
		servers = make(map[string]interface{})
	}

	entry := map[string]interface{}{
		"command": m.Command,
		"args":    m.Args,
	}
	if len(envValues) > 0 {
		entry["env"] = envValues
	}
	servers[m.Name] = entry
	config["mcpServers"] = servers

	return writeJSONConfig(configPath, config)
}

func readJSONConfig(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]interface{}{}, nil
		}
		return nil, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return map[string]interface{}{}, nil
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func writeJSONConfig(path string, cfg map[string]interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}

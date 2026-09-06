package adapter

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/axsmak/aim/internal/fsutil"
	"github.com/axsmak/aim/internal/mcp"
)

// scanMCPFromJSON reads mcpServers from a JSON config file and returns DiscoveredMCP entries.
// Used by both ClaudeCode (settings.json) and Cursor (mcp.json) which share the same format.
func scanMCPFromJSON(configPath, source string) ([]DiscoveredMCP, error) {
	servers, err := readMCPServersJSON(configPath)
	if err != nil {
		return nil, err
	}
	out, _ := splitMCPServers(servers, source)
	return out, nil
}

// scanUnsupportedMCPFromJSON reads mcpServers from a JSON config file and
// returns entries whose transport isn't supported (see UnsupportedMCP).
func scanUnsupportedMCPFromJSON(configPath, source string) ([]UnsupportedMCP, error) {
	servers, err := readMCPServersJSON(configPath)
	if err != nil {
		return nil, err
	}
	_, unsupported := splitMCPServers(servers, source)
	return unsupported, nil
}

func readMCPServersJSON(configPath string) (map[string]interface{}, error) {
	cfg, err := readJSONConfig(configPath)
	if err != nil {
		return nil, err
	}
	servers, _ := cfg["mcpServers"].(map[string]interface{})
	return servers, nil
}

// splitMCPServers parses a raw mcpServers map into stdio-backed DiscoveredMCP
// entries and UnsupportedMCP entries for anything using a non-stdio
// transport. An entry with neither a "command" nor a recognizable non-stdio
// transport marker is dropped silently, matching prior behavior for
// malformed entries.
func splitMCPServers(servers map[string]interface{}, source string) ([]DiscoveredMCP, []UnsupportedMCP) {
	if len(servers) == 0 {
		return nil, nil
	}
	var out []DiscoveredMCP
	var unsupported []UnsupportedMCP
	for name, raw := range servers {
		entry, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		cmd, _ := entry["command"].(string)
		if cmd == "" {
			if reason, ok := unsupportedMCPReason(entry); ok {
				unsupported = append(unsupported, UnsupportedMCP{Name: name, Source: source, Reason: reason})
				continue
			}
		}

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
		out = nil
	}
	if len(unsupported) == 0 {
		unsupported = nil
	}
	return out, unsupported
}

// unsupportedMCPReason inspects an MCP server entry that has no "command"
// and decides whether it's a recognizable non-stdio transport (HTTP/SSE,
// signaled by "type" and/or "url") rather than just a malformed entry. It
// returns a human-readable reason and true when the entry should be reported
// as UnsupportedMCP instead of silently dropped.
func unsupportedMCPReason(entry map[string]interface{}) (string, bool) {
	typ, _ := entry["type"].(string)
	url, hasURL := entry["url"].(string)
	hasURL = hasURL && url != ""

	switch {
	case typ != "" && typ != "stdio":
		return "unsupported transport \"" + typ + "\" (only stdio servers can be imported)", true
	case hasURL:
		return "unsupported transport (url-based server, only stdio servers can be imported)", true
	default:
		return "", false
	}
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

// removeMCPJSON deletes the mcpServers entry keyed by name from a JSON config
// file. Every other key — inside and outside mcpServers — is preserved. When
// the file or the key does not exist nothing is written, so removal is
// idempotent and never creates a config file as a side effect.
func removeMCPJSON(configPath, name string) error {
	config, err := readJSONConfig(configPath)
	if err != nil {
		return err
	}

	servers, _ := config["mcpServers"].(map[string]interface{})
	if _, ok := servers[name]; !ok {
		return nil
	}
	delete(servers, name)
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
	return fsutil.WriteFile(path, append(data, '\n'), 0644)
}

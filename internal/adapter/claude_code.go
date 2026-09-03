package adapter

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/axsmak/aim/internal/mcp"
	"github.com/axsmak/aim/internal/skill"
)

type ClaudeCodeAdapter struct{ configBaseDir string }

func NewClaudeCodeAdapter(configBaseDir string) ClaudeCodeAdapter {
	return ClaudeCodeAdapter{configBaseDir: configBaseDir}
}

func (a ClaudeCodeAdapter) Name() string { return "claude-code" }

func (a ClaudeCodeAdapter) ScanSkills(baseDir string) ([]DiscoveredSkill, error) {
	dir := baseDir
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		dir = filepath.Join(home, ".claude")
	}
	skillsDir := filepath.Join(dir, "skills")
	if _, err := os.Stat(skillsDir); os.IsNotExist(err) {
		return nil, nil
	}

	seen := make(map[string]bool)
	var out []DiscoveredSkill

	// Native flat files: skills/<name>.md
	flatMatches, err := filepath.Glob(filepath.Join(skillsDir, "*.md"))
	if err != nil {
		return nil, err
	}
	for _, path := range flatMatches {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		name := strings.TrimSuffix(filepath.Base(path), ".md")
		seen[name] = true
		out = append(out, DiscoveredSkill{Name: name, Source: "claude-code", Raw: raw, IsFolder: false})
	}

	// AIM-installed format: skills/<name>/SKILL.md
	subdirMatches, err := filepath.Glob(filepath.Join(skillsDir, "*", "SKILL.md"))
	if err != nil {
		return nil, err
	}
	for _, path := range subdirMatches {
		name := filepath.Base(filepath.Dir(path))
		if seen[name] {
			continue // flat file takes precedence
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		out = append(out, DiscoveredSkill{Name: name, Source: "claude-code", Raw: raw, IsFolder: true})
	}

	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func (a ClaudeCodeAdapter) ScanMCP(baseDir string) ([]DiscoveredMCP, error) {
	dir := baseDir
	var homeDir string
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		dir = filepath.Join(home, ".claude")
		homeDir = home
	} else {
		// Derive home from the provided configBaseDir (e.g. ~/.claude → ~).
		homeDir = filepath.Dir(dir)
	}

	// Claude Code stores globally-added MCP servers in ~/.claude.json.
	// ~/.claude/settings.json may also carry project-scoped entries.
	global, err := scanMCPFromJSON(filepath.Join(homeDir, ".claude.json"), "claude-code")
	if err != nil {
		return nil, err
	}
	local, err := scanMCPFromJSON(filepath.Join(dir, "settings.json"), "claude-code")
	if err != nil {
		return nil, err
	}

	// Merge: global takes precedence; local fills in anything not already present.
	seen := make(map[string]bool, len(global))
	out := make([]DiscoveredMCP, 0, len(global)+len(local))
	for _, d := range global {
		seen[d.ServerName] = true
		out = append(out, d)
	}
	for _, d := range local {
		if !seen[d.ServerName] {
			out = append(out, d)
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// ScanUnsupportedMCP reports MCP server entries in ~/.claude.json and
// settings.json whose transport isn't supported (see UnsupportedMCP).
// Entries are merged the same way ScanMCP merges DiscoveredMCP: global
// (~/.claude.json) takes precedence over local (settings.json) by name.
func (a ClaudeCodeAdapter) ScanUnsupportedMCP(baseDir string) ([]UnsupportedMCP, error) {
	dir := baseDir
	var homeDir string
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		dir = filepath.Join(home, ".claude")
		homeDir = home
	} else {
		homeDir = filepath.Dir(dir)
	}

	global, err := scanUnsupportedMCPFromJSON(filepath.Join(homeDir, ".claude.json"), "claude-code")
	if err != nil {
		return nil, err
	}
	local, err := scanUnsupportedMCPFromJSON(filepath.Join(dir, "settings.json"), "claude-code")
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool, len(global))
	out := make([]UnsupportedMCP, 0, len(global)+len(local))
	for _, u := range global {
		seen[u.Name] = true
		out = append(out, u)
	}
	for _, u := range local {
		if !seen[u.Name] {
			out = append(out, u)
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func (a ClaudeCodeAdapter) Detect(homeDir string) (string, bool) {
	baseDir := a.configBaseDir
	if baseDir == "" {
		baseDir = filepath.Join(homeDir, ".claude")
	}
	if _, err := os.Stat(baseDir); err != nil {
		return "", false
	}
	return baseDir, true
}

func (a ClaudeCodeAdapter) InstallSkill(s skill.Skill, baseDir string) error {
	return installSkillDir(s, baseDir)
}

func (a ClaudeCodeAdapter) InstallMCP(m mcp.MCP, baseDir string, envValues map[string]string) error {
	// Claude Code reads user-scope MCP server definitions from ~/.claude.json, not
	// settings.json (which only carries policy keys like allowedMcpServers). Mirror
	// the home derivation ScanMCP uses (~/.claude → ~) so install and scan agree.
	homeDir := filepath.Dir(baseDir)
	return installMCPJSON(filepath.Join(homeDir, ".claude.json"), m, envValues)
}

func (a ClaudeCodeAdapter) RemoveSkill(name string, baseDir string) error {
	return removeSkillDir(name, baseDir)
}

func (a ClaudeCodeAdapter) RemoveMCP(name string, baseDir string) error {
	// Mirror InstallMCP: user-scope MCP servers live in ~/.claude.json.
	// settings.json is never written by install, so removal leaves it alone.
	homeDir := filepath.Dir(baseDir)
	return removeMCPJSON(filepath.Join(homeDir, ".claude.json"), name)
}

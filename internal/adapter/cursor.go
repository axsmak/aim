package adapter

import (
	"os"
	"path/filepath"

	"github.com/axsmak/aim/internal/mcp"
	"github.com/axsmak/aim/internal/skill"
)

type CursorAdapter struct{ configBaseDir string }

func NewCursorAdapter(configBaseDir string) CursorAdapter {
	return CursorAdapter{configBaseDir: configBaseDir}
}

func (a CursorAdapter) Name() string { return "cursor" }

func (a CursorAdapter) ScanSkills(baseDir string) ([]DiscoveredSkill, error) {
	dir := baseDir
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		dir = filepath.Join(home, ".cursor")
	}
	skillsDir := filepath.Join(dir, "skills")
	if _, err := os.Stat(skillsDir); os.IsNotExist(err) {
		return nil, nil
	}
	// Cursor only supports folder-format skills: <name>/SKILL.md
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
		out = append(out, DiscoveredSkill{Name: name, Source: "cursor", Raw: raw, IsFolder: true})
	}
	return out, nil
}

func (a CursorAdapter) ScanMCP(baseDir string) ([]DiscoveredMCP, error) {
	dir := baseDir
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		dir = filepath.Join(home, ".cursor")
	}
	return scanMCPFromJSON(filepath.Join(dir, "mcp.json"), "cursor")
}

// ScanUnsupportedMCP reports MCP server entries in mcp.json whose transport
// isn't supported (see UnsupportedMCP).
func (a CursorAdapter) ScanUnsupportedMCP(baseDir string) ([]UnsupportedMCP, error) {
	dir := baseDir
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		dir = filepath.Join(home, ".cursor")
	}
	return scanUnsupportedMCPFromJSON(filepath.Join(dir, "mcp.json"), "cursor")
}

func (a CursorAdapter) Detect(homeDir string) (string, bool) {
	baseDir := a.configBaseDir
	if baseDir == "" {
		baseDir = filepath.Join(homeDir, ".cursor")
	}
	if _, err := os.Stat(baseDir); err != nil {
		return "", false
	}
	return baseDir, true
}

func (a CursorAdapter) InstallSkill(s skill.Skill, baseDir string) error {
	return installSkillDir(s, baseDir)
}

func (a CursorAdapter) InstallMCP(m mcp.MCP, baseDir string, envValues map[string]string) error {
	return installMCPJSON(filepath.Join(baseDir, "mcp.json"), m, envValues)
}

func (a CursorAdapter) RemoveSkill(name string, baseDir string) error {
	return removeSkillDir(name, baseDir)
}

func (a CursorAdapter) RemoveMCP(name string, baseDir string) error {
	return removeMCPJSON(filepath.Join(baseDir, "mcp.json"), name)
}

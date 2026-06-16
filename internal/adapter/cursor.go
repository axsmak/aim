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
	return nil, nil
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

package adapter

import (
	"github.com/axsmak/aim/internal/mcp"
	"github.com/axsmak/aim/internal/skill"
)

type Adapter interface {
	Name() string
	Detect(homeDir string) (string, bool)
	InstallSkill(s skill.Skill, baseDir string) error
	InstallMCP(m mcp.MCP, baseDir string, envValues map[string]string) error
}

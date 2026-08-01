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
	// RemoveSkill deletes the installed skill directory baseDir/skills/<name>/
	// including nested reference files. Removing a skill that is not installed
	// is a no-op, not an error.
	RemoveSkill(name string, baseDir string) error
	// RemoveMCP deletes the server entry keyed by name from the environment's
	// MCP config, leaving all other keys untouched. Removing a key that is not
	// present is a no-op, not an error.
	RemoveMCP(name string, baseDir string) error
}

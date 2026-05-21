package adapter

type SkillScanner interface {
	Name() string
	ScanSkills(baseDir string) ([]DiscoveredSkill, error)
}

type DiscoveredSkill struct {
	Name   string
	Source string // "cursor" | "claude-code" | "codex"
	Raw    []byte
}

type MCPScanner interface {
	Name() string
	ScanMCP(baseDir string) ([]DiscoveredMCP, error)
}

type DiscoveredMCP struct {
	ServerName string
	Source     string            // "cursor" | "claude-code" | "codex"
	Command    string
	Args       []string
	Env        map[string]string // real values — stripped in importer
}

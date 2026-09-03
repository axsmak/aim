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
	Source     string // "cursor" | "claude-code" | "codex"
	Command    string
	Args       []string
	Env        map[string]string // real values — stripped in importer
}

// UnsupportedMCP describes an MCP server entry a scanner found but could not
// represent as a DiscoveredMCP — currently, any entry that specifies a
// non-stdio transport ("type": "http"/"sse", or a bare "url") instead of a
// "command". Scanners never fabricate a DiscoveredMCP with an empty Command
// for these; they report them here instead, so a caller looking up a server
// by name (e.g. `aiman import mcp <name>`) can surface Reason instead of a
// generic "not found".
type UnsupportedMCP struct {
	Name   string
	Source string // "cursor" | "claude-code" | "codex"
	Reason string
}

// UnsupportedMCPScanner is an optional extension of MCPScanner: a scanner
// that also implements it can report MCP server entries it intentionally
// skipped because their transport isn't supported. It's a separate interface
// (rather than a change to MCPScanner.ScanMCP's signature) so existing
// callers of ScanMCP keep compiling unchanged.
type UnsupportedMCPScanner interface {
	ScanUnsupportedMCP(baseDir string) ([]UnsupportedMCP, error)
}

package mcp

// EnvVar describes an environment variable required by an MCP server.
// Value is populated only at ingestion time (aim add mcp); stored files always have it empty.
type EnvVar struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
	Required    bool   `yaml:"required"`
	Example     string `yaml:"example,omitempty"`
	Value       string `yaml:"value,omitempty"`
}

// MCP represents a single MCP server library item.
type MCP struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description,omitempty"`
	Command     string   `yaml:"command"`
	Args        []string `yaml:"args"`
	Targets     []string `yaml:"targets"`
	Env         []EnvVar `yaml:"env"`
}

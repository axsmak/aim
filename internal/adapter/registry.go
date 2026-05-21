package adapter

import "github.com/axsmak/aim/internal/localconfig"

// DefaultAdapters returns all adapters configured with optional base dir overrides.
func DefaultAdapters(cfg localconfig.Config) []Adapter {
	return []Adapter{
		NewClaudeCodeAdapter(cfg.Adapters.ClaudeCode.BaseDir),
		NewCursorAdapter(cfg.Adapters.Cursor.BaseDir),
		NewCodexAdapter(cfg.Adapters.Codex.BaseDir),
	}
}

// Registry returns all adapters with default configuration.
// Kept for backward compatibility with existing tests.
func Registry() []Adapter {
	return DefaultAdapters(localconfig.Config{})
}

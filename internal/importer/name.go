package importer

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode"
)

// maxItemNameLength is the maximum allowed length, in runes, for a skill or
// MCP item name imported from an external AI environment.
const maxItemNameLength = 255

// ValidateItemName rejects item names (skill or MCP server names discovered
// in a foreign AI environment config) that could escape the inventory
// workDir when joined into a file path, e.g. filepath.Join(workDir, "mcp",
// name+".yaml"). It intentionally does not normalize the name (no trimming,
// no case-folding): two distinct valid names must never be silently folded
// into the same one.
func ValidateItemName(name string) error {
	if name == "" {
		return fmt.Errorf("invalid item name %q: must not be empty", name)
	}
	if name == "." || name == ".." {
		return fmt.Errorf("invalid item name %q: must not be \".\" or \"..\"", name)
	}
	if strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("invalid item name %q: must not contain \"/\" or \"\\\"", name)
	}
	if filepath.IsAbs(name) {
		return fmt.Errorf("invalid item name %q: must not be an absolute path", name)
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return fmt.Errorf("invalid item name %q: must not contain control characters", name)
		}
	}
	if len([]rune(name)) > maxItemNameLength {
		return fmt.Errorf("invalid item name %q: must not exceed %d characters", name, maxItemNameLength)
	}
	return nil
}

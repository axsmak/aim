package adder

import (
	"fmt"
	"io"
)

type AddOptions struct {
	WorkDir   string
	Name      string
	Overwrite bool
}

// AddResult carries the outcome of a successful Add call.
type AddResult struct {
	Name       string
	Identical  bool // true when the file already existed with identical content (no-op)
	HasSecrets bool // true when MCP env values were written to aim.local.yaml
}

func Add(itemType string, r io.Reader, opts AddOptions) (AddResult, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return AddResult{}, err
	}
	switch itemType {
	case "skill":
		return addSkill(raw, opts)
	case "mcp":
		return addMCP(raw, opts)
	default:
		return AddResult{}, fmt.Errorf("unknown item type: %s", itemType)
	}
}

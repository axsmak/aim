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

func Add(itemType string, r io.Reader, opts AddOptions) error {
	raw, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	switch itemType {
	case "skill":
		return addSkill(raw, opts)
	case "mcp":
		return addMCP(raw, opts)
	default:
		return fmt.Errorf("unknown item type: %s", itemType)
	}
}

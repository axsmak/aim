package loadout

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RefError is a broken reference: a loadout item pointing at a library item
// that does not exist in the inventory (BFT section 5.2).
type RefError struct {
	// LoadoutName is the display name of the loadout (its `name` field).
	LoadoutName string
	// FilePath is the loadout file the broken reference lives in.
	FilePath string
	// Ref is the item reference that did not resolve.
	Ref ItemRef
}

func (e RefError) Error() string {
	return fmt.Sprintf("loadout %q references unknown %s %q", e.LoadoutName, e.Ref.Kind, e.Ref.Name)
}

// CheckRefs verifies reference integrity of a parsed loadout against the
// inventory on disk (BFT section 5.2): every skill:<name> must exist as
// skills/<name>.md or skills/<name>/SKILL.md (the same flat + folder
// discovery rules as skill.ReadAll), every mcp:<name> as mcp/<name>.yaml.
// ALL broken references are returned, not just the first (US-L04).
func CheckRefs(l Loadout, skillsDir, mcpDir string) []RefError {
	var broken []RefError
	for _, ref := range l.Refs {
		if refExists(ref, skillsDir, mcpDir) {
			continue
		}
		broken = append(broken, RefError{LoadoutName: l.Name, FilePath: l.FilePath, Ref: ref})
	}
	return broken
}

// refExists reports whether ref resolves to an inventory file. Names with
// path separators never resolve (defensive: keeps the check from escaping
// the inventory directories, mirroring candidateFileNames in Resolve).
func refExists(ref ItemRef, skillsDir, mcpDir string) bool {
	if strings.ContainsAny(ref.Name, `/\`) {
		return false
	}
	switch ref.Kind {
	case KindSkill:
		return fileExists(filepath.Join(skillsDir, ref.Name+".md")) ||
			fileExists(filepath.Join(skillsDir, ref.Name, "SKILL.md"))
	case KindMCP:
		return fileExists(filepath.Join(mcpDir, ref.Name+".yaml"))
	}
	return false
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

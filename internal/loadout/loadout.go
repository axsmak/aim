package loadout

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ItemKind is the kind of a library item referenced by a loadout.
type ItemKind string

const (
	KindSkill ItemKind = "skill"
	KindMCP   ItemKind = "mcp"
)

// ItemRef is the typed form of an items entry: "skill:<name>" or "mcp:<name>".
type ItemRef struct {
	Kind ItemKind
	Name string
}

func (r ItemRef) String() string {
	return string(r.Kind) + ":" + r.Name
}

// Loadout is a named subset of the inventory (ADR-0004, BFT section 3).
type Loadout struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Items       []string `yaml:"items"`
	Targets     []string `yaml:"targets"`

	// FilePath is the file the loadout was read from; used for error
	// reporting and the file-name invariant. Empty for in-memory parses.
	FilePath string `yaml:"-"`
	// Refs holds the typed form of the Items entries that parsed
	// successfully. Populated during validation.
	Refs []ItemRef `yaml:"-"`
}

// ValidationError is a blocking validation finding (same shape as
// skill.ValidationError so CLI consumers format them uniformly).
type ValidationError struct {
	FilePath string
	Field    string
	Message  string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s: %s", e.FilePath, e.Field, e.Message)
}

// Warning is a non-blocking validation finding (BFT section 5.2:
// missing description, file name not matching the normalized name).
type Warning struct {
	FilePath string
	Field    string
	Message  string
}

func (w Warning) String() string {
	return fmt.Sprintf("%s: %s: %s", w.FilePath, w.Field, w.Message)
}

// NotFoundError is returned by Resolve when no loadout matches the requested
// name. The final user-facing message is owned by the apply command (US-L05).
type NotFoundError struct {
	Name string
}

func (e NotFoundError) Error() string {
	return fmt.Sprintf("loadout %q not found in loadouts/", e.Name)
}

// Normalize converts a loadout name to its canonical file-name form:
// spaces become hyphens, letters are lowercased (BFT section 4).
func Normalize(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, " ", "-"))
}

// parseItemRef parses one items entry into its typed form.
func parseItemRef(raw string) (ItemRef, error) {
	kind, name, ok := strings.Cut(raw, ":")
	if !ok {
		return ItemRef{}, fmt.Errorf("missing prefix in %q (expected skill:<name> or mcp:<name>)", raw)
	}
	switch ItemKind(kind) {
	case KindSkill, KindMCP:
	default:
		return ItemRef{}, fmt.Errorf("unknown prefix %q in %q (expected skill:<name> or mcp:<name>)", kind, raw)
	}
	if name == "" {
		return ItemRef{}, fmt.Errorf("empty item name in %q", raw)
	}
	return ItemRef{Kind: ItemKind(kind), Name: name}, nil
}

// validate checks the invariants of BFT section 4 and returns ALL findings,
// so apply can fail fast on the first error while push reports every one
// (US-L04). It also populates l.Refs for entries that parsed successfully.
func validate(l *Loadout) ([]ValidationError, []Warning) {
	var errs []ValidationError
	var warns []Warning

	if l.Name == "" {
		errs = append(errs, ValidationError{FilePath: l.FilePath, Field: "name", Message: "required"})
	}
	if len(l.Items) == 0 {
		errs = append(errs, ValidationError{FilePath: l.FilePath, Field: "items", Message: "cannot be empty"})
	}
	for i, raw := range l.Items {
		ref, err := parseItemRef(raw)
		if err != nil {
			errs = append(errs, ValidationError{
				FilePath: l.FilePath,
				Field:    fmt.Sprintf("items[%d]", i),
				Message:  err.Error(),
			})
			continue
		}
		l.Refs = append(l.Refs, ref)
	}
	if l.Description == "" {
		warns = append(warns, Warning{FilePath: l.FilePath, Field: "description", Message: "missing"})
	}
	if l.Name != "" && l.FilePath != "" {
		base := strings.TrimSuffix(filepath.Base(l.FilePath), ".yaml")
		if norm := Normalize(l.Name); base != norm {
			warns = append(warns, Warning{
				FilePath: l.FilePath,
				Field:    "name",
				Message:  fmt.Sprintf("file name %q does not match normalized name %q", base, norm),
			})
		}
	}
	return errs, warns
}

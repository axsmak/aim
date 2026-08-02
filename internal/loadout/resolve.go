package loadout

import (
	"os"
	"path/filepath"
	"strings"
)

// List reads all *.yaml files from dir and parses each one. Valid loadouts
// are returned in file-name order alongside all validation errors and
// warnings from every file (partial success, like mcp.ParseDir). A missing
// dir is not an error: empty result — whether that matters is the caller's
// decision (US-L05 hint, future `aiman list`).
func List(dir string) ([]Loadout, []ValidationError, []Warning, error) {
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil, nil
		}
		return nil, nil, nil, err
	}

	matches, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil, nil, nil, err
	}

	var valid []Loadout
	var allErrs []ValidationError
	var allWarns []Warning
	for _, path := range matches {
		l, errs, warns, sysErr := ParseFile(path)
		if sysErr != nil {
			return nil, nil, nil, sysErr
		}
		allWarns = append(allWarns, warns...)
		if len(errs) > 0 {
			allErrs = append(allErrs, errs...)
			continue
		}
		valid = append(valid, l)
	}
	return valid, allErrs, allWarns, nil
}

// Resolve finds a loadout in dir by the name given on the CLI (BFT 5.1):
//
//  1. exact file name match: dir/<name>.yaml
//  2. normalized file name: dir/<Normalize(name)>.yaml
//  3. a loadout whose normalized `name` field equals Normalize(name)
//
// When a match is found its validation findings are returned with it, so
// apply can fail on the first error and still surface warnings. When nothing
// matches, the error is a NotFoundError; the caller owns the final message
// and the available-loadouts hint (US-L05).
func Resolve(dir, name string) (Loadout, []ValidationError, []Warning, error) {
	for _, base := range candidateFileNames(name) {
		path := filepath.Join(dir, base+".yaml")
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return Loadout{}, nil, nil, err
		}
		l, errs, warns, sysErr := ParseFile(path)
		if sysErr != nil {
			return Loadout{}, nil, nil, sysErr
		}
		return l, errs, warns, nil
	}

	// Fall back to matching the normalized `name` field across all files.
	matches, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return Loadout{}, nil, nil, err
	}
	want := Normalize(name)
	for _, path := range matches {
		l, errs, warns, sysErr := ParseFile(path)
		if sysErr != nil {
			return Loadout{}, nil, nil, sysErr
		}
		if l.Name != "" && Normalize(l.Name) == want {
			return l, errs, warns, nil
		}
	}
	return Loadout{}, nil, nil, NotFoundError{Name: name}
}

// candidateFileNames returns the file base names to try for a CLI-supplied
// loadout name. Names containing path separators never map to a file inside
// dir (defensive: keeps Resolve from escaping the loadouts directory).
func candidateFileNames(name string) []string {
	if name == "" || strings.ContainsAny(name, `/\`) {
		return nil
	}
	norm := Normalize(name)
	if norm == name {
		return []string{name}
	}
	return []string{name, norm}
}

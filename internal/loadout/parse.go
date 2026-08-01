package loadout

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Parse parses and validates loadout YAML bytes. path is used for error
// reporting and the file-name invariant; pass "" when no file is involved.
// A YAML syntax failure is a validation error, not a system error: push
// treats an unparseable loadouts/<name>.yaml as a blocking finding
// (BFT section 5.2). All findings are returned so callers can either fail
// fast on the first error (apply) or report every one (push, US-L04).
func Parse(data []byte, path string) (Loadout, []ValidationError, []Warning) {
	var l Loadout
	if err := yaml.Unmarshal(data, &l); err != nil {
		return Loadout{FilePath: path}, []ValidationError{{
			FilePath: path,
			Field:    "yaml",
			Message:  err.Error(),
		}}, nil
	}
	l.FilePath = path
	errs, warns := validate(&l)
	return l, errs, warns
}

// ParseFile reads and parses the loadout file at path. The last return value
// is a system error (unreadable file); validation findings come separately.
func ParseFile(path string) (Loadout, []ValidationError, []Warning, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Loadout{}, nil, nil, err
	}
	l, errs, warns := Parse(data, path)
	return l, errs, warns, nil
}

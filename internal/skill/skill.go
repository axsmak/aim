package skill

import (
	"fmt"
	"os"
	"path/filepath"
)

type Skill struct {
	Name        string
	Description string
	Body        string
	Raw         []byte
	FilePath    string
}

type ValidationError struct {
	FilePath string
	Field    string
	Message  string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s: %s", e.FilePath, e.Field, e.Message)
}

// ReadAll reads all *.md files from skillsDir.
// Returns valid skills, per-file validation errors, and a system error if the directory can't be read.
func ReadAll(skillsDir string) (valid []Skill, invalid []ValidationError, err error) {
	if _, err := os.Stat(skillsDir); err != nil {
		return nil, nil, err
	}

	matches, err := filepath.Glob(filepath.Join(skillsDir, "*.md"))
	if err != nil {
		return nil, nil, err
	}

	for _, path := range matches {
		s, ve, sysErr := parseFile(path)
		if sysErr != nil {
			return nil, nil, sysErr
		}
		if ve != nil {
			invalid = append(invalid, *ve)
			continue
		}
		valid = append(valid, s)
	}
	return valid, invalid, nil
}

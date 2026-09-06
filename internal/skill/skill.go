package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Skill struct {
	Name        string
	Description string
	Body        string
	Raw         []byte
	FilePath    string
	SourceDir   string   // non-empty for folder skills: path to skills/<name>/
	RefFiles    []string // relative paths of reference files inside SourceDir (everything except SKILL.md)
	Targets     []string // environments this skill is delivered to; nil or empty means all discovered environments
}

type ValidationError struct {
	FilePath string
	Field    string
	Message  string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s: %s", e.FilePath, e.Field, e.Message)
}

// WriteTo writes s into baseDir/skills/<s.Name>/SKILL.md. For folder-format
// skills (s.SourceDir != "") every path in s.RefFiles is also copied from
// s.SourceDir alongside SKILL.md.
func WriteTo(s Skill, baseDir string) error {
	destDir := filepath.Join(baseDir, "skills", s.Name)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(destDir, "SKILL.md"), s.Raw, 0644); err != nil {
		return err
	}
	for _, ref := range s.RefFiles {
		srcPath := filepath.Join(s.SourceDir, ref)
		info, err := os.Stat(srcPath)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(srcPath)
		if err != nil {
			return err
		}
		dest := filepath.Join(destDir, ref)
		// Ref paths may be nested (e.g. references/backend.tpl.md).
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return err
		}
		perm := info.Mode().Perm()
		if err := os.WriteFile(dest, data, perm); err != nil {
			return err
		}
		// os.WriteFile's perm only applies on O_CREATE; an existing dest
		// keeps its old mode unless explicitly chmod'd, so a re-run over a
		// previously written file would otherwise leave a stale mode behind.
		if err := os.Chmod(dest, perm); err != nil {
			return err
		}
	}
	return nil
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

	// Folder skills: skills/<name>/SKILL.md
	// Flat names take precedence — track seen names from both valid and invalid flat skills.
	seen := make(map[string]bool)
	for _, s := range valid {
		seen[s.Name] = true
	}
	for _, ve := range invalid {
		baseName := strings.TrimSuffix(filepath.Base(ve.FilePath), ".md")
		seen[baseName] = true
	}

	subdirMatches, err := filepath.Glob(filepath.Join(skillsDir, "*", "SKILL.md"))
	if err != nil {
		return nil, nil, err
	}
	for _, path := range subdirMatches {
		name := filepath.Base(filepath.Dir(path))
		if seen[name] {
			continue
		}
		seen[name] = true
		s, ve, sysErr := parseFolderSkill(path, name)
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

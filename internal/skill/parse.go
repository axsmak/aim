package skill

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type frontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

func parseFile(path string) (Skill, *ValidationError, error) {
	raw, err := readFile(path)
	if err != nil {
		return Skill{}, nil, err
	}

	fm, body, ok := splitFrontmatter(raw)
	if !ok {
		return Skill{}, &ValidationError{
			FilePath: path,
			Field:    "frontmatter",
			Message:  "file does not start with ---",
		}, nil
	}

	var meta frontmatter
	if err := yaml.Unmarshal([]byte(fm), &meta); err != nil {
		return Skill{}, nil, err
	}

	if meta.Name == "" {
		return Skill{}, &ValidationError{FilePath: path, Field: "name", Message: "required"}, nil
	}
	if meta.Description == "" {
		return Skill{}, &ValidationError{FilePath: path, Field: "description", Message: "required"}, nil
	}
	if strings.TrimSpace(body) == "" {
		return Skill{}, &ValidationError{FilePath: path, Field: "body", Message: "must not be empty"}, nil
	}

	return Skill{
		Name:        meta.Name,
		Description: meta.Description,
		Body:        body,
		Raw:         raw,
		FilePath:    path,
	}, nil, nil
}

// parseFolderSkill parses a folder-format skill from its SKILL.md file.
// The skill name is taken from the directory name, not from frontmatter.
// Returns a ValidationError (not a system error) when required fields are missing.
func parseFolderSkill(skillMDPath string, name string) (Skill, *ValidationError, error) {
	raw, err := readFile(skillMDPath)
	if err != nil {
		return Skill{}, nil, err
	}

	fm, body, ok := splitFrontmatter(raw)
	if !ok {
		return Skill{}, &ValidationError{
			FilePath: skillMDPath,
			Field:    "frontmatter",
			Message:  "file does not start with ---",
		}, nil
	}

	var meta frontmatter
	if err := yaml.Unmarshal([]byte(fm), &meta); err != nil {
		return Skill{}, nil, err
	}

	// Name comes from directory; description and body are still required.
	if meta.Description == "" {
		return Skill{}, &ValidationError{FilePath: skillMDPath, Field: "description", Message: "required"}, nil
	}
	if strings.TrimSpace(body) == "" {
		return Skill{}, &ValidationError{FilePath: skillMDPath, Field: "body", Message: "must not be empty"}, nil
	}

	// Collect reference files: every file under the folder except the root
	// SKILL.md, recursively, as paths relative to the skill directory. Skills
	// commonly keep templates in subdirectories (e.g. references/*.tpl.md);
	// a non-recursive listing silently dropped them (issue #145).
	dir := filepath.Dir(skillMDPath)
	var refFiles []string
	walkErr := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if rel == "SKILL.md" {
			return nil
		}
		refFiles = append(refFiles, rel)
		return nil
	})
	if walkErr != nil {
		return Skill{}, nil, walkErr
	}

	return Skill{
		Name:        name,
		Description: meta.Description,
		Body:        body,
		Raw:         raw,
		FilePath:    skillMDPath,
		SourceDir:   dir,
		RefFiles:    refFiles,
	}, nil, nil
}

// ReadFolderSkill parses a folder-format skill given the path to its SKILL.md
// file, deriving the skill name from the parent directory's name. Unlike
// parseFolderSkill, this is exported for ingestion of folder skills from
// arbitrary external paths (not just skillsDir/*/SKILL.md during ReadAll).
func ReadFolderSkill(skillMDPath string) (Skill, *ValidationError, error) {
	name := filepath.Base(filepath.Dir(skillMDPath))
	return parseFolderSkill(skillMDPath, name)
}

// ParseRaw parses a skill from raw bytes. sourcePath is used only for error reporting.
func ParseRaw(raw []byte, sourcePath string) (Skill, *ValidationError, error) {
	fm, body, ok := splitFrontmatter(raw)
	if !ok {
		return Skill{}, &ValidationError{
			FilePath: sourcePath,
			Field:    "frontmatter",
			Message:  "file does not start with ---",
		}, nil
	}

	var meta frontmatter
	if err := yaml.Unmarshal([]byte(fm), &meta); err != nil {
		return Skill{}, nil, err
	}

	if meta.Name == "" {
		return Skill{}, &ValidationError{FilePath: sourcePath, Field: "name", Message: "required"}, nil
	}
	if meta.Description == "" {
		return Skill{}, &ValidationError{FilePath: sourcePath, Field: "description", Message: "required"}, nil
	}
	if strings.TrimSpace(body) == "" {
		return Skill{}, &ValidationError{FilePath: sourcePath, Field: "body", Message: "must not be empty"}, nil
	}

	return Skill{
		Name:        meta.Name,
		Description: meta.Description,
		Body:        body,
		Raw:         raw,
	}, nil, nil
}

// splitFrontmatter splits raw file content into frontmatter YAML and body.
// The file must start with "---" on the first line; frontmatter ends at the
// first subsequent line that equals "---" exactly.
func splitFrontmatter(raw []byte) (fm string, body string, ok bool) {
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	if len(lines) == 0 || lines[0] != "---" {
		return "", "", false
	}
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			fm = strings.Join(lines[1:i], "\n")
			body = strings.Join(lines[i+1:], "\n")
			return fm, body, true
		}
	}
	return "", "", false
}

// readFile is a thin wrapper so tests can verify system errors are propagated.
func readFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

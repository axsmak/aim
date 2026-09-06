package skill

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Default limits on folder-format skill package resources (ADR-0008a, п.4).
// A package exceeding any of these is rejected outright rather than
// silently truncated — the caller sees a ValidationError with the exact
// reason, not a partial RefFiles list.
const (
	maxSkillPackageFiles     = 200
	maxSkillPackageFileSize  = 1 << 20 // 1 MB
	maxSkillPackageTotalSize = 5 << 20 // 5 MB
	maxSkillPackageDepth     = 5       // nested directories below the skill root
)

type frontmatter struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Targets     []string `yaml:"targets"`
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
		Targets:     meta.Targets,
	}, nil, nil
}

// parseFolderSkill parses a folder-format skill from its SKILL.md file.
// The skill name is taken from the directory name, not from frontmatter.
// Returns a ValidationError (not a system error) when required fields are
// missing, or when the resource walk below rejects the package — either
// way the caller (ReadAll) can mark this one skill invalid without failing
// the read of the rest of the inventory.
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
	//
	// The walk enforces default resource limits (ADR-0008a, п.4): this path
	// now also runs over directories from other AI environments during
	// `import skill` (issue #180), not just directories the user names
	// directly with `add skill <dir>`. filepath.WalkDir uses Lstat semantics
	// and does not itself follow symlinks, so the entry's type is known
	// without an extra stat call — a symlink or other special file (device,
	// socket, FIFO) is rejected here, before anything reads through it.
	dir := filepath.Dir(skillMDPath)
	var refFiles []string
	var vErr *ValidationError
	var fileCount int
	var totalSize int64

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

		if d.Type()&fs.ModeSymlink != 0 {
			vErr = &ValidationError{FilePath: path, Field: "resource", Message: "symlink not allowed in skill package"}
			return fs.SkipAll
		}
		if !d.Type().IsRegular() {
			vErr = &ValidationError{FilePath: path, Field: "resource", Message: "special file (device, socket, or FIFO) not allowed in skill package"}
			return fs.SkipAll
		}

		if depth := strings.Count(rel, string(filepath.Separator)); depth > maxSkillPackageDepth {
			vErr = &ValidationError{FilePath: path, Field: "resource", Message: fmt.Sprintf("nested deeper than %d directories", maxSkillPackageDepth)}
			return fs.SkipAll
		}

		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Size() > maxSkillPackageFileSize {
			vErr = &ValidationError{FilePath: path, Field: "resource", Message: fmt.Sprintf("exceeds %d byte file size limit", maxSkillPackageFileSize)}
			return fs.SkipAll
		}

		fileCount++
		if fileCount > maxSkillPackageFiles {
			vErr = &ValidationError{FilePath: dir, Field: "resource", Message: fmt.Sprintf("exceeds %d file limit", maxSkillPackageFiles)}
			return fs.SkipAll
		}

		totalSize += info.Size()
		if totalSize > maxSkillPackageTotalSize {
			vErr = &ValidationError{FilePath: dir, Field: "resource", Message: fmt.Sprintf("exceeds %d byte total package size limit", maxSkillPackageTotalSize)}
			return fs.SkipAll
		}

		refFiles = append(refFiles, rel)
		return nil
	})
	if walkErr != nil {
		return Skill{}, nil, walkErr
	}
	if vErr != nil {
		return Skill{}, vErr, nil
	}

	return Skill{
		Name:        name,
		Description: meta.Description,
		Body:        body,
		Raw:         raw,
		FilePath:    skillMDPath,
		SourceDir:   dir,
		RefFiles:    refFiles,
		Targets:     meta.Targets,
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
		Targets:     meta.Targets,
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

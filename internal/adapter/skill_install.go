package adapter

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/axsmak/aim/internal/skill"
)

// installSkillDir writes s into baseDir/skills/<s.Name>/. For flat skills
// (s.SourceDir == "") only SKILL.md is written. For folder-format skills
// every path listed in s.RefFiles is copied from s.SourceDir alongside SKILL.md.
func installSkillDir(s skill.Skill, baseDir string) error {
	return skill.WriteTo(s, baseDir)
}

// removeSkillDir deletes baseDir/skills/<name>/ recursively — the exact
// footprint installSkillDir writes, including nested reference files of
// folder-format skills. A missing directory is a no-op so repeated
// reconciliation runs never fail.
func removeSkillDir(name, baseDir string) error {
	if err := validateSkillName(name); err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(baseDir, "skills", name))
}

// validateSkillName rejects names that could escape the skills directory:
// empty names, ".", "..", path separators. Checked before any deletion so a
// malformed inventory entry can never remove files outside baseDir/skills/.
func validateSkillName(name string) error {
	if name == "" || name == "." || name == ".." ||
		strings.ContainsAny(name, `/\`) ||
		strings.ContainsRune(name, filepath.Separator) {
		return fmt.Errorf("invalid skill name %q: must not be empty or contain path separators or '..'", name)
	}
	return nil
}

package adapter

import (
	"os"
	"path/filepath"

	"github.com/axsmak/aim/internal/skill"
)

// installSkillDir writes s into baseDir/skills/<s.Name>/. For flat skills
// (s.SourceDir == "") only SKILL.md is written. For folder-format skills
// every path listed in s.RefFiles is copied from s.SourceDir alongside SKILL.md.
func installSkillDir(s skill.Skill, baseDir string) error {
	destDir := filepath.Join(baseDir, "skills", s.Name)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(destDir, "SKILL.md"), s.Raw, 0644); err != nil {
		return err
	}
	if s.SourceDir == "" {
		return nil
	}
	// Copy reference files for folder skills
	for _, ref := range s.RefFiles {
		data, err := os.ReadFile(filepath.Join(s.SourceDir, ref))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(destDir, ref), data, 0644); err != nil {
			return err
		}
	}
	return nil
}

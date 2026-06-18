package adapter

import (
	"github.com/axsmak/aim/internal/skill"
)

// installSkillDir writes s into baseDir/skills/<s.Name>/. For flat skills
// (s.SourceDir == "") only SKILL.md is written. For folder-format skills
// every path listed in s.RefFiles is copied from s.SourceDir alongside SKILL.md.
func installSkillDir(s skill.Skill, baseDir string) error {
	return skill.WriteTo(s, baseDir)
}

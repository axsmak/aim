package adder

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/axsmak/aim/internal/importer"
	"github.com/axsmak/aim/internal/skill"
)

func addSkill(raw []byte, opts AddOptions) error {
	s, ve, err := skill.ParseRaw(raw, "<stdin>")
	if err != nil {
		return err
	}
	if ve != nil {
		return fmt.Errorf("%w", ve)
	}

	name := s.Name
	if opts.Name != "" {
		name = opts.Name
	}
	if name == "" {
		return fmt.Errorf("skill name is required: set 'name' in frontmatter or use --name")
	}

	destPath := filepath.Join(opts.WorkDir, "skills", name+".md")

	if err := importer.CheckConflict(destPath, raw, opts.Overwrite); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}
	return os.WriteFile(destPath, raw, 0644)
}

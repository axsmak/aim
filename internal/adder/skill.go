package adder

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/axsmak/aim/internal/importer"
	"github.com/axsmak/aim/internal/skill"
)

func addSkill(raw []byte, opts AddOptions) (AddResult, error) {
	s, ve, err := skill.ParseRaw(raw, "<stdin>")
	if err != nil {
		return AddResult{}, err
	}
	if ve != nil {
		return AddResult{}, fmt.Errorf("%w", ve)
	}

	name := s.Name
	if opts.Name != "" {
		name = opts.Name
	}
	if name == "" {
		return AddResult{}, fmt.Errorf("skill name is required: set 'name' in frontmatter or use --name")
	}

	destPath := filepath.Join(opts.WorkDir, "skills", name+".md")

	if err := importer.CheckConflict(destPath, raw, opts.Overwrite); err != nil {
		if errors.Is(err, importer.ErrIdentical) {
			return AddResult{Name: name, Identical: true}, nil
		}
		return AddResult{}, err
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return AddResult{}, err
	}
	if err := os.WriteFile(destPath, raw, 0644); err != nil {
		return AddResult{}, err
	}
	return AddResult{Name: name, Identical: false}, nil
}

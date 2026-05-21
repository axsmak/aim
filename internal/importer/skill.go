package importer

import (
	"fmt"

	"github.com/axsmak/aim/internal/adapter"
	"github.com/axsmak/aim/internal/skill"
)

func NormalizeSkill(d adapter.DiscoveredSkill) (skill.Skill, error) {
	s, ve, err := skill.ParseRaw(d.Raw, d.Source+"/"+d.Name)
	if err != nil {
		return skill.Skill{}, err
	}
	if ve != nil {
		return skill.Skill{}, fmt.Errorf("%w", ve)
	}
	return s, nil
}

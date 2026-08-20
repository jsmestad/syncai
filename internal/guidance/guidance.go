package guidance

import (
	_ "embed"

	"github.com/jsmestad/syncai/internal/renderers"
)

const SkillName = "syncai"

//go:embed guide.md
var Guide string

//go:embed syncai-skill.md
var skill []byte

func BuiltInSkills() []renderers.BuiltInSkill {
	return []renderers.BuiltInSkill{{Name: SkillName, Content: skill}}
}

func IsBuiltInSkill(name string) bool {
	return name == SkillName
}

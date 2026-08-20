package renderers

import (
	"fmt"
	"path/filepath"

	"github.com/jsmestad/syncai/internal/load"
)

type BuiltInSkill struct {
	Name    string
	Content []byte
}

func WriteSkills(outRoot, destination string, sourceDirs []string, builtIns []BuiltInSkill) ([]string, error) {
	if err := ValidateSkillConflicts(sourceDirs, builtIns); err != nil {
		return nil, err
	}

	var written []string
	for _, source := range sourceDirs {
		name := filepath.Base(source)
		target := filepath.Join(destination, name)
		if err := load.CopyDir(outRoot, source, target); err != nil {
			return nil, fmt.Errorf("copying skill %s: %w", name, err)
		}
		written = append(written, target)
	}
	for _, builtIn := range builtIns {
		target := filepath.Join(destination, builtIn.Name)
		if err := load.WriteFileReplacing(outRoot, filepath.Join(target, "SKILL.md"), builtIn.Content, 0o644); err != nil {
			return nil, fmt.Errorf("writing built-in skill %s: %w", builtIn.Name, err)
		}
		written = append(written, target)
	}
	return written, nil
}

func ValidateSkillConflicts(sourceDirs []string, builtIns []BuiltInSkill) error {
	for _, source := range sourceDirs {
		for _, builtIn := range builtIns {
			if filepath.Base(source) == builtIn.Name {
				return fmt.Errorf("source skill %q conflicts with a built-in SyncAI skill", builtIn.Name)
			}
		}
	}
	return nil
}

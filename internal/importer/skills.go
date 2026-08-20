package importer

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jsmestad/syncai/internal/load"
)

// SkillCandidate is one installed skill directory that has no equivalent in
// ai-source/skills/. Unlike agents, skill SKILL.md files carry no per-tool
// frontmatter (renderers copy every skill dir verbatim to every target), so
// every tool's candidates are auto-portable via a plain directory copy.
type SkillCandidate struct {
	Name       string // "minga-preview"
	Tool       string // "pi", "claude", "codex", "antigravity"
	InputPath  string // /Users/x/.claude/skills/minga-preview
	SourcePath string // /Users/x/code/.../ai-source/skills/minga-preview
}

// SkillScanRoots returns the (tool, dir) pairs to scan for installed skills.
// OpenCode is omitted: syncai's opencode renderer does not emit skills, so
// there is no installed directory to compare against.
func SkillScanRoots(homeDir string) []struct{ Tool, Dir string } {
	return []struct{ Tool, Dir string }{
		{"pi", filepath.Join(homeDir, ".pi", "agent", "skills")},
		{"claude", filepath.Join(homeDir, ".claude", "skills")},
		{"codex", filepath.Join(homeDir, ".codex", "skills")},
		{"antigravity", filepath.Join(homeDir, ".gemini", "antigravity-cli", "plugins", "dfiles", "skills")},
	}
}

// ScanSkills walks every installed skills dir, compares each subdirectory's
// name against <sourceRoot>/skills/, and returns the orphans.
//
// Filtering rules mirror ScanExtensionDirectories:
//   - Dot-prefixed entries are skipped (e.g. Codex ships a built-in
//     `.system/` skills directory that isn't user content).
//   - Symlinked directories are skipped — those are already managed
//     elsewhere (e.g. a project-local skill symlinked in from another
//     dfiles-managed tree).
//   - A directory only counts as a skill if it contains a SKILL.md file.
//   - Anything already present in <sourceRoot>/skills/ is skipped.
func ScanSkills(homeDir, sourceRoot string) ([]SkillCandidate, error) {
	known, err := dirNameSet(filepath.Join(sourceRoot, "skills"))
	if err != nil {
		return nil, err
	}
	var out []SkillCandidate
	for _, root := range SkillScanRoots(homeDir) {
		entries, err := os.ReadDir(root.Dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".") || !e.IsDir() {
				continue
			}
			if known[e.Name()] {
				continue
			}
			dirPath := filepath.Join(root.Dir, e.Name())
			info, err := os.Lstat(dirPath)
			if err != nil {
				return nil, err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				continue
			}
			if _, err := os.Stat(filepath.Join(dirPath, "SKILL.md")); err != nil {
				continue
			}
			out = append(out, SkillCandidate{
				Name:       e.Name(),
				Tool:       root.Tool,
				InputPath:  dirPath,
				SourcePath: filepath.Join(sourceRoot, "skills", e.Name()),
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Tool < out[j].Tool
	})
	return out, nil
}

// PortSkill copies the installed skill directory verbatim into
// ai-source/skills/<name>/. A plain copy is already the generic, tool-
// agnostic form — skills have no per-target rewrites the way agents do.
func PortSkill(c SkillCandidate) error {
	return load.CopyDir(c.InputPath, c.SourcePath)
}

// dirNameSet returns the basenames of every subdirectory in dir.
func dirNameSet(dir string) (map[string]bool, error) {
	out := map[string]bool{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() {
			out[e.Name()] = true
		}
	}
	return out, nil
}

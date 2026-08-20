package load

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jsmestad/syncai/internal/schema"
)

// SkillDirs returns the absolute paths of every skill directory under
// <sourceRoot>/skills/, sorted by name. When scopeFilter is non-empty, skills
// whose SKILL.md frontmatter declares a scope list that does not include the
// filter are excluded. Skills with no `scope:` line are universal and always
// included.
func SkillDirs(sourceRoot, scopeFilter string) ([]string, error) {
	dir := filepath.Join(sourceRoot, "skills")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading %s: %w", dir, err)
	}
	var dirs []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(dir, e.Name())
		scope, err := readSkillScope(path)
		if err != nil {
			return nil, err
		}
		if !scopeMatches(scope, scopeFilter) {
			continue
		}
		dirs = append(dirs, path)
	}
	sort.Strings(dirs)
	return dirs, nil
}

// scopeMatches reports whether a scope list contains the requested filter.
// Empty list = universal (matches every filter). Empty filter = no filter
// (matches everything).
func scopeMatches(scope []string, filter string) bool {
	if filter == "" || len(scope) == 0 {
		return true
	}
	for _, s := range scope {
		if s == filter {
			return true
		}
	}
	return false
}

// readSkillScope returns the parsed scope list from SKILL.md frontmatter,
// or nil if the file/field is absent. Accepts CSV (`scope: home, work`).
// Each entry must be a known profile name; unknown values error out.
func readSkillScope(skillDir string) ([]string, error) {
	path := filepath.Join(skillDir, "SKILL.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	const sep = "---\n"
	if !bytes.HasPrefix(raw, []byte(sep)) {
		return nil, nil
	}
	rest := raw[len(sep):]
	end := bytes.Index(rest, []byte("\n"+sep))
	if end < 0 {
		return nil, nil
	}
	for _, line := range strings.Split(string(rest[:end]), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		if strings.TrimSpace(line[:idx]) != "scope" {
			continue
		}
		list := schema.SplitCSV(strings.TrimSpace(line[idx+1:]))
		for _, s := range list {
			if !schema.ValidScope(s) {
				return nil, fmt.Errorf("%s: unknown scope %q (must be \"home\" or \"work\")", path, s)
			}
		}
		return list, nil
	}
	return nil, nil
}

// CopyDir recursively copies src to dst. Removes dst first if it already
// exists so stale files from a previous render don't linger.
func CopyDir(src, dst string) error {
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

// ReadInstructions returns the body of <sourceRoot>/instructions/global.md
// and, if present, the body of <sourceRoot>/instructions/<targetPrefix>.md.
// Renderers concatenate prefix + global with a `---` divider.
func ReadInstructions(sourceRoot, targetPrefix string) (global, prefix string, err error) {
	globalPath := filepath.Join(sourceRoot, "instructions", "global.md")
	g, err := os.ReadFile(globalPath)
	if err != nil {
		return "", "", fmt.Errorf("reading %s: %w", globalPath, err)
	}
	if targetPrefix != "" {
		prefixPath := filepath.Join(sourceRoot, "instructions", targetPrefix+".md")
		p, perr := os.ReadFile(prefixPath)
		if perr == nil {
			prefix = string(p)
		} else if !os.IsNotExist(perr) {
			return "", "", fmt.Errorf("reading %s: %w", prefixPath, perr)
		}
	}
	return string(g), prefix, nil
}

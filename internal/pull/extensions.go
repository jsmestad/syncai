package pull

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ExtPlan describes how to pull a Pi extension's installed content back
// into the source tree. Extensions are verbatim TS code; the diff is
// strictly byte-level, with no field-level reverse mapping (unlike agents).
type ExtPlan struct {
	Name        string
	IsDirectory bool

	// InstalledPath is the install location, e.g.
	// ~/.pi/agent/extensions/btw.ts or ~/.pi/agent/extensions/review-loop.
	InstalledPath string

	// SourcePath is where the canonical version lives, e.g.
	// ai-source/extensions/btw.ts or ai-source/extensions/review-loop.
	SourcePath string

	// Changes lists every file that differs between install and source.
	// For single-file extensions there's at most one entry. For directory
	// extensions there may be many, each with the relative path inside
	// the extension and a kind describing the change.
	Changes []ExtFileChange
}

// ExtFileChange is one file-level diff inside an extension.
type ExtFileChange struct {
	// RelPath is "" for single-file extensions and a relative path
	// like "tests/foo.test.ts" for files inside a directory extension.
	RelPath string

	// Kind explains what changed:
	//   "edited"  installed bytes differ from source bytes
	//   "added"   file exists in install but not source
	//   "removed" file exists in source but not install (handled by skip
	//             — pulling deletes is dangerous; user can rm manually)
	Kind string
}

// HasChanges reports whether Apply would write anything.
func (p ExtPlan) HasChanges() bool {
	for _, c := range p.Changes {
		if c.Kind == "edited" || c.Kind == "added" {
			return true
		}
	}
	return false
}

// PlanExtension builds an ExtPlan by comparing every installed file
// against the corresponding source file. The "extension.toml" sidecar in
// the source directory is preserved and never reported as missing — it's
// build-time metadata that intentionally never appears in the install tree.
func PlanExtension(name, installedPath, sourcePath string, isDirectory bool) (ExtPlan, error) {
	plan := ExtPlan{
		Name:          name,
		IsDirectory:   isDirectory,
		InstalledPath: installedPath,
		SourcePath:    sourcePath,
	}
	if !isDirectory {
		change, err := compareFile(installedPath, sourcePath, "")
		if err != nil {
			return plan, err
		}
		if change != nil {
			plan.Changes = append(plan.Changes, *change)
		}
		return plan, nil
	}

	// Directory: walk install and source, compute per-file changes.
	installFiles, err := walkExtFiles(installedPath)
	if err != nil {
		return plan, err
	}
	sourceFiles, err := walkExtFiles(sourcePath)
	if err != nil {
		return plan, err
	}
	// "added" + "edited" relative to install side.
	for rel := range installFiles {
		change, err := compareFile(
			filepath.Join(installedPath, rel),
			filepath.Join(sourcePath, rel),
			rel,
		)
		if err != nil {
			return plan, err
		}
		if change != nil {
			plan.Changes = append(plan.Changes, *change)
		}
	}
	// "removed" relative to install side, ignoring sidecars.
	for rel := range sourceFiles {
		if rel == "extension.toml" {
			continue
		}
		if _, present := installFiles[rel]; !present {
			plan.Changes = append(plan.Changes, ExtFileChange{RelPath: rel, Kind: "removed"})
		}
	}
	sort.Slice(plan.Changes, func(i, j int) bool { return plan.Changes[i].RelPath < plan.Changes[j].RelPath })
	return plan, nil
}

// Apply writes installed bytes back into source for every "edited" or
// "added" change. "removed" changes are reported but not actioned —
// pulling a deletion is hazardous (a transient install glitch could wipe
// canonical source) and the user can `rm` deliberately if intended.
func (p ExtPlan) Apply() error {
	for _, c := range p.Changes {
		switch c.Kind {
		case "edited", "added":
			src := filepath.Join(p.InstalledPath, c.RelPath)
			dst := filepath.Join(p.SourcePath, c.RelPath)
			if !p.IsDirectory {
				src = p.InstalledPath
				dst = p.SourcePath
			}
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return err
			}
			if err := copyExtPullFile(src, dst); err != nil {
				return fmt.Errorf("copying %s: %w", src, err)
			}
		}
	}
	return nil
}

// compareFile returns a non-nil change if installed and source differ.
// A missing source means "added" (install has new file). A missing install
// is reported as nil here because the caller's source-walk pass handles
// "removed" cases.
func compareFile(installPath, sourcePath, rel string) (*ExtFileChange, error) {
	installBytes, err := os.ReadFile(installPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	sourceBytes, sErr := os.ReadFile(sourcePath)
	if sErr != nil {
		if os.IsNotExist(sErr) {
			return &ExtFileChange{RelPath: rel, Kind: "added"}, nil
		}
		return nil, sErr
	}
	if string(installBytes) == string(sourceBytes) {
		return nil, nil
	}
	return &ExtFileChange{RelPath: rel, Kind: "edited"}, nil
}

// walkExtFiles returns the relative paths of every regular file under root.
// Symlinks are excluded so external files aren't accidentally vendored on
// pull (matches the importer's filter).
func walkExtFiles(root string) (map[string]struct{}, error) {
	out := map[string]struct{}{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.IsDir() {
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		out[rel] = struct{}{}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return out, nil
}

func copyExtPullFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}

// FormatChange is a short human-readable summary of one change. Used by
// the CLI to print drift descriptions.
func FormatChange(c ExtFileChange) string {
	if c.RelPath == "" {
		return c.Kind
	}
	return fmt.Sprintf("%s: %s", c.Kind, c.RelPath)
}

// SummariseChanges produces a one-line summary for the CLI listing,
// e.g. "edited (3), added (1)".
func SummariseChanges(changes []ExtFileChange) string {
	counts := map[string]int{}
	for _, c := range changes {
		counts[c.Kind]++
	}
	parts := []string{}
	for _, kind := range []string{"edited", "added", "removed"} {
		if n := counts[kind]; n > 0 {
			parts = append(parts, fmt.Sprintf("%s (%d)", kind, n))
		}
	}
	if len(parts) == 0 {
		return "no changes"
	}
	return strings.Join(parts, ", ")
}

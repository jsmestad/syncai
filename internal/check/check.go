// Package check compares two file trees and reports per-file drift.
// Used by `syncai render --check` to validate generated files against
// what's currently committed without writing anything to disk.
package check

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Diff is one drift entry between expected and actual trees.
type Diff struct {
	Kind string // "missing" | "extra" | "stale"
	Path string // relative to compare root
}

func (d Diff) String() string { return fmt.Sprintf("%s: %s", d.Kind, d.Path) }

// Trees walks expectedRoot and reports drift in actualRoot. Each file in
// expectedRoot must exist byte-identically under actualRoot (otherwise
// "missing" or "stale"). Files in actualRoot that aren't under expectedRoot
// are intentionally ignored — the rendered tree is co-mingled with files
// owned by Pi/Stow/the user, and we only own what we wrote.
func Trees(expectedRoot, actualRoot string) ([]Diff, error) {
	expected, err := walk(expectedRoot)
	if err != nil {
		return nil, err
	}
	var diffs []Diff
	for _, rel := range expected {
		ePath := filepath.Join(expectedRoot, rel)
		aPath := filepath.Join(actualRoot, rel)
		eData, err := os.ReadFile(ePath)
		if err != nil {
			return nil, err
		}
		aData, aerr := os.ReadFile(aPath)
		if aerr != nil {
			if os.IsNotExist(aerr) {
				diffs = append(diffs, Diff{Kind: "missing", Path: rel})
				continue
			}
			return nil, aerr
		}
		if !bytes.Equal(eData, aData) {
			diffs = append(diffs, Diff{Kind: "stale", Path: rel})
		}
	}
	sort.Slice(diffs, func(i, j int) bool {
		if diffs[i].Kind != diffs[j].Kind {
			return diffs[i].Kind < diffs[j].Kind
		}
		return diffs[i].Path < diffs[j].Path
	})
	return diffs, nil
}

func walk(root string) ([]string, error) {
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil, nil
	}
	var out []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			// Skip hidden dirs that aren't part of the rendered tree (none for us
			// since each renderer roots itself at .pi/.claude/.codex/etc.).
			return nil
		}
		// Skip macOS junk that can creep into committed trees.
		if strings.HasSuffix(path, "/.DS_Store") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		out = append(out, rel)
		return nil
	})
	sort.Strings(out)
	return out, err
}

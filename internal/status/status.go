// Package status compares the on-disk install against what syncai would
// render now from current source, and against the manifest of what syncai
// last wrote. The output is a workflow-friendly report:
//
//   - drifted: tracked files that have local edits since last render
//   - missing: tracked files that have been deleted locally
//   - stale:   files in the manifest that the next render would no longer write
//
// Drift detection compares current installed bytes against a fresh render
// into a tempdir. Hash compare via byte equality.
package status

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/jsmestad/syncai/internal/manifest"
)

// Report classifies every manifest-tracked file.
type Report struct {
	Drifted []string // file paths with local edits since last render
	Missing []string // file paths in manifest but no longer present on disk
	Stale   []string // file paths in manifest that the new render won't reproduce
}

// HasChanges reports whether the workflow needs attention.
func (r Report) HasChanges() bool {
	return len(r.Drifted)+len(r.Missing)+len(r.Stale) > 0
}

// CompareWithRoots walks the manifest and a fresh-render tempdir to
// classify every tracked file. freshRoot is where a fresh render was
// written (tempdir); installRoot is the live install (usually $HOME). For
// each manifest path, the equivalent under freshRoot is the relative path
// from installRoot.
//
// Drift is detected via the manifest's recorded SHA-256 against the
// current on-disk content (so a recent source edit doesn't masquerade as
// drift on tools that haven't been re-rendered yet). Stale is detected via
// "render no longer produces this path" — a fresh-render comparison.
func CompareWithRoots(old *manifest.Manifest, freshRoot, installRoot string) (Report, error) {
	var r Report
	for _, file := range old.Files {
		actualBody, actualErr := os.ReadFile(file.Path)
		switch {
		case actualErr != nil && os.IsNotExist(actualErr):
			r.Missing = append(r.Missing, file.Path)
			continue
		case actualErr != nil:
			return Report{}, fmt.Errorf("reading %s: %w", file.Path, actualErr)
		}

		rel, err := filepath.Rel(installRoot, file.Path)
		if err != nil {
			continue
		}
		freshPath := filepath.Join(freshRoot, rel)
		_, freshErr := os.ReadFile(freshPath)
		willRender := freshErr == nil

		switch {
		case !willRender:
			r.Stale = append(r.Stale, file.Path)
		case manifest.HashBytes(actualBody) != file.Hash:
			r.Drifted = append(r.Drifted, file.Path)
		}
	}
	sort.Strings(r.Drifted)
	sort.Strings(r.Missing)
	sort.Strings(r.Stale)
	return r, nil
}

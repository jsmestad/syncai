// Package pathguard validates filesystem mutation targets against a declared root.
package pathguard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Resolve returns candidate as an absolute path when both its lexical path and
// its symlink-resolved existing ancestors remain within root.
func Resolve(root, candidate string) (string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolving candidate %q within root %q: %w", candidate, root, err)
	}
	absRoot = filepath.Clean(absRoot)

	absCandidate := candidate
	if !filepath.IsAbs(absCandidate) {
		absCandidate = filepath.Join(absRoot, absCandidate)
	}
	absCandidate, err = filepath.Abs(absCandidate)
	if err != nil {
		return "", fmt.Errorf("resolving candidate %q within root %q: %w", candidate, root, err)
	}
	absCandidate = filepath.Clean(absCandidate)

	if !contained(absRoot, absCandidate) {
		return "", fmt.Errorf("candidate %q is outside root %q", candidate, root)
	}

	resolvedRoot, err := resolveExisting(absRoot)
	if err != nil {
		return "", fmt.Errorf("resolving candidate %q within root %q: %w", candidate, root, err)
	}
	resolvedCandidate, err := resolveExisting(absCandidate)
	if err != nil {
		return "", fmt.Errorf("resolving candidate %q within root %q: %w", candidate, root, err)
	}
	if !contained(resolvedRoot, resolvedCandidate) {
		return "", fmt.Errorf("candidate %q resolves outside root %q", candidate, root)
	}

	return absCandidate, nil
}

func contained(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator)) && !filepath.IsAbs(relative)
}

func resolveExisting(path string) (string, error) {
	current := path
	var missing []string
	for {
		_, err := os.Lstat(current)
		if err == nil {
			resolved, err := filepath.EvalSymlinks(current)
			if err != nil {
				return "", err
			}
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return filepath.Clean(resolved), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", err
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

package load

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jsmestad/syncai/internal/pathguard"
)

// MkdirAll creates candidate and its parents after proving that the mutation
// remains within root.
func MkdirAll(root, candidate string, perm os.FileMode) error {
	path, err := pathguard.Resolve(root, candidate)
	if err != nil {
		return err
	}
	return os.MkdirAll(path, perm)
}

// WriteFileReplacing writes data to path, replacing whatever is currently
// there (regular file, broken symlink, stale stow link, etc.). Without the
// explicit Remove, os.WriteFile follows symlinks and fails when the target
// doesn't exist — common when migrating off a stow-based dotfiles flow.
func WriteFileReplacing(root, candidate string, data []byte, perm os.FileMode) error {
	path, err := resolveReplacement(root, candidate)
	if err != nil {
		return err
	}
	if err := MkdirAll(root, filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// Lstat doesn't follow symlinks; we want to know if *anything* is at
	// the path so we can clear it.
	writePerm := perm
	if info, err := os.Lstat(path); err == nil {
		if info.Mode().IsRegular() {
			writePerm = info.Mode().Perm()
		}
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("removing stale %s: %w", path, err)
		}
	}
	return os.WriteFile(path, data, writePerm)
}

func resolveReplacement(root, candidate string) (string, error) {
	cleanCandidate := filepath.Clean(candidate)
	parent, err := pathguard.Resolve(root, filepath.Dir(cleanCandidate))
	if err != nil {
		return "", fmt.Errorf("resolving replacement candidate %q within root %q: %w", candidate, root, err)
	}
	path := filepath.Join(parent, filepath.Base(cleanCandidate))
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolving replacement root %q: %w", root, err)
	}
	if filepath.Clean(path) == filepath.Clean(absRoot) {
		return "", fmt.Errorf("replacement candidate %q must be below root %q", candidate, root)
	}
	return path, nil
}

// CopyFileReplacing is the file equivalent of CopyDir's clear-then-write
// behaviour. Uses WriteFileReplacing for the same symlink-safety reasons.
func CopyFileReplacing(root, src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return WriteFileReplacing(root, dst, data, 0o644)
}

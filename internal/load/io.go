package load

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteFileReplacing writes data to path, replacing whatever is currently
// there (regular file, broken symlink, stale stow link, etc.). Without the
// explicit Remove, os.WriteFile follows symlinks and fails when the target
// doesn't exist — common when migrating off a stow-based dotfiles flow.
func WriteFileReplacing(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// Lstat doesn't follow symlinks; we want to know if *anything* is at
	// the path so we can clear it.
	if _, err := os.Lstat(path); err == nil {
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("removing stale %s: %w", path, err)
		}
	}
	return os.WriteFile(path, data, perm)
}

// CopyFileReplacing is the file equivalent of CopyDir's clear-then-write
// behaviour. Uses WriteFileReplacing for the same symlink-safety reasons.
func CopyFileReplacing(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return WriteFileReplacing(dst, data, 0o644)
}

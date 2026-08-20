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

// WriteFileReplacing atomically writes data to path, replacing whatever is
// currently there (regular file, broken symlink, stale stow link, etc.). The
// temporary file lives beside the destination so Rename cannot cross devices.
func WriteFileReplacing(root, candidate string, data []byte, perm os.FileMode) error {
	path, err := resolveReplacement(root, candidate)
	if err != nil {
		return err
	}
	if err := MkdirAll(root, filepath.Dir(path), 0o755); err != nil {
		return err
	}
	writePerm := perm
	if info, err := os.Lstat(path); err == nil {
		if info.Mode().IsRegular() {
			writePerm = info.Mode().Perm()
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspecting replacement target %s: %w", path, err)
	}

	temporary, err := os.CreateTemp(filepath.Dir(path), ".syncai-write-*")
	if err != nil {
		return fmt.Errorf("creating temporary file for %s: %w", path, err)
	}
	temporaryPath := temporary.Name()
	closed := false
	defer func() {
		if !closed {
			_ = temporary.Close()
		}
		_ = os.Remove(temporaryPath)
	}()

	if _, err := temporary.Write(data); err != nil {
		return fmt.Errorf("writing temporary file for %s: %w", path, err)
	}
	if err := temporary.Chmod(writePerm.Perm()); err != nil {
		return fmt.Errorf("applying permissions to temporary file for %s: %w", path, err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("syncing temporary file for %s: %w", path, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("closing temporary file for %s: %w", path, err)
	}
	closed = true
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replacing %s: %w", path, err)
	}
	return nil
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

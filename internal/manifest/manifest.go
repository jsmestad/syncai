// Package manifest tracks every file and directory syncai created in its
// last render so a subsequent render can prune anything we previously wrote
// but no longer would. It only deletes paths it has on record — files the
// user placed by hand are never touched.
//
// Each file entry carries a SHA-256 of the content as we wrote it. Drift
// detection compares the current file's hash against the recorded hash, so
// "we just changed source" (manifest hash already reflects new content
// after the previous render) is distinguishable from "the user edited the
// installed file" (current bytes differ from the recorded hash).
package manifest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Version 2 introduces FileEntry with hashes. Older manifests (Version 1)
// load successfully but Files is empty; first run after upgrade behaves
// like a fresh install — nothing pruned, no drift detected.
const Version = 2

// FileEntry pairs a written file path with the SHA-256 of its content.
type FileEntry struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
}

// Manifest is the on-disk record of a render's outputs.
type Manifest struct {
	Version     int         `json:"version"`
	Scope       string      `json:"scope,omitempty"`
	RenderedAt  time.Time   `json:"renderedAt"`
	Files       []FileEntry `json:"files"`
	Directories []string    `json:"directories"`
}

// DefaultPath returns where the manifest is stored. Honours XDG_STATE_HOME
// per the freedesktop spec; falls back to ~/.local/state/syncai/manifest.json.
func DefaultPath() (string, error) {
	root := os.Getenv("XDG_STATE_HOME")
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(root, "syncai", "manifest.json"), nil
}

// Load reads the manifest at path. A missing file is not an error; returns
// an empty manifest so first-run renders behave correctly. v1 manifests
// (path-only, no hashes) load successfully but Files is reset to empty so
// the upgrade doesn't trigger spurious drift on every previously-rendered
// file.
func Load(path string) (*Manifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Manifest{Version: Version}, nil
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	// Probe version first; v1 had Files []string, v2 has Files []FileEntry.
	// Decoding directly into Manifest fails on v1 because of the Files type
	// mismatch.
	var probe struct {
		Version     int      `json:"version"`
		Scope       string   `json:"scope"`
		Directories []string `json:"directories"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if probe.Version < Version {
		return &Manifest{
			Version:     Version,
			Scope:       probe.Scope,
			Directories: probe.Directories,
		}, nil
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &m, nil
}

// Save writes the manifest atomically (temp file + rename) so a crash mid-
// write can't corrupt it.
func Save(path string, m *Manifest) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if m.Version == 0 {
		m.Version = Version
	}
	if m.RenderedAt.IsZero() {
		m.RenderedAt = time.Now().UTC()
	}
	sort.Slice(m.Files, func(i, j int) bool { return m.Files[i].Path < m.Files[j].Path })
	sort.Strings(m.Directories)
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// HashFile returns the hex-encoded SHA-256 of the file at path.
func HashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return HashBytes(data), nil
}

// HashBytes returns the hex-encoded SHA-256 of data.
func HashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// FilePaths returns just the paths from m.Files, useful for diffing.
func (m *Manifest) FilePaths() []string {
	out := make([]string, 0, len(m.Files))
	for _, f := range m.Files {
		out = append(out, f.Path)
	}
	return out
}

// Drifted reports manifest-tracked files whose current on-disk hash differs
// from the recorded hash. Missing files (deleted under us) and unreadable
// files are reported separately. Used by the render drift guard and by
// `syncai status`.
func Drifted(m *Manifest) (drifted, missing []string, err error) {
	for _, f := range m.Files {
		actual, hErr := HashFile(f.Path)
		if hErr != nil {
			if os.IsNotExist(hErr) {
				missing = append(missing, f.Path)
				continue
			}
			return nil, nil, hErr
		}
		if actual != f.Hash {
			drifted = append(drifted, f.Path)
		}
	}
	sort.Strings(drifted)
	sort.Strings(missing)
	return drifted, missing, nil
}

// Diff returns the paths in old that are not in next. These are the files
// and directories that should be removed before the new render is written.
func Diff(old, next *Manifest) (filesToRemove, dirsToRemove []string) {
	nextFiles := map[string]bool{}
	for _, f := range next.Files {
		nextFiles[f.Path] = true
	}
	for _, f := range old.Files {
		if !nextFiles[f.Path] {
			filesToRemove = append(filesToRemove, f.Path)
		}
	}
	nextDirs := stringSet(next.Directories)
	for _, d := range old.Directories {
		if !nextDirs[d] {
			dirsToRemove = append(dirsToRemove, d)
		}
	}
	return filesToRemove, dirsToRemove
}

// Prune removes files and directories that are no longer rendered. Best-
// effort: it logs (returns) errors per path but doesn't stop on them.
func Prune(filesToRemove, dirsToRemove []string) []error {
	var errs []error
	for _, f := range filesToRemove {
		if _, err := os.Lstat(f); os.IsNotExist(err) {
			continue
		}
		if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("removing %s: %w", f, err))
		}
	}
	for _, d := range dirsToRemove {
		if _, err := os.Lstat(d); os.IsNotExist(err) {
			continue
		}
		if err := os.RemoveAll(d); err != nil {
			errs = append(errs, fmt.Errorf("removing %s: %w", d, err))
		}
	}
	return errs
}

func stringSet(in []string) map[string]bool {
	out := make(map[string]bool, len(in))
	for _, s := range in {
		out[s] = true
	}
	return out
}

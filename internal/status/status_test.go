package status

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jsmestad/syncai/internal/manifest"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// ST1: Manifest file present in both, byte-equal: not classified.
func TestCompareCleanIsEmpty(t *testing.T) {
	d := t.TempDir()
	install := filepath.Join(d, "home")
	fresh := filepath.Join(d, "fresh")
	writeFile(t, filepath.Join(install, ".claude/agents/x.md"), "canonical")
	writeFile(t, filepath.Join(fresh, ".claude/agents/x.md"), "canonical")
	m := &manifest.Manifest{Files: []manifest.FileEntry{
		{Path: filepath.Join(install, ".claude/agents/x.md"), Hash: manifest.HashBytes([]byte("canonical"))},
	}}
	r, err := CompareWithRoots(m, fresh, install)
	if err != nil {
		t.Fatal(err)
	}
	if r.HasChanges() {
		t.Errorf("expected clean, got %+v", r)
	}
}

// ST2: Bytes differ → Drifted.
func TestCompareDrifted(t *testing.T) {
	d := t.TempDir()
	install := filepath.Join(d, "home")
	fresh := filepath.Join(d, "fresh")
	target := filepath.Join(install, ".claude/agents/x.md")
	writeFile(t, target, "EDITED")
	writeFile(t, filepath.Join(fresh, ".claude/agents/x.md"), "canonical")
	m := &manifest.Manifest{Files: []manifest.FileEntry{
		{Path: target, Hash: manifest.HashBytes([]byte("canonical"))}, // recorded as the original
	}}
	r, err := CompareWithRoots(m, fresh, install)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Drifted) != 1 || r.Drifted[0] != target {
		t.Errorf("expected drift at %s, got %+v", target, r)
	}
}

// ST3: Manifest file missing from installRoot → Missing.
func TestCompareMissing(t *testing.T) {
	d := t.TempDir()
	install := filepath.Join(d, "home")
	fresh := filepath.Join(d, "fresh")
	writeFile(t, filepath.Join(fresh, ".claude/agents/x.md"), "canonical")
	target := filepath.Join(install, ".claude/agents/x.md")
	m := &manifest.Manifest{Files: []manifest.FileEntry{
		{Path: target, Hash: manifest.HashBytes([]byte("canonical"))},
	}}
	r, err := CompareWithRoots(m, fresh, install)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Missing) != 1 || r.Missing[0] != target {
		t.Errorf("expected missing at %s, got %+v", target, r)
	}
}

// ST4: Manifest file no longer rendered → Stale.
func TestCompareStale(t *testing.T) {
	d := t.TempDir()
	install := filepath.Join(d, "home")
	fresh := filepath.Join(d, "fresh")
	target := filepath.Join(install, ".claude/agents/old.md")
	writeFile(t, target, "still installed")
	// fresh dir has no equivalent file
	writeFile(t, filepath.Join(fresh, ".claude/agents/other.md"), "current")

	m := &manifest.Manifest{Files: []manifest.FileEntry{
		{Path: target, Hash: manifest.HashBytes([]byte("still installed"))},
	}}
	r, err := CompareWithRoots(m, fresh, install)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Stale) != 1 || r.Stale[0] != target {
		t.Errorf("expected stale at %s, got %+v", target, r)
	}
}

// ST7: HasChanges returns true iff any of Drifted/Missing/Stale is non-empty.
func TestHasChanges(t *testing.T) {
	if (Report{}).HasChanges() {
		t.Errorf("empty report should be unchanged")
	}
	if !(Report{Drifted: []string{"x"}}).HasChanges() {
		t.Errorf("drift should count")
	}
	if !(Report{Missing: []string{"x"}}).HasChanges() {
		t.Errorf("missing should count")
	}
	if !(Report{Stale: []string{"x"}}).HasChanges() {
		t.Errorf("stale should count")
	}
}

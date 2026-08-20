package manifest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// M1: Diff returns paths in old missing from next, both files and dirs.
func TestDiffReportsRemovedFilesAndDirs(t *testing.T) {
	old := &Manifest{
		Files: []FileEntry{
			{Path: "/a/x.md", Hash: "h1"},
			{Path: "/a/y.md", Hash: "h2"},
		},
		Directories: []string{"/a/skill1", "/a/skill2"},
	}
	next := &Manifest{
		Files:       []FileEntry{{Path: "/a/x.md", Hash: "h1"}},
		Directories: []string{"/a/skill1"},
	}
	files, dirs := Diff(old, next)
	if len(files) != 1 || files[0] != "/a/y.md" {
		t.Errorf("files: got %v, want [/a/y.md]", files)
	}
	if len(dirs) != 1 || dirs[0] != "/a/skill2" {
		t.Errorf("dirs: got %v, want [/a/skill2]", dirs)
	}
}

// M2: Diff returns empty when next is a superset.
func TestDiffEmptyWhenNextSuperset(t *testing.T) {
	old := &Manifest{Files: []FileEntry{{Path: "/a/x.md", Hash: "h1"}}}
	next := &Manifest{
		Files: []FileEntry{
			{Path: "/a/x.md", Hash: "h1"},
			{Path: "/a/y.md", Hash: "h2"},
		},
	}
	files, dirs := Diff(old, next)
	if len(files) != 0 || len(dirs) != 0 {
		t.Errorf("expected no removals, got files=%v dirs=%v", files, dirs)
	}
}

// M3: Diff is empty when old is empty (first-run case — nothing to prune).
func TestDiffEmptyWhenOldEmpty(t *testing.T) {
	old := &Manifest{}
	next := &Manifest{Files: []FileEntry{{Path: "/a/x.md", Hash: "h1"}}}
	files, dirs := Diff(old, next)
	if len(files) != 0 || len(dirs) != 0 {
		t.Errorf("first-run diff should be empty, got files=%v dirs=%v", files, dirs)
	}
}

// M4: Prune deletes only files in its argument list.
func TestPruneOnlyDeletesArgumentedPaths(t *testing.T) {
	d := t.TempDir()
	doomed := filepath.Join(d, "doomed.md")
	survives := filepath.Join(d, "survives.md")
	mustWrite(t, doomed, "x")
	mustWrite(t, survives, "y")

	if errs := Prune([]string{doomed}, nil); len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if _, err := os.Stat(doomed); !os.IsNotExist(err) {
		t.Errorf("doomed file should have been removed; err=%v", err)
	}
	if _, err := os.Stat(survives); err != nil {
		t.Errorf("unrelated file should survive: %v", err)
	}
}

// M5: Prune is a no-op for already-missing paths.
func TestPruneNoopForMissing(t *testing.T) {
	d := t.TempDir()
	missing := filepath.Join(d, "ghost.md")
	if errs := Prune([]string{missing}, nil); len(errs) > 0 {
		t.Errorf("Prune of missing path should not error: %v", errs)
	}
}

// M7: Save then Load round-trips Files/Directories/Scope/Version.
func TestSaveLoadRoundTrip(t *testing.T) {
	d := t.TempDir()
	path := filepath.Join(d, "manifest.json")
	original := &Manifest{
		Scope: "home",
		Files: []FileEntry{
			{Path: "/a/x.md", Hash: "abc123"},
			{Path: "/a/y.md", Hash: "def456"},
		},
		Directories: []string{"/a/skill1"},
	}
	if err := Save(path, original); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Scope != "home" {
		t.Errorf("Scope: got %q, want home", loaded.Scope)
	}
	if loaded.Version != Version {
		t.Errorf("Version: got %d, want %d", loaded.Version, Version)
	}
	if len(loaded.Files) != 2 || loaded.Files[0].Hash != "abc123" {
		t.Errorf("Files round-trip wrong: %+v", loaded.Files)
	}
	if len(loaded.Directories) != 1 || loaded.Directories[0] != "/a/skill1" {
		t.Errorf("Directories round-trip wrong: %+v", loaded.Directories)
	}
}

// M8: Load of missing file returns empty manifest, no error.
func TestLoadMissingReturnsEmpty(t *testing.T) {
	d := t.TempDir()
	path := filepath.Join(d, "no-such-file.json")
	m, err := Load(path)
	if err != nil {
		t.Fatalf("Load of missing file should not error: %v", err)
	}
	if len(m.Files) != 0 || len(m.Directories) != 0 {
		t.Errorf("expected empty manifest, got %+v", m)
	}
	if m.Version != Version {
		t.Errorf("Version on empty manifest: got %d, want %d", m.Version, Version)
	}
}

// M9: Load of malformed JSON errors. Critical guard against silent loss.
func TestLoadMalformedErrors(t *testing.T) {
	d := t.TempDir()
	path := filepath.Join(d, "bad.json")
	mustWrite(t, path, "{ this is not json")
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected Load to error on malformed JSON, got nil")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error should name the file: %v", err)
	}
}

// M10: Save uses tempfile + rename (atomic). A leftover .tmp file from an
// earlier failed write doesn't end up as the final manifest.
func TestSaveIsAtomic(t *testing.T) {
	d := t.TempDir()
	path := filepath.Join(d, "manifest.json")
	mustWrite(t, path+".tmp", `{"version":99,"files":[]}`)
	m := &Manifest{Files: []FileEntry{{Path: "/a/x.md", Hash: "abc"}}}
	if err := Save(path, m); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Version != Version {
		t.Errorf("Version reflects new write, not stale .tmp: got %d", loaded.Version)
	}
}

// M11: Save sorts entries so manifests are diff-friendly.
func TestSaveSortsEntries(t *testing.T) {
	d := t.TempDir()
	path := filepath.Join(d, "manifest.json")
	m := &Manifest{
		Files: []FileEntry{
			{Path: "/z.md", Hash: "z"},
			{Path: "/a.md", Hash: "a"},
			{Path: "/m.md", Hash: "m"},
		},
		Directories: []string{"/z-dir", "/a-dir"},
	}
	if err := Save(path, m); err != nil {
		t.Fatalf("Save: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var disk Manifest
	if err := json.Unmarshal(raw, &disk); err != nil {
		t.Fatal(err)
	}
	if disk.Files[0].Path != "/a.md" || disk.Files[2].Path != "/z.md" {
		t.Errorf("Files not sorted: %+v", disk.Files)
	}
	if disk.Directories[0] != "/a-dir" || disk.Directories[1] != "/z-dir" {
		t.Errorf("Directories not sorted: %+v", disk.Directories)
	}
}

// V1 manifests load successfully but Files is reset (no hashes available),
// so the upgrade does not trigger spurious drift on every previously-
// rendered file.
func TestLoadV1ManifestDropsFiles(t *testing.T) {
	d := t.TempDir()
	path := filepath.Join(d, "manifest.json")
	v1 := `{"version":1,"files":["/old/path/one.md","/old/path/two.md"],"directories":["/old/skill"]}`
	mustWrite(t, path, v1)
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Version != Version {
		t.Errorf("Version: got %d, want %d", loaded.Version, Version)
	}
	if len(loaded.Files) != 0 {
		t.Errorf("v1 Files should reset to empty, got %d entries", len(loaded.Files))
	}
	if len(loaded.Directories) != 1 {
		t.Errorf("v1 Directories should be preserved, got %d", len(loaded.Directories))
	}
}

// Drifted detects content drift via hash compare. Files matching the
// recorded hash are clean; mismatches are drifted; missing files are
// reported separately.
func TestDriftedDetectsLocalEdits(t *testing.T) {
	d := t.TempDir()
	clean := filepath.Join(d, "clean.md")
	drifted := filepath.Join(d, "drifted.md")
	missing := filepath.Join(d, "missing.md") // never created
	mustWrite(t, clean, "canonical")
	mustWrite(t, drifted, "canonical-but-now-different")

	cleanHash := HashBytes([]byte("canonical"))
	driftedRecorded := HashBytes([]byte("canonical")) // recorded as the original
	missingRecorded := HashBytes([]byte("anything"))
	m := &Manifest{Files: []FileEntry{
		{Path: clean, Hash: cleanHash},
		{Path: drifted, Hash: driftedRecorded},
		{Path: missing, Hash: missingRecorded},
	}}
	driftedPaths, missingPaths, err := Drifted(m)
	if err != nil {
		t.Fatalf("Drifted: %v", err)
	}
	if len(driftedPaths) != 1 || driftedPaths[0] != drifted {
		t.Errorf("expected one drift at %s, got %v", drifted, driftedPaths)
	}
	if len(missingPaths) != 1 || missingPaths[0] != missing {
		t.Errorf("expected one missing at %s, got %v", missing, missingPaths)
	}
}

// HashBytes is deterministic and produces a 64-char hex string (sha256).
func TestHashBytesShape(t *testing.T) {
	h := HashBytes([]byte("hello"))
	if len(h) != 64 {
		t.Errorf("sha256 hex should be 64 chars, got %d", len(h))
	}
	if HashBytes([]byte("hello")) != h {
		t.Errorf("HashBytes should be deterministic")
	}
	if HashBytes([]byte("hellp")) == h {
		t.Errorf("HashBytes should differ for different inputs")
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

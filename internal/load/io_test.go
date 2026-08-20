package load

import (
	"os"
	"path/filepath"
	"testing"
)

// L1: WriteFileReplacing creates a new file when path doesn't exist.
func TestWriteFileReplacingNew(t *testing.T) {
	d := t.TempDir()
	path := filepath.Join(d, "new.md")
	if err := WriteFileReplacing(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello" {
		t.Errorf("content: %q", got)
	}
}

// L2: WriteFileReplacing overwrites an existing regular file.
func TestWriteFileReplacingExisting(t *testing.T) {
	d := t.TempDir()
	path := filepath.Join(d, "existing.md")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileReplacing(path, []byte("new"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "new" {
		t.Errorf("content not overwritten: %q", got)
	}
}

// L3: WriteFileReplacing replaces a broken symlink with a real file. This
// is the bug WriteFileReplacing exists to fix — os.WriteFile follows the
// symlink and fails when the target doesn't exist.
func TestWriteFileReplacingBrokenSymlink(t *testing.T) {
	d := t.TempDir()
	path := filepath.Join(d, "link.md")
	if err := os.Symlink(filepath.Join(d, "does-not-exist"), path); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileReplacing(path, []byte("real content"), 0o644); err != nil {
		t.Fatalf("write through broken symlink: %v", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() {
		t.Errorf("expected regular file after replace, got mode %v", info.Mode())
	}
	got, _ := os.ReadFile(path)
	if string(got) != "real content" {
		t.Errorf("content: %q", got)
	}
}

// L4: WriteFileReplacing replaces a valid symlink, leaving the original
// target untouched.
func TestWriteFileReplacingValidSymlinkPreservesTarget(t *testing.T) {
	d := t.TempDir()
	target := filepath.Join(d, "target.md")
	link := filepath.Join(d, "link.md")
	if err := os.WriteFile(target, []byte("target content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileReplacing(link, []byte("link replacement"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	// The link path should now be a regular file.
	linkInfo, _ := os.Lstat(link)
	if !linkInfo.Mode().IsRegular() {
		t.Errorf("link should now be a regular file")
	}
	// The target should still have its original content.
	tgt, _ := os.ReadFile(target)
	if string(tgt) != "target content" {
		t.Errorf("target file content was modified: %q", tgt)
	}
}

// L5: WriteFileReplacing creates parent directories.
func TestWriteFileReplacingCreatesParents(t *testing.T) {
	d := t.TempDir()
	deep := filepath.Join(d, "a", "b", "c", "deep.md")
	if err := WriteFileReplacing(deep, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := os.Stat(deep); err != nil {
		t.Errorf("file not created: %v", err)
	}
}

// L6: CopyFileReplacing copies content and uses replacing semantics
// (verified by symlink case — covered transitively, one happy-path test).
func TestCopyFileReplacing(t *testing.T) {
	d := t.TempDir()
	src := filepath.Join(d, "src.md")
	dst := filepath.Join(d, "dst.md")
	if err := os.WriteFile(src, []byte("source content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("old dst"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CopyFileReplacing(src, dst); err != nil {
		t.Fatalf("copy: %v", err)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != "source content" {
		t.Errorf("dst not overwritten with src: %q", got)
	}
}

package load

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// L1: WriteFileReplacing creates a new file when path doesn't exist.
func TestWriteFileReplacingNew(t *testing.T) {
	d := t.TempDir()
	path := filepath.Join(d, "new.md")
	if err := WriteFileReplacing(d, path, []byte("hello"), 0o644); err != nil {
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
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileReplacing(d, path, []byte("new"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "new" {
		t.Errorf("content not overwritten: %q", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 600", info.Mode().Perm())
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
	if err := WriteFileReplacing(d, path, []byte("real content"), 0o644); err != nil {
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
	if err := WriteFileReplacing(d, link, []byte("link replacement"), 0o644); err != nil {
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
	if err := WriteFileReplacing(d, deep, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := os.Stat(deep); err != nil {
		t.Errorf("file not created: %v", err)
	}
}

func TestWriteFileReplacingCleansTemporaryFileWhenRenameFails(t *testing.T) {
	d := t.TempDir()
	path := filepath.Join(d, "occupied")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileReplacing(d, path, []byte("replacement"), 0o644); err == nil {
		t.Fatal("write unexpectedly replaced a directory")
	}
	matches, err := filepath.Glob(filepath.Join(d, ".syncai-write-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary files remain after failed replacement: %v", matches)
	}
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		t.Fatalf("original directory changed after failed replacement: info=%v err=%v", info, err)
	}
}

func TestWriteFileReplacingNearMaximumFilenameLength(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("near-NAME_MAX regression is supported on macOS and Linux")
	}
	d := t.TempDir()
	path := filepath.Join(d, strings.Repeat("n", 250))
	if err := WriteFileReplacing(d, path, []byte("content"), 0o644); err != nil {
		t.Fatalf("writing near-NAME_MAX destination: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "content" {
		t.Fatalf("content = %q, want content", got)
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
	if err := CopyFileReplacing(d, src, dst); err != nil {
		t.Fatalf("copy: %v", err)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != "source content" {
		t.Errorf("dst not overwritten with src: %q", got)
	}
}

func TestCopyDirRejectsSymlinkedDestinationAncestor(t *testing.T) {
	root := t.TempDir()
	source := t.TempDir()
	external := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("source"), 0o644); err != nil {
		t.Fatal(err)
	}
	externalFile := filepath.Join(external, "example", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(externalFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(externalFile, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(root, "skills")); err != nil {
		t.Fatal(err)
	}

	err := CopyDir(root, source, filepath.Join(root, "skills", "example"))
	if err == nil {
		t.Fatal("CopyDir succeeded through a symlinked destination ancestor")
	}
	got, readErr := os.ReadFile(externalFile)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "outside" {
		t.Fatalf("external file changed to %q", got)
	}
}

func TestCopyDirRejectsDestinationThatNormalizesToRoot(t *testing.T) {
	root := t.TempDir()
	source := t.TempDir()
	sentinel := filepath.Join(root, "sentinel")
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("source"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sentinel, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := CopyDir(root, source, filepath.Join("child", "..")); err == nil {
		t.Fatal("CopyDir accepted a destination that normalizes to the root")
	}
	got, err := os.ReadFile(sentinel)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "keep" {
		t.Fatalf("root sentinel changed to %q", got)
	}
}

func TestCopyDirRejectsSourceSymlink(t *testing.T) {
	root := t.TempDir()
	source := t.TempDir()
	external := t.TempDir()
	privateFile := filepath.Join(external, "private.txt")
	if err := os.WriteFile(privateFile, []byte("private bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(source, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(privateFile, filepath.Join(source, "nested", "private.txt")); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(root, "skills", "example")

	if err := CopyDir(root, source, destination); err == nil {
		t.Fatal("CopyDir accepted a source symlink")
	}
	if _, err := os.ReadFile(filepath.Join(destination, "nested", "private.txt")); !os.IsNotExist(err) {
		t.Fatalf("private bytes reached destination: %v", err)
	}
}

func TestCopyDirPreservesFileMode(t *testing.T) {
	root := t.TempDir()
	source := t.TempDir()
	sourceFile := filepath.Join(source, "run.sh")
	if err := os.WriteFile(sourceFile, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(root, "skills", "example")

	if err := CopyDir(root, source, destination); err != nil {
		t.Fatalf("CopyDir: %v", err)
	}
	info, err := os.Stat(filepath.Join(destination, "run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o755 {
		t.Fatalf("copied mode = %o, want 755", got)
	}
}

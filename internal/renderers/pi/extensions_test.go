package pi

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jsmestad/syncai/internal/renderers"
	"github.com/jsmestad/syncai/internal/schema"
)

// makeExtFile writes content to <root>/<rel> creating parent dirs.
func makeExtFile(t *testing.T, root, rel, content string) string {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return full
}

func TestRender_NoExtensions_DoesNotCreateDir(t *testing.T) {
	out := t.TempDir()
	in := renderers.Inputs{}
	written, err := New().Render(in, out)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range written {
		if filepath.Base(filepath.Dir(p)) == "extensions" {
			t.Errorf("expected no extension paths written, got %s", p)
		}
	}
	if _, err := os.Stat(filepath.Join(out, ".pi", "agent", "extensions")); !os.IsNotExist(err) {
		t.Errorf("extensions/ dir should not exist when no extensions present, got err=%v", err)
	}
}

func TestRender_SingleFileExtension_CopiesVerbatim(t *testing.T) {
	src := t.TempDir()
	srcFile := makeExtFile(t, src, "btw.ts", "// btw extension body\nexport default { id: 1 };\n")

	out := t.TempDir()
	in := renderers.Inputs{
		Extensions: []*schema.Extension{
			{Name: "btw", SourcePath: srcFile, IsDirectory: false},
		},
	}
	written, err := New().Render(in, out)
	if err != nil {
		t.Fatal(err)
	}

	target := filepath.Join(out, ".pi", "agent", "extensions", "btw.ts")
	if !contains(written, target) {
		t.Errorf("written paths %v missing %s", written, target)
	}
	gotBytes, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotBytes) != "// btw extension body\nexport default { id: 1 };\n" {
		t.Errorf("rendered file differs from source:\n%s", gotBytes)
	}
}

func TestRender_DirectoryExtension_RecursivelyCopies(t *testing.T) {
	src := t.TempDir()
	srcDir := filepath.Join(src, "review-loop")
	makeExtFile(t, src, "review-loop/index.ts", "export default {};\n")
	makeExtFile(t, src, "review-loop/flows.ts", "export const x = 1;\n")
	makeExtFile(t, src, "review-loop/tests/foo.test.ts", "// test\n")
	makeExtFile(t, src, "review-loop/package.json", `{"name":"review-loop"}`)

	out := t.TempDir()
	in := renderers.Inputs{
		Extensions: []*schema.Extension{
			{Name: "review-loop", SourcePath: srcDir, IsDirectory: true},
		},
	}
	if _, err := New().Render(in, out); err != nil {
		t.Fatal(err)
	}

	want := []string{
		"index.ts",
		"flows.ts",
		"tests/foo.test.ts",
		"package.json",
	}
	for _, rel := range want {
		path := filepath.Join(out, ".pi", "agent", "extensions", "review-loop", rel)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected %s to exist, got %v", path, err)
		}
	}

	// Sanity: the install path should NOT have a .ts suffix on the dir name.
	bare := filepath.Join(out, ".pi", "agent", "extensions", "review-loop")
	st, err := os.Stat(bare)
	if err != nil || !st.IsDir() {
		t.Errorf("review-loop should be a dir at %s, got %v", bare, err)
	}
	tsName := filepath.Join(out, ".pi", "agent", "extensions", "review-loop.ts")
	if _, err := os.Stat(tsName); err == nil {
		t.Errorf("must not write .ts suffixed dir name at %s", tsName)
	}
}

func TestRender_TomlSidecarsNotCopied(t *testing.T) {
	// Sidecars are build-time metadata. They must not show up in the
	// installed extensions/ tree, in either single-file or directory form.
	src := t.TempDir()
	srcFile := makeExtFile(t, src, "commit-gate.ts", "// gate\n")
	makeExtFile(t, src, "commit-gate.toml", "scope = \"work\"\n")

	srcDir := filepath.Join(src, "review-loop")
	makeExtFile(t, src, "review-loop/index.ts", "// rl\n")
	makeExtFile(t, src, "review-loop/extension.toml", "scope = \"work\"\n")

	out := t.TempDir()
	in := renderers.Inputs{
		Extensions: []*schema.Extension{
			{Name: "commit-gate", SourcePath: srcFile, IsDirectory: false, Scope: []string{"work"}},
			{Name: "review-loop", SourcePath: srcDir, IsDirectory: true, Scope: []string{"work"}},
		},
	}
	if _, err := New().Render(in, out); err != nil {
		t.Fatal(err)
	}

	extRoot := filepath.Join(out, ".pi", "agent", "extensions")
	if _, err := os.Stat(filepath.Join(extRoot, "commit-gate.ts")); err != nil {
		t.Errorf("expected commit-gate.ts to be copied, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(extRoot, "commit-gate.toml")); err == nil {
		t.Errorf("single-file sidecar TOML must not be copied to install tree")
	}
	if _, err := os.Stat(filepath.Join(extRoot, "review-loop", "extension.toml")); err == nil {
		t.Errorf("directory sidecar extension.toml must not be copied to install tree")
	}
	if _, err := os.Stat(filepath.Join(extRoot, "review-loop", "index.ts")); err != nil {
		t.Errorf("expected review-loop/index.ts to remain after sidecar strip, got %v", err)
	}
}

func TestRender_ProjectMode_UsesProjectExtensionsPath(t *testing.T) {
	src := t.TempDir()
	srcFile := makeExtFile(t, src, "btw.ts", "// btw\n")

	out := t.TempDir()
	in := renderers.Inputs{
		Extensions: []*schema.Extension{
			{Name: "btw", SourcePath: srcFile, IsDirectory: false},
		},
		ProjectMode: true,
	}
	if _, err := New().Render(in, out); err != nil {
		t.Fatal(err)
	}

	// Project mode: <out>/.pi/extensions/btw.ts (no /agent/)
	projectTarget := filepath.Join(out, ".pi", "extensions", "btw.ts")
	if _, err := os.Stat(projectTarget); err != nil {
		t.Errorf("project-mode target missing at %s: %v", projectTarget, err)
	}
	globalTarget := filepath.Join(out, ".pi", "agent", "extensions", "btw.ts")
	if _, err := os.Stat(globalTarget); err == nil {
		t.Errorf("project mode should not write global path %s", globalTarget)
	}
}

func TestRender_StaleExtensionsCleanedByDirCopy(t *testing.T) {
	// CopyDir clears the target before writing. Verify a previously-rendered
	// file inside a directory extension goes away when the source no longer
	// has it. This exercises the "rename a file in source → previous file
	// is pruned" path.
	src := t.TempDir()
	srcDir := filepath.Join(src, "ext-a")
	makeExtFile(t, src, "ext-a/old.ts", "// stale\n")

	out := t.TempDir()
	target := filepath.Join(out, ".pi", "agent", "extensions", "ext-a")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	// Pre-existing file from a previous render that's no longer in source.
	if err := os.WriteFile(filepath.Join(target, "old.ts"), []byte("// pre-existing\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "ghost.ts"), []byte("// removed in new render\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	in := renderers.Inputs{
		Extensions: []*schema.Extension{
			{Name: "ext-a", SourcePath: srcDir, IsDirectory: true},
		},
	}
	if _, err := New().Render(in, out); err != nil {
		t.Fatal(err)
	}

	// ghost.ts must be gone (source no longer has it).
	if _, err := os.Stat(filepath.Join(target, "ghost.ts")); err == nil {
		t.Errorf("stale ghost.ts must be cleaned by CopyDir")
	}
	// old.ts is still in source so it should be present and match source.
	got, err := os.ReadFile(filepath.Join(target, "old.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "// stale\n" {
		t.Errorf("old.ts not refreshed from source, got: %q", got)
	}
}

func contains(s []string, want string) bool {
	for _, x := range s {
		if x == want {
			return true
		}
	}
	return false
}

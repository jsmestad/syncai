package load

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jsmestad/syncai/internal/schema"
)

// makeExtTree builds a minimal ai-source/extensions/ tree under t.TempDir
// and returns the source root (the parent of "extensions/").
func makeExtTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "extensions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for rel, body := range files {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestExtensions_MissingDirReturnsNil(t *testing.T) {
	root := t.TempDir()
	out, err := Extensions(root, "")
	if err != nil {
		t.Fatalf("expected no error for missing extensions dir, got %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected 0 extensions, got %d", len(out))
	}
}

func TestExtensions_SingleFileNoSidecar(t *testing.T) {
	root := makeExtTree(t, map[string]string{
		"btw.ts": "// btw extension\nexport default {};\n",
	})
	out, err := Extensions(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 extension, got %d", len(out))
	}
	got := out[0]
	if got.Name != "btw" {
		t.Errorf("Name: want btw, got %q", got.Name)
	}
	if got.IsDirectory {
		t.Errorf("IsDirectory: want false, got true")
	}
	if len(got.Scope) != 0 {
		t.Errorf("Scope: want empty, got %v", got.Scope)
	}
	if got.InstallName() != "btw.ts" {
		t.Errorf("InstallName: want btw.ts, got %q", got.InstallName())
	}
}

func TestExtensions_SingleFileWithSidecar(t *testing.T) {
	root := makeExtTree(t, map[string]string{
		"commit-gate.ts":   "// guts\n",
		"commit-gate.toml": "scope = \"work\"\n",
	})
	out, err := Extensions(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 extension (sidecar should not count as separate), got %d", len(out))
	}
	got := out[0]
	if got.Name != "commit-gate" || !equalStrings(got.Scope, []string{"work"}) {
		t.Errorf("got %+v, want name=commit-gate scope=[work]", got)
	}
}

func TestExtensions_DirectoryWithSidecar(t *testing.T) {
	root := makeExtTree(t, map[string]string{
		"review-loop/index.ts":       "export default {};\n",
		"review-loop/flows.ts":       "export const x = 1;\n",
		"review-loop/extension.toml": "scope = \"work\"\n",
	})
	out, err := Extensions(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 extension, got %d", len(out))
	}
	got := out[0]
	if got.Name != "review-loop" {
		t.Errorf("Name: want review-loop, got %q", got.Name)
	}
	if !got.IsDirectory {
		t.Errorf("IsDirectory: want true, got false")
	}
	if !equalStrings(got.Scope, []string{"work"}) {
		t.Errorf("Scope: want [work], got %v", got.Scope)
	}
	if got.InstallName() != "review-loop" {
		t.Errorf("InstallName: want review-loop (no .ts), got %q", got.InstallName())
	}
}

func TestExtensions_ScopeFilterExcludesOthers(t *testing.T) {
	root := makeExtTree(t, map[string]string{
		"btw.ts":         "// universal\n",
		"home-only.ts":   "// home\n",
		"home-only.toml": "scope = \"home\"\n",
		"work-only.ts":   "// work\n",
		"work-only.toml": "scope = \"work\"\n",
	})

	all, err := Extensions(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("unfiltered: expected 3, got %d (%v)", len(all), names(all))
	}

	work, err := Extensions(root, "work")
	if err != nil {
		t.Fatal(err)
	}
	if got := names(work); !equalStrings(got, []string{"btw", "work-only"}) {
		t.Errorf("scope=work: got %v, want [btw work-only]", got)
	}

	home, err := Extensions(root, "home")
	if err != nil {
		t.Fatal(err)
	}
	if got := names(home); !equalStrings(got, []string{"btw", "home-only"}) {
		t.Errorf("scope=home: got %v, want [btw home-only]", got)
	}
}

func TestExtensions_TopLevelTomlIgnored(t *testing.T) {
	// A naked TOML at top level shouldn't appear as an extension. It has
	// no .ts pair so we silently skip it. Catches the case where someone
	// drops a stray config file.
	root := makeExtTree(t, map[string]string{
		"foo.ts":      "// real\n",
		"orphan.toml": "scope = \"home\"\n",
	})
	out, err := Extensions(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].Name != "foo" {
		t.Errorf("got %v, want [foo]", names(out))
	}
}

func TestExtensions_InvalidScopeIsRejected(t *testing.T) {
	root := makeExtTree(t, map[string]string{
		"bad.ts":   "// nope\n",
		"bad.toml": "scope = \"production\"\n",
	})
	_, err := Extensions(root, "")
	if err == nil {
		t.Fatal("expected error for invalid scope, got nil")
	}
}

func TestExtensions_NonTsTopLevelFileIgnored(t *testing.T) {
	// A README.md at top level isn't an extension. Silently skip it.
	root := makeExtTree(t, map[string]string{
		"README.md": "# extensions\n",
		"real.ts":   "// real\n",
	})
	out, err := Extensions(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].Name != "real" {
		t.Errorf("got %v, want [real]", names(out))
	}
}

func TestExtensions_SortedByName(t *testing.T) {
	root := makeExtTree(t, map[string]string{
		"zeta.ts":  "//\n",
		"alpha.ts": "//\n",
		"mu.ts":    "//\n",
	})
	out, err := Extensions(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := names(out); !equalStrings(got, []string{"alpha", "mu", "zeta"}) {
		t.Errorf("got %v, want sorted", got)
	}
}

func names(exts []*schema.Extension) []string {
	out := make([]string, len(exts))
	for i, e := range exts {
		out[i] = e.Name
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

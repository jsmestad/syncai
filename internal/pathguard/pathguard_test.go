package pathguard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveRejectsEscapes(t *testing.T) {
	root := t.TempDir()
	external := t.TempDir()
	link := filepath.Join(root, "linked")
	if err := os.Symlink(external, link); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		candidate string
	}{
		{name: "absolute", candidate: filepath.Join(external, "file")},
		{name: "traversal", candidate: filepath.Join("..", filepath.Base(external), "file")},
		{name: "symlinked ancestor", candidate: filepath.Join(link, "file")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Resolve(root, tt.candidate)
			if err == nil {
				t.Fatalf("Resolve(%q, %q) succeeded", root, tt.candidate)
			}
			if !strings.Contains(err.Error(), tt.candidate) || !strings.Contains(err.Error(), root) {
				t.Fatalf("error %q must name candidate %q and root %q", err, tt.candidate, root)
			}
		})
	}
}

func TestResolveAcceptsContainedPaths(t *testing.T) {
	root := t.TempDir()
	existing := filepath.Join(root, "existing")
	if err := os.WriteFile(existing, []byte("present"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, candidate := range []string{existing, filepath.Join("missing", "child")} {
		got, err := Resolve(root, candidate)
		if err != nil {
			t.Fatalf("Resolve(%q, %q): %v", root, candidate, err)
		}
		if !filepath.IsAbs(got) {
			t.Fatalf("Resolve(%q, %q) = %q, want absolute path", root, candidate, got)
		}
	}
}

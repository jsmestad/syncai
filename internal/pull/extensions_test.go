package pull

import (
	"os"
	"path/filepath"
	"testing"
)

func writeRel(t *testing.T, root, rel, content string) string {
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

func TestPlanExtension_SingleFile_NoChanges(t *testing.T) {
	src := t.TempDir()
	installed := writeRel(t, src, "btw.ts", "// same\n")
	source := writeRel(t, src, "src/btw.ts", "// same\n")

	plan, err := PlanExtension("btw", installed, source, false)
	if err != nil {
		t.Fatal(err)
	}
	if plan.HasChanges() {
		t.Errorf("expected no changes, got %+v", plan.Changes)
	}
}

func TestPlanExtension_SingleFile_BodyDiff(t *testing.T) {
	src := t.TempDir()
	installed := writeRel(t, src, "btw.ts", "// installed body\n")
	source := writeRel(t, src, "src/btw.ts", "// source body\n")

	plan, err := PlanExtension("btw", installed, source, false)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.HasChanges() {
		t.Fatal("expected changes")
	}
	if len(plan.Changes) != 1 || plan.Changes[0].Kind != "edited" {
		t.Errorf("expected 1 edited change, got %+v", plan.Changes)
	}

	if err := plan.Apply(filepath.Join(src, "src")); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(source)
	if string(got) != "// installed body\n" {
		t.Errorf("source not updated, got %q", got)
	}
}

func TestPlanExtension_Directory_PerFileDiff(t *testing.T) {
	src := t.TempDir()

	installRoot := filepath.Join(src, "install", "review-loop")
	writeRel(t, src, "install/review-loop/index.ts", "// new index\n")
	writeRel(t, src, "install/review-loop/flows.ts", "// unchanged\n")
	writeRel(t, src, "install/review-loop/added.ts", "// brand new file\n")
	writeRel(t, src, "install/review-loop/tests/foo.test.ts", "// edited test\n")

	sourceRoot := filepath.Join(src, "source", "review-loop")
	writeRel(t, src, "source/review-loop/index.ts", "// old index\n")
	writeRel(t, src, "source/review-loop/flows.ts", "// unchanged\n")
	writeRel(t, src, "source/review-loop/tests/foo.test.ts", "// old test\n")
	writeRel(t, src, "source/review-loop/extension.toml", "scope = \"work\"\n")
	writeRel(t, src, "source/review-loop/orphan.ts", "// in source only\n")

	plan, err := PlanExtension("review-loop", installRoot, sourceRoot, true)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.HasChanges() {
		t.Fatal("expected changes")
	}

	wantKinds := map[string]string{
		"index.ts":          "edited",
		"added.ts":          "added",
		"tests/foo.test.ts": "edited",
		"orphan.ts":         "removed",
	}
	gotKinds := map[string]string{}
	for _, c := range plan.Changes {
		gotKinds[c.RelPath] = c.Kind
	}
	for path, kind := range wantKinds {
		if gotKinds[path] != kind {
			t.Errorf("%s: want %q, got %q", path, kind, gotKinds[path])
		}
	}
	// flows.ts unchanged should not appear at all.
	if _, ok := gotKinds["flows.ts"]; ok {
		t.Errorf("unchanged flows.ts must not appear in changes")
	}
	// extension.toml in source must not be reported as removed.
	if gotKinds["extension.toml"] != "" {
		t.Errorf("extension.toml sidecar must be excluded from removal report (got %q)", gotKinds["extension.toml"])
	}
}

func TestPlanExtension_Apply_DirectoryChanges(t *testing.T) {
	src := t.TempDir()
	installRoot := filepath.Join(src, "install", "ext")
	writeRel(t, src, "install/ext/index.ts", "// new index\n")
	writeRel(t, src, "install/ext/added.ts", "// brand new\n")
	sourceRoot := filepath.Join(src, "source", "ext")
	writeRel(t, src, "source/ext/index.ts", "// old index\n")
	writeRel(t, src, "source/ext/extension.toml", "scope = \"work\"\n")
	writeRel(t, src, "source/ext/will-be-removed.ts", "// will be skipped on apply\n")

	plan, err := PlanExtension("ext", installRoot, sourceRoot, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(filepath.Join(src, "source")); err != nil {
		t.Fatal(err)
	}
	// Edited file: source updated.
	got, _ := os.ReadFile(filepath.Join(sourceRoot, "index.ts"))
	if string(got) != "// new index\n" {
		t.Errorf("index.ts not updated, got %q", got)
	}
	// Added file: now exists in source.
	got, err = os.ReadFile(filepath.Join(sourceRoot, "added.ts"))
	if err != nil {
		t.Errorf("added.ts not written: %v", err)
	}
	if string(got) != "// brand new\n" {
		t.Errorf("added.ts wrong content: %q", got)
	}
	// Sidecar preserved.
	got, _ = os.ReadFile(filepath.Join(sourceRoot, "extension.toml"))
	if string(got) != "scope = \"work\"\n" {
		t.Errorf("extension.toml clobbered, got %q", got)
	}
	// Removed file: deliberately NOT deleted (Apply leaves "removed" alone).
	if _, err := os.Stat(filepath.Join(sourceRoot, "will-be-removed.ts")); err != nil {
		t.Errorf("will-be-removed.ts must NOT be deleted by Apply, got err=%v", err)
	}
}

func TestExtensionApplyRejectsSymlinkedSourceAncestor(t *testing.T) {
	root := t.TempDir()
	installed := writeRel(t, root, "installed/ext/index.ts", "installed")
	external := t.TempDir()
	externalFile := writeRel(t, external, "ext/index.ts", "outside")
	sourceRoot := filepath.Join(root, "source")
	if err := os.MkdirAll(sourceRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(sourceRoot, "extensions")); err != nil {
		t.Fatal(err)
	}
	plan := ExtPlan{
		Name:          "ext",
		IsDirectory:   true,
		InstalledPath: filepath.Dir(installed),
		SourcePath:    filepath.Join(sourceRoot, "extensions", "ext"),
		Changes:       []ExtFileChange{{RelPath: "index.ts", Kind: "edited"}},
	}

	if err := plan.Apply(sourceRoot); err == nil {
		t.Fatal("Apply succeeded through a symlinked source ancestor")
	}
	got, err := os.ReadFile(externalFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "outside" {
		t.Fatalf("external file changed to %q", got)
	}
}

func TestExtensionApplyReplacesFinalSourceSymlink(t *testing.T) {
	root := t.TempDir()
	installed := writeRel(t, root, "installed/ext.ts", "installed bytes")
	sourceRoot := filepath.Join(root, "source")
	target := writeRel(t, sourceRoot, "extensions/other.ts", "other canonical bytes")
	intended := filepath.Join(sourceRoot, "extensions", "ext.ts")
	if err := os.Chmod(target, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("other.ts", intended); err != nil {
		t.Fatal(err)
	}
	plan := ExtPlan{
		Name:          "ext",
		InstalledPath: installed,
		SourcePath:    intended,
		Changes:       []ExtFileChange{{Kind: "edited"}},
	}

	if err := plan.Apply(sourceRoot); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	assertFileUnchanged(t, target, "other canonical bytes", 0o600)
	assertRegularFile(t, intended, "installed bytes", 0o644)
}

func TestSummariseChanges(t *testing.T) {
	cases := []struct {
		name string
		in   []ExtFileChange
		want string
	}{
		{"empty", nil, "no changes"},
		{"single edit", []ExtFileChange{{Kind: "edited"}}, "edited (1)"},
		{
			"mixed",
			[]ExtFileChange{
				{Kind: "edited"},
				{Kind: "edited"},
				{Kind: "added"},
				{Kind: "removed"},
			},
			"edited (2), added (1), removed (1)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SummariseChanges(tc.in); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

package importer

import (
	"os"
	"path/filepath"
	"testing"
)

// makeFile writes bytes under root/relPath, creating parent dirs.
func makeFile(t *testing.T, root, relPath, body string) string {
	t.Helper()
	full := filepath.Join(root, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return full
}

// fakeHome builds a $HOME-shaped temp tree with .pi/agent/extensions/.
func fakeHome(t *testing.T, files map[string]string, dirs []string, symlinks map[string]string) string {
	t.Helper()
	home := t.TempDir()
	extDir := filepath.Join(home, ".pi", "agent", "extensions")
	if err := os.MkdirAll(extDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for rel, body := range files {
		makeFile(t, extDir, rel, body)
	}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(extDir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for link, target := range symlinks {
		if err := os.Symlink(target, filepath.Join(extDir, link)); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

func fakeSource(t *testing.T, names map[string]bool) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "extensions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, isDir := range names {
		if isDir {
			if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
				t.Fatal(err)
			}
		} else {
			makeFile(t, dir, name+".ts", "// in source\n")
		}
	}
	return root
}

func TestScanExtensions_FindsUntrackedFiles(t *testing.T) {
	home := fakeHome(t,
		map[string]string{
			"prototype.ts": "// new\n",
			"vendored.ts":  "// also new\n",
		}, nil, nil)
	source := fakeSource(t, nil)

	got, err := ScanExtensions(home, source)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 candidates, got %d (%+v)", len(got), got)
	}
	if got[0].Name != "prototype" || got[1].Name != "vendored" {
		t.Errorf("unexpected names: %+v", got)
	}
	for _, c := range got {
		if c.IsDirectory {
			t.Errorf("%s should not be marked directory", c.Name)
		}
	}
}

func TestScanExtensions_SkipsAllDirectories(t *testing.T) {
	// Directory entries at top of ~/.pi/agent/extensions/ are intentionally
	// not import candidates: external pi extensions (third-party-ext, observe,
	// slack, subagent) are installed as real directories with their own
	// package.json. Auto-importing those would vendor third-party code.
	// Vendoring a real directory extension is a deliberate cp -R decision.
	home := fakeHome(t, nil, []string{"new-dir-ext", "third-party-ext"}, nil)
	makeFile(t, filepath.Join(home, ".pi", "agent", "extensions"), "new-dir-ext/index.ts", "// dir ext\n")
	makeFile(t, filepath.Join(home, ".pi", "agent", "extensions"), "third-party-ext/index.ts", "// external\n")
	source := fakeSource(t, nil)

	got, err := ScanExtensions(home, source)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("directories must not be import candidates, got %+v", got)
	}
}

func TestScanExtensions_SkipsKnownExtensions(t *testing.T) {
	// btw is in source — should be skipped.
	home := fakeHome(t, map[string]string{
		"btw.ts":       "// installed\n",
		"new-thing.ts": "// untracked\n",
	}, nil, nil)
	source := fakeSource(t, map[string]bool{"btw": false})

	got, err := ScanExtensions(home, source)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "new-thing" {
		t.Errorf("expected [new-thing], got %+v", got)
	}
}

func TestScanExtensions_SkipsSymlinks(t *testing.T) {
	// Symlinks at the top level represent external pi extensions
	// (nix store, package-managed clones). They should not be import candidates.
	target := t.TempDir()
	makeFile(t, target, "external.ts", "// external\n")

	home := fakeHome(t,
		map[string]string{"local.ts": "// local\n"},
		nil,
		map[string]string{"external-link.ts": filepath.Join(target, "external.ts")},
	)
	source := fakeSource(t, nil)

	got, err := ScanExtensions(home, source)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "local" {
		t.Errorf("expected only local file, got %+v", got)
	}
}

func TestScanExtensions_SkipsNonTsFilesAtTopLevel(t *testing.T) {
	// A README.md at the top of ~/.pi/agent/extensions/ is not an extension.
	home := fakeHome(t, map[string]string{
		"README.md": "# extensions\n",
		"real.ts":   "// real\n",
	}, nil, nil)
	source := fakeSource(t, nil)

	got, err := ScanExtensions(home, source)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "real" {
		t.Errorf("got %+v, want only real", got)
	}
}

func TestScanExtensions_HandlesMissingExtensionsDir(t *testing.T) {
	// Fresh home with no ~/.pi/agent/extensions/ directory at all.
	home := t.TempDir()
	source := fakeSource(t, nil)

	got, err := ScanExtensions(home, source)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("expected nil candidates, got %+v", got)
	}
}

func TestPortExtension_SingleFile_CopiesVerbatim(t *testing.T) {
	src := t.TempDir()
	body := "// the body\nexport default {};\n"
	srcFile := makeFile(t, src, "mine.ts", body)

	out := t.TempDir()
	target := filepath.Join(out, "extensions", "mine.ts")

	c := ExtensionCandidate{Name: "mine", InputPath: srcFile, SourcePath: target}
	if err := PortExtension(out, c); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != body {
		t.Errorf("ported bytes differ:\n%s", got)
	}
}

func TestPortExtensionRejectsSymlinkedSourceAncestor(t *testing.T) {
	inputRoot := t.TempDir()
	input := makeFile(t, inputRoot, "mine.ts", "installed")
	sourceRoot := t.TempDir()
	external := t.TempDir()
	externalFile := makeFile(t, external, "mine.ts", "outside")
	if err := os.Symlink(external, filepath.Join(sourceRoot, "extensions")); err != nil {
		t.Fatal(err)
	}
	candidate := ExtensionCandidate{Name: "mine", InputPath: input, SourcePath: filepath.Join(sourceRoot, "extensions", "mine.ts")}

	if err := PortExtension(sourceRoot, candidate); err == nil {
		t.Fatal("PortExtension succeeded through a symlinked source ancestor")
	}
	got, err := os.ReadFile(externalFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "outside" {
		t.Fatalf("external file changed to %q", got)
	}
}

func TestPortExtensionReplacesFinalSourceSymlink(t *testing.T) {
	inputRoot := t.TempDir()
	input := makeFile(t, inputRoot, "mine.ts", "installed bytes")
	sourceRoot := t.TempDir()
	target := makeFile(t, sourceRoot, "extensions/other.ts", "other canonical bytes")
	intended := filepath.Join(sourceRoot, "extensions", "mine.ts")
	if err := os.Chmod(target, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("other.ts", intended); err != nil {
		t.Fatal(err)
	}
	candidate := ExtensionCandidate{Name: "mine", InputPath: input, SourcePath: intended}

	if err := PortExtension(sourceRoot, candidate); err != nil {
		t.Fatalf("PortExtension: %v", err)
	}
	assertFileUnchanged(t, target, "other canonical bytes", 0o600)
	assertRegularFile(t, intended, "installed bytes", 0o644)
}

func TestPortExtension_Directory_RejectedWithError(t *testing.T) {
	// Belt-and-suspenders: even if a caller constructs an
	// ExtensionCandidate{IsDirectory:true} by hand (ScanExtensions never
	// produces one), PortExtension should refuse rather than vendor a tree.
	c := ExtensionCandidate{Name: "x", InputPath: "/tmp/x", SourcePath: "/tmp/y", IsDirectory: true}
	if err := PortExtension(t.TempDir(), c); err == nil {
		t.Errorf("expected error for directory candidate, got nil")
	}
}

func TestScanExtensionDirectories_ClassifiesByHint(t *testing.T) {
	home := fakeHome(t, nil, []string{"external-pkg", "my-prototype", "ambiguous"}, nil)
	extRoot := filepath.Join(home, ".pi", "agent", "extensions")
	// external-pkg: has package.json with a name field
	makeFile(t, extRoot, "external-pkg/package.json", `{"name":"@example/pi-tool","version":"0.3.2"}`)
	makeFile(t, extRoot, "external-pkg/index.ts", "// external\n")
	// my-prototype: no package.json
	makeFile(t, extRoot, "my-prototype/index.ts", "// proto\n")
	// ambiguous: package.json without a name field
	makeFile(t, extRoot, "ambiguous/package.json", `{"version":"0.0.0","private":true}`)
	makeFile(t, extRoot, "ambiguous/index.ts", "// hmm\n")

	source := fakeSource(t, nil)

	got, err := ScanExtensionDirectories(home, source)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 dirs, got %d (%+v)", len(got), got)
	}
	hints := map[string]string{}
	for _, d := range got {
		hints[d.Name] = d.Hint()
	}
	wantHints := map[string]string{
		"external-pkg": "external",
		"my-prototype": "prototype",
		"ambiguous":    "check",
	}
	for name, want := range wantHints {
		if hints[name] != want {
			t.Errorf("%s: want hint %q, got %q", name, want, hints[name])
		}
	}
}

func TestScanExtensionDirectories_SkipsKnownAndSymlinks(t *testing.T) {
	extern := t.TempDir()
	if err := os.MkdirAll(filepath.Join(extern, "some-target"), 0o755); err != nil {
		t.Fatal(err)
	}
	home := fakeHome(t, nil, []string{"in-source", "untracked"}, map[string]string{
		"linked": filepath.Join(extern, "some-target"),
	})
	extRoot := filepath.Join(home, ".pi", "agent", "extensions")
	makeFile(t, extRoot, "in-source/index.ts", "// src has this\n")
	makeFile(t, extRoot, "untracked/index.ts", "// not in src\n")

	source := fakeSource(t, map[string]bool{"in-source": true})

	got, err := ScanExtensionDirectories(home, source)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "untracked" {
		t.Errorf("want only [untracked], got %+v", got)
	}
}

func TestScanExtensionDirectories_HandlesMissingExtensionsDir(t *testing.T) {
	home := t.TempDir()
	source := fakeSource(t, nil)

	got, err := ScanExtensionDirectories(home, source)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("expected nil for missing extensions dir, got %+v", got)
	}
}

func TestExtensionDirectory_Hint(t *testing.T) {
	cases := []struct {
		name string
		in   ExtensionDirectory
		want string
	}{
		{"no package", ExtensionDirectory{HasPackage: false}, "prototype"},
		{"named package", ExtensionDirectory{HasPackage: true, PackageName: "@x/y"}, "external"},
		{"unnamed package", ExtensionDirectory{HasPackage: true, PackageName: ""}, "check"},
	}
	for _, tc := range cases {
		if got := tc.in.Hint(); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

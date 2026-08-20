package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	aipackages "github.com/jsmestad/syncai/internal/packages"
)

func TestCompleteExampleRendersAllTargets(t *testing.T) {
	source := completeExampleSource(t)
	out := renderCompleteExample(t, source, "balanced", "")

	assertFileContains(t, out, filepath.Join(".pi", "agent", "agents", "explorer.md"),
		"model: example-lab/orbit-small\n",
		"thinking: medium\n",
		"tools: read, bash, grep, find\n",
	)
	assertFileContains(t, out, filepath.Join(".omp", "agent", "agents", "explorer.md"),
		"model: example-lab/orbit-small\n",
		"thinkingLevel: medium\n",
		"tools: read, bash, grep, glob\n",
	)
	assertFileContains(t, out, filepath.Join(".claude", "agents", "explorer.md"),
		"model: example-claude-explorer\n",
		"tools: Read, Bash, Grep, Glob\n",
	)
	assertFileContains(t, out, filepath.Join(".codex", "agents", "explorer.toml"),
		"model = \"example-codex-explorer\"\n",
		"model_reasoning_effort = \"medium\"\n",
		"sandbox_mode = \"read-only\"\n",
	)
	assertFileContains(t, out, filepath.Join(".config", "opencode", "agents", "explorer.md"),
		"model: example-lab/orbit-small\n",
		"  edit: deny\n",
		"  bash: allow\n",
		"  read: allow\n",
	)
	assertFileContains(t, out, filepath.Join(".gemini", "antigravity-cli", "plugins", "dfiles", "agents", "explorer.md"),
		"model: example-antigravity-explorer\n",
		"  - \"glob\"\n",
		"  - \"grep_search\"\n",
		"  - \"read_file\"\n",
		"  - \"run_shell_command\"\n",
	)

	for _, path := range []string{
		filepath.Join(".pi", "agent", "skills", "example-skill", "SKILL.md"),
		filepath.Join(".claude", "skills", "example-skill", "SKILL.md"),
		filepath.Join(".codex", "skills", "example-skill", "SKILL.md"),
		filepath.Join(".gemini", "antigravity-cli", "plugins", "dfiles", "skills", "example-skill", "SKILL.md"),
	} {
		assertFileContains(t, out, path, "name: example-skill\n")
	}
	for _, path := range []string{
		filepath.Join(".pi", "agent", "AGENTS.md"),
		filepath.Join(".claude", "CLAUDE.md"),
		filepath.Join(".codex", "AGENTS.md"),
		filepath.Join(".config", "opencode", "AGENTS.md"),
	} {
		assertFileContains(t, out, path, "# Shared instructions\n")
	}
	assertFileContains(t, out, filepath.Join(".pi", "agent", "extensions", "example", "index.ts"), "export default function exampleExtension()")
	assertPathAbsent(t, out, filepath.Join(".pi", "agent", "extensions", "example", "extension.toml"))
	assertFileContains(t, out, filepath.Join(".pi", "agent", "agents", "reviewer.md"), "model: example-lab/orbit-large\n")
}

func TestCompleteExampleAppliesEnvironmentOverrides(t *testing.T) {
	for _, test := range []struct {
		name       string
		profile    string
		scope      string
		path       string
		contains   []string
		absentPath string
	}{
		{
			name:       "balanced home",
			profile:    "balanced",
			scope:      "home",
			path:       filepath.Join(".pi", "agent", "agents", "explorer.md"),
			contains:   []string{"model: home-sample/orbit-compact\n", "thinking: low\n"},
			absentPath: filepath.Join(".pi", "agent", "agents", "reviewer.md"),
		},
		{
			name:     "focused work",
			profile:  "focused",
			scope:    "work",
			path:     filepath.Join(".config", "opencode", "agents", "reviewer.md"),
			contains: []string{"model: work-sample/compass-review\n"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := completeExampleSource(t)
			out := renderCompleteExample(t, source, test.profile, test.scope)
			assertFileContains(t, out, test.path, test.contains...)
			if test.absentPath != "" {
				assertPathAbsent(t, out, test.absentPath)
			}
			if test.profile == "focused" {
				assertFileContains(t, out, filepath.Join(".pi", "agent", "agents", "explorer.md"),
					"model: sample-foundry/compass-small\n",
					"thinking: low\n",
				)
			}
		})
	}
}

func TestCompleteExamplePackageManifestIsInert(t *testing.T) {
	source := completeExampleSource(t)
	manifest, err := aipackages.Load(aipackages.DefaultPath(source))
	if err != nil {
		t.Fatalf("loading complete example package manifest: %v", err)
	}

	resources := map[string][]string{
		"pi.packages":         manifest.Pi.Packages,
		"pi.npmCommand":       manifest.Pi.NPMCommand,
		"claude.marketplaces": manifest.Claude.Marketplaces,
		"claude.plugins":      manifest.Claude.Plugins,
		"codex.plugins":       manifest.Codex.Plugins,
		"antigravity.plugins": manifest.Antigravity.Plugins,
	}
	for name, values := range resources {
		if len(values) != 0 {
			t.Errorf("%s contains package actions: %v", name, values)
		}
	}
	if len(manifest.Pi.PackagesByScope) != 0 {
		t.Errorf("pi.packagesByScope contains package actions: %v", manifest.Pi.PackagesByScope)
	}
	if len(manifest.Pi.NPMCommandByScope) != 0 {
		t.Errorf("pi.npmCommandByScope contains package actions: %v", manifest.Pi.NPMCommandByScope)
	}
	for _, scope := range []string{"home", "work"} {
		resolved := manifest.ForScope(scope)
		if len(resolved.Pi.Packages) != 0 || len(resolved.Pi.NPMCommand) != 0 {
			t.Errorf("Pi package actions for scope %q: packages=%v npmCommand=%v", scope, resolved.Pi.Packages, resolved.Pi.NPMCommand)
		}
	}
}

type exampleEntrySnapshot struct {
	typeBits    os.FileMode
	permissions os.FileMode
	linkTarget  string
	body        []byte
}

func completeExampleSource(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolving complete example: runtime.Caller returned no path")
	}
	workingDirectory, workingDirectoryErr := os.Getwd()
	var source string
	if filepath.IsAbs(filename) {
		source = filepath.Join(filepath.Dir(filename), "..", "..", "examples", "complete")
	} else {
		if workingDirectoryErr != nil {
			t.Fatalf("resolving complete example from trimmed caller path %q: package test working directory: %v", filename, workingDirectoryErr)
		}
		source = filepath.Join(workingDirectory, "..", "..", "examples", "complete")
	}
	source = filepath.Clean(source)
	info, err := os.Stat(source)
	if err != nil {
		t.Fatalf("complete example fixture not found at %q (runtime caller path %q, package test working directory %q): %v", source, filename, workingDirectory, err)
	}
	if !info.IsDir() {
		t.Fatalf("complete example fixture at %q is not a directory (runtime caller path %q, package test working directory %q)", source, filename, workingDirectory)
	}
	return source
}

func renderCompleteExample(t *testing.T, source, profile, scope string) string {
	t.Helper()
	before := snapshotExample(t, source)
	out := t.TempDir()
	app := New(Streams{In: strings.NewReader(""), Out: &bytes.Buffer{}, Err: &bytes.Buffer{}})
	args := []string{"render", "--source", source, "--out", out, "--profile", profile}
	if scope != "" {
		args = append(args, "--scope", scope)
	}
	if err := app.Execute(context.Background(), args); err != nil {
		t.Fatalf("rendering complete example with profile %q and scope %q: %v", profile, scope, err)
	}
	after := snapshotExample(t, source)
	assertSnapshotsEqual(t, before, after)
	return out
}

func snapshotExample(t *testing.T, root string) map[string]exampleEntrySnapshot {
	t.Helper()
	entries := map[string]exampleEntrySnapshot{}
	err := filepath.WalkDir(root, func(path string, _ os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		snapshot := exampleEntrySnapshot{
			typeBits:    info.Mode() & os.ModeType,
			permissions: info.Mode().Perm(),
		}
		switch {
		case info.Mode().IsRegular():
			snapshot.body, err = os.ReadFile(path)
		case info.Mode()&os.ModeSymlink != 0:
			snapshot.linkTarget, err = os.Readlink(path)
		}
		if err != nil {
			return err
		}
		entries[rel] = snapshot
		return nil
	})
	if err != nil {
		t.Fatalf("snapshotting complete example: %v", err)
	}
	return entries
}

func assertSnapshotsEqual(t *testing.T, before, after map[string]exampleEntrySnapshot) {
	t.Helper()
	if len(after) != len(before) {
		t.Errorf("source entry count changed from %d to %d", len(before), len(after))
	}
	for _, path := range sortedSnapshotPaths(before) {
		want := before[path]
		got, ok := after[path]
		if !ok {
			t.Errorf("source entry removed during render: %s", path)
			continue
		}
		if got.typeBits != want.typeBits {
			t.Errorf("source entry type changed during render for %s: got %v, want %v", path, got.typeBits, want.typeBits)
		}
		if got.permissions != want.permissions {
			t.Errorf("source entry permissions changed during render for %s: got %v, want %v", path, got.permissions, want.permissions)
		}
		if got.linkTarget != want.linkTarget {
			t.Errorf("source symlink target changed during render for %s: got %q, want %q", path, got.linkTarget, want.linkTarget)
		}
		if !bytes.Equal(got.body, want.body) {
			t.Errorf("source file bytes changed during render: %s", path)
		}
	}
	for _, path := range sortedSnapshotPaths(after) {
		if _, ok := before[path]; !ok {
			t.Errorf("source entry created during render: %s", path)
		}
	}
}

func assertFileContains(t *testing.T, root, relativePath string, fragments ...string) {
	t.Helper()
	path := filepath.Join(root, relativePath)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading rendered file %s: %v", relativePath, err)
	}
	for _, fragment := range fragments {
		if !strings.Contains(string(body), fragment) {
			t.Errorf("rendered file %s does not contain %q:\n%s", relativePath, fragment, body)
		}
	}
}

func assertPathAbsent(t *testing.T, root, relativePath string) {
	t.Helper()
	if _, err := os.Lstat(filepath.Join(root, relativePath)); !os.IsNotExist(err) {
		t.Errorf("rendered path %s should be absent, got %v", relativePath, err)
	}
}

func sortedSnapshotPaths(snapshot map[string]exampleEntrySnapshot) []string {
	paths := make([]string, 0, len(snapshot))
	for path := range snapshot {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

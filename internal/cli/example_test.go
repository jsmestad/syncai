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
	out := renderCompleteExample(t, source, "openai", "")

	assertFileContains(t, out, filepath.Join(".pi", "agent", "agents", "worker.md"),
		"model: openai-codex/gpt-5.6-luna\n",
		"thinking: high\n",
		"tools: read, bash, edit, write, grep, find, ls\n",
	)
	assertFileContains(t, out, filepath.Join(".omp", "agent", "agents", "worker.md"),
		"model: openai-codex/gpt-5.6-luna\n",
		"thinkingLevel: high\n",
		"tools: read, bash, edit, write, grep, glob\n",
	)
	assertFileContains(t, out, filepath.Join(".claude", "agents", "worker.md"),
		"model: haiku\n",
		"tools: Read, Bash, Edit, Write, Grep, Glob\n",
	)
	assertFileContains(t, out, filepath.Join(".codex", "agents", "worker.toml"),
		"model = \"gpt-5.6-luna\"\n",
		"model_reasoning_effort = \"high\"\n",
	)
	assertFileContains(t, out, filepath.Join(".config", "opencode", "agents", "worker.md"),
		"model: openai/gpt-5.6-luna\n",
		"  edit: allow\n",
		"  bash: allow\n",
		"  read: allow\n",
	)
	assertFileContains(t, out, filepath.Join(".gemini", "antigravity-cli", "plugins", "dfiles", "agents", "worker.md"),
		"model: gemini-2.5-flash\n",
		"  - \"replace\"\n",
		"  - \"write_file\"\n",
	)

	for _, skill := range []string{"plan", "review-dag", "standup"} {
		for _, path := range []string{
			filepath.Join(".pi", "agent", "skills", skill, "SKILL.md"),
			filepath.Join(".claude", "skills", skill, "SKILL.md"),
			filepath.Join(".codex", "skills", skill, "SKILL.md"),
			filepath.Join(".gemini", "antigravity-cli", "plugins", "dfiles", "skills", skill, "SKILL.md"),
		} {
			assertFileContains(t, out, path, "name: "+skill+"\n")
		}
	}
	for _, path := range []string{
		filepath.Join(".pi", "agent", "skills", "syncai", "SKILL.md"),
		filepath.Join(".claude", "skills", "syncai", "SKILL.md"),
		filepath.Join(".codex", "skills", "syncai", "SKILL.md"),
		filepath.Join(".gemini", "antigravity-cli", "plugins", "dfiles", "skills", "syncai", "SKILL.md"),
	} {
		assertFileContains(t, out, path, "name: syncai\n", "syncai guide")
	}
	for _, path := range []string{
		filepath.Join(".pi", "agent", "AGENTS.md"),
		filepath.Join(".claude", "CLAUDE.md"),
		filepath.Join(".codex", "AGENTS.md"),
		filepath.Join(".config", "opencode", "AGENTS.md"),
	} {
		assertFileContains(t, out, path, "# Shared agent instructions\n")
	}
	assertFileContains(t, out, filepath.Join(".pi", "agent", "extensions", "session-name", "index.ts"), "export default function sessionName")
	assertPathAbsent(t, out, filepath.Join(".pi", "agent", "extensions", "session-name", "extension.toml"))
	assertFileContains(t, out, filepath.Join(".pi", "agent", "extensions", "zelda-hearts.ts"), "export default function zeldaHearts")
	assertPathAbsent(t, out, filepath.Join(".pi", "agent", "extensions", "zelda-hearts.toml"))
	assertFileContains(t, out, filepath.Join(".pi", "agent", "agents", "senior-worker.md"), "model: openai-codex/gpt-5.6-sol\n")
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
			name:       "openai home",
			profile:    "openai",
			scope:      "home",
			path:       filepath.Join(".pi", "agent", "agents", "worker.md"),
			contains:   []string{"model: openai-codex/gpt-5.6-luna\n", "thinking: medium\n"},
			absentPath: filepath.Join(".pi", "agent", "skills", "review-dag", "SKILL.md"),
		},
		{
			name:       "mixed work",
			profile:    "mixed",
			scope:      "work",
			path:       filepath.Join(".pi", "agent", "agents", "senior-worker.md"),
			contains:   []string{"model: anthropic/claude-opus-5\n", "thinking: xhigh\n"},
			absentPath: filepath.Join(".pi", "agent", "skills", "standup", "SKILL.md"),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := completeExampleSource(t)
			out := renderCompleteExample(t, source, test.profile, test.scope)
			assertFileContains(t, out, test.path, test.contains...)
			if test.absentPath != "" {
				assertPathAbsent(t, out, test.absentPath)
			}
		})
	}
}

func TestCompleteExamplePackageManifestDemonstratesUniversalAndScopedResources(t *testing.T) {
	source := completeExampleSource(t)
	manifest, err := aipackages.Load(aipackages.DefaultPath(source))
	if err != nil {
		t.Fatalf("loading complete example package manifest: %v", err)
	}

	assertStringsEqual(t, manifest.Pi.Packages, []string{"npm:@tintinweb/pi-subagents", "npm:@tintinweb/pi-tasks"})
	assertStringsEqual(t, manifest.Pi.NPMCommand, []string{"npm"})
	assertStringsEqual(t, manifest.Claude.Plugins, []string{"code-simplifier@claude-plugins-official", "pr-review-toolkit@claude-plugins-official"})
	assertStringsEqual(t, manifest.Codex.Plugins, []string{"github@openai-curated"})

	home := manifest.ForScope("home")
	assertStringsEqual(t, home.Pi.Packages, []string{"npm:@narumitw/pi-goal", "npm:@tintinweb/pi-subagents", "npm:@tintinweb/pi-tasks"})
	assertStringsEqual(t, home.Pi.NPMCommand, []string{"npm"})

	work := manifest.ForScope("work")
	assertStringsEqual(t, work.Pi.Packages, []string{"npm:@tintinweb/pi-subagents", "npm:@tintinweb/pi-tasks"})
	assertStringsEqual(t, work.Pi.NPMCommand, []string{"pnpm"})
}

func assertStringsEqual(t *testing.T, got, want []string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("values = %v, want %v", got, want)
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

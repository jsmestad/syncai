package packages

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestManifestIncludesCodexPlugins(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "packages.json")
	raw := []byte(`{
  "codex": {
    "plugins": [
      "superpowers@openai-curated",
      "",
      "github@openai-curated",
      "superpowers@openai-curated"
    ]
  }
}`)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"github@openai-curated", "superpowers@openai-curated"}
	if !reflect.DeepEqual(m.Codex.Plugins, want) {
		t.Fatalf("codex plugins = %#v, want %#v", m.Codex.Plugins, want)
	}

	out := filepath.Join(dir, "saved.json")
	if err := Save(out, m); err != nil {
		t.Fatal(err)
	}
	var saved map[string]struct {
		Plugins []string `json:"plugins"`
	}
	if err := json.Unmarshal(mustRead(t, out), &saved); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(saved["codex"].Plugins, want) {
		t.Fatalf("saved codex plugins = %#v, want %#v", saved["codex"].Plugins, want)
	}
}

func TestCompareIncludesCodexPluginsFromConfig(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".codex", "config.toml"), `model = "gpt-5.5"

[plugins."superpowers@openai-curated"]
enabled = true

[plugins."disabled@openai-curated"]
enabled = false

[plugins."github@openai-curated"]
enabled = true
`)
	desired := &Manifest{Codex: CodexManifest{Plugins: []string{"github@openai-curated", "build-ios-apps@openai-curated"}}}
	desired.Normalize()

	st, err := Compare(home, desired)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(st.Codex.OK, []string{"github@openai-curated"}) {
		t.Fatalf("codex ok = %#v", st.Codex.OK)
	}
	if !reflect.DeepEqual(st.Codex.Missing, []string{"build-ios-apps@openai-curated"}) {
		t.Fatalf("codex missing = %#v", st.Codex.Missing)
	}
	if !reflect.DeepEqual(st.Codex.Untracked, []string{"superpowers@openai-curated"}) {
		t.Fatalf("codex untracked = %#v", st.Codex.Untracked)
	}
}

func TestMergeInstalledIncludesUntrackedCodexPlugins(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "packages.json")
	writeFile(t, path, `{
  "codex": {
    "plugins": [
      "github@openai-curated"
    ]
  }
}`)

	err := MergeInstalled(path, Status{
		Codex: ResourceStatus{Untracked: []string{"superpowers@openai-curated"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	m, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"github@openai-curated", "superpowers@openai-curated"}
	if !reflect.DeepEqual(m.Codex.Plugins, want) {
		t.Fatalf("codex plugins = %#v, want %#v", m.Codex.Plugins, want)
	}
}

func TestManifestResolvesPiPackagesByScope(t *testing.T) {
	m := &Manifest{Pi: PiManifest{
		Packages: []string{"npm:pi-lens"},
		PackagesByScope: map[string][]string{
			"work": {"https://github.com/example/work-pi-package"},
		},
		NPMCommandByScope: map[string][]string{
			"home": {"npm"},
			"work": {"pnpm"},
		},
	}}
	m.Normalize()

	work := m.ForScope("work")
	wantWork := []string{"https://github.com/example/work-pi-package", "npm:pi-lens"}
	if !reflect.DeepEqual(work.Pi.Packages, wantWork) {
		t.Fatalf("work packages = %#v, want %#v", work.Pi.Packages, wantWork)
	}
	if got := m.ForScope("home").Pi.Packages; !reflect.DeepEqual(got, []string{"npm:pi-lens"}) {
		t.Fatalf("home packages = %#v", got)
	}
	if !reflect.DeepEqual(work.Pi.NPMCommand, []string{"pnpm"}) {
		t.Fatalf("work npm command = %#v, want [pnpm]", work.Pi.NPMCommand)
	}
	if !reflect.DeepEqual(m.ForScope("home").Pi.NPMCommand, []string{"npm"}) {
		t.Fatalf("home npm command = %#v, want [npm]", m.ForScope("home").Pi.NPMCommand)
	}
	if len(work.Pi.PackagesByScope) != 0 || len(work.Pi.NPMCommandByScope) != 0 {
		t.Fatalf("resolved manifest retained scoped Pi settings: %#v", work.Pi)
	}
}

func TestMergeInstalledPiPackageIntoSelectedScope(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "packages.json")
	writeFile(t, path, `{"pi":{"packages":["npm:pi-lens"]}}`)

	if err := MergeInstalledForScope(path, Status{Pi: ResourceStatus{Untracked: []string{"git:github.com/example/work-only"}}}, "work"); err != nil {
		t.Fatal(err)
	}
	m, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(m.Pi.Packages, []string{"npm:pi-lens"}) {
		t.Fatalf("base packages = %#v", m.Pi.Packages)
	}
	if !reflect.DeepEqual(m.Pi.PackagesByScope["work"], []string{"git:github.com/example/work-only"}) {
		t.Fatalf("work packages = %#v", m.Pi.PackagesByScope["work"])
	}
}

func TestHomeScopeRemovesWorkOnlyPiPackage(t *testing.T) {
	home := t.TempDir()
	m := &Manifest{Pi: PiManifest{
		Packages: []string{"npm:pi-lens"},
		PackagesByScope: map[string][]string{
			"work": {"https://github.com/example/work-pi-package"},
		},
	}}
	m.Normalize()
	if err := ApplyPiSettings(context.Background(), home, m.ForScope("work").Pi); err != nil {
		t.Fatal(err)
	}
	if err := ApplyPiSettings(context.Background(), home, m.ForScope("home").Pi); err != nil {
		t.Fatal(err)
	}

	settingsPath := filepath.Join(home, ".pi", "agent", "settings.json")
	var settings map[string]any
	if err := json.Unmarshal(mustRead(t, settingsPath), &settings); err != nil {
		t.Fatal(err)
	}
	if got := packageValues(settings["packages"]); !reflect.DeepEqual(got, []string{"npm:pi-lens"}) {
		t.Fatalf("home packages = %#v", got)
	}
}

func TestApplyPiSettingsReconcilesPackageManifest(t *testing.T) {
	home := t.TempDir()
	settingsPath := filepath.Join(home, ".pi", "agent", "settings.json")
	writeFile(t, settingsPath, `{
  "theme": "dark",
  "npmCommand": ["pnpm"],
  "packages": [
    "npm:pi-lens",
    "npm:pi-rtk-optimizer",
    {
      "source": "npm:@tintinweb/pi-subagents",
      "enabled": true
    },
    {
      "source": "npm:untracked-object",
      "enabled": true
    }
  ]
}`)

	if err := ApplyPiSettings(context.Background(), home, PiManifest{
		Packages:   []string{"npm:pi-lens", "npm:@tintinweb/pi-subagents"},
		NPMCommand: []string{"npm"},
	}); err != nil {
		t.Fatal(err)
	}

	var settings map[string]any
	if err := json.Unmarshal(mustRead(t, settingsPath), &settings); err != nil {
		t.Fatal(err)
	}
	got := packageValues(settings["packages"])
	want := []string{"npm:@tintinweb/pi-subagents", "npm:pi-lens"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("packages = %#v, want %#v", got, want)
	}
	if got := packageValues(settings["npmCommand"]); !reflect.DeepEqual(got, []string{"npm"}) {
		t.Fatalf("npmCommand = %#v, want [npm]", got)
	}
}

func TestApplyCodexAddsMissingPlugins(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".codex", "config.toml"), `[plugins."github@openai-curated"]
enabled = true
`)
	var calls [][]string
	oldLookPath := lookPath
	lookPath = func(name string) (string, error) {
		return "/bin/" + name, nil
	}
	oldRun := runCommand
	runCommand = func(_ context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{name}, args...))
		return nil, nil
	}
	t.Cleanup(func() {
		lookPath = oldLookPath
		runCommand = oldRun
	})

	errs := ApplyCodex(context.Background(), home, CodexManifest{Plugins: []string{"github@openai-curated", "superpowers@openai-curated"}})
	if len(errs) > 0 {
		t.Fatalf("ApplyCodex errors = %#v", errs)
	}

	want := [][]string{{"codex", "plugin", "add", "superpowers@openai-curated"}}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
}

func TestRunCommandStopsWhenContextCanceled(t *testing.T) {
	t.Setenv("SYNCAI_PACKAGE_HELPER_PROCESS", "sleep")
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	time.AfterFunc(100*time.Millisecond, cancel)

	out, err := runCommand(ctx, os.Args[0], "-test.run=^TestPackageCommandHelper$")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("runCommand error = %v, want context.Canceled", err)
	}
	if !strings.Contains(string(out), "helper started") {
		t.Fatalf("runCommand output = %q, want helper diagnostic", out)
	}
}

func TestRunCommandPreservesOrdinaryFailureOutput(t *testing.T) {
	t.Setenv("SYNCAI_PACKAGE_HELPER_PROCESS", "fail")

	out, err := runCommand(context.Background(), os.Args[0], "-test.run=^TestPackageCommandHelper$")
	if err == nil {
		t.Fatal("runCommand error = nil, want command failure")
	}
	if !strings.Contains(string(out), "helper failed") {
		t.Fatalf("runCommand output = %q, want failure diagnostic", out)
	}
}

func TestPackageCommandHelper(t *testing.T) {
	switch os.Getenv("SYNCAI_PACKAGE_HELPER_PROCESS") {
	case "sleep":
		_, _ = os.Stdout.WriteString("helper started\n")
		time.Sleep(10 * time.Second)
	case "fail":
		_, _ = os.Stderr.WriteString("helper failed\n")
		os.Exit(7)
	default:
		return
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

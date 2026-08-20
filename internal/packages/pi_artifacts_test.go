package packages

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCompareReportsOrphanedPiArtifacts(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".pi", "agent", "settings.json"), `{
  "packages": [
    "npm:@tintinweb/pi-subagents@0.13.0",
    "https://github.com/example/desired"
  ]
}`)
	writeFile(t, filepath.Join(home, ".pi", "agent", "npm", "package.json"), `{
  "name": "pi-extensions",
  "private": true,
  "dependencies": {
    "@tintinweb/pi-subagents": "^0.13.0",
    "pi-subagents": "^0.34.0"
  }
}`)
	writeFile(t, filepath.Join(home, ".pi", "agent", "npm", "node_modules", "transitive-only", "package.json"), `{"name":"transitive-only"}`)
	writeFile(t, filepath.Join(home, ".pi", "agent", "git", "github.com", "example", "desired", ".git", "config"), "")
	writeFile(t, filepath.Join(home, ".pi", "agent", "git", "github.com", "example", "retired", ".git", "config"), "")

	desired := &Manifest{Pi: PiManifest{Packages: []string{
		"npm:@tintinweb/pi-subagents@0.13.0",
		"https://github.com/example/desired",
	}}}
	desired.Normalize()
	status, err := Compare(home, desired)
	if err != nil {
		t.Fatal(err)
	}

	wantOK := []string{"https://github.com/example/desired", "npm:@tintinweb/pi-subagents@0.13.0"}
	if !reflect.DeepEqual(status.Pi.OK, wantOK) {
		t.Fatalf("Pi OK = %#v, want %#v", status.Pi.OK, wantOK)
	}
	wantOrphaned := []string{"git:github.com/example/retired", "npm:pi-subagents"}
	if !reflect.DeepEqual(status.Pi.Orphaned, wantOrphaned) {
		t.Fatalf("Pi orphaned = %#v, want %#v", status.Pi.Orphaned, wantOrphaned)
	}
}

func TestApplyPiWithAbsentManifestLeavesArtifactsUntouched(t *testing.T) {
	home := t.TempDir()
	npmManifestPath := filepath.Join(home, ".pi", "agent", "npm", "package.json")
	gitPath := filepath.Join(home, ".pi", "agent", "git", "github.com", "example", "retired")
	writeFile(t, npmManifestPath, `{"dependencies":{"retired":"1.0.0"}}`)
	writeFile(t, filepath.Join(gitPath, ".git", "config"), "")

	oldRun := runCommand
	runCommand = func(_ context.Context, name string, args ...string) ([]byte, error) {
		t.Fatalf("empty manifest unexpectedly invoked %s with %#v", name, args)
		return nil, nil
	}
	t.Cleanup(func() { runCommand = oldRun })

	if err := ApplyPi(context.Background(), home, PiManifest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(npmManifestPath); err != nil {
		t.Fatalf("npm package inventory was removed: %v", err)
	}
	if _, err := os.Stat(gitPath); err != nil {
		t.Fatalf("Git package was removed: %v", err)
	}
}

func TestApplyPiWithEmptyPackageListAndCommandRemovesAllArtifacts(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".pi", "agent", "npm", "package.json"), `{"dependencies":{"retired":"1.0.0"}}`)
	gitPath := filepath.Join(home, ".pi", "agent", "git", "github.com", "example", "retired")
	writeFile(t, filepath.Join(gitPath, ".git", "config"), "")

	var calls [][]string
	oldRun := runCommand
	runCommand = func(_ context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{name}, args...))
		return nil, nil
	}
	t.Cleanup(func() { runCommand = oldRun })

	if err := ApplyPi(context.Background(), home, PiManifest{NPMCommand: []string{"npm"}}); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0][0] != "npm" {
		t.Fatalf("package-manager calls = %#v, want one npm uninstall", calls)
	}
	if _, err := os.Stat(gitPath); !os.IsNotExist(err) {
		t.Fatalf("Git package still exists at %s", gitPath)
	}
}

func TestApplyPiUsesInstalledPackageManagerWhenManifestCommandIsUnscoped(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".pi", "agent", "settings.json"), `{"npmCommand":["pnpm"]}`)
	writeFile(t, filepath.Join(home, ".pi", "agent", "npm", "package.json"), `{"dependencies":{"desired":"1.0.0","retired":"1.0.0"}}`)

	var calls [][]string
	oldRun := runCommand
	runCommand = func(_ context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{name}, args...))
		return nil, nil
	}
	t.Cleanup(func() { runCommand = oldRun })

	if err := ApplyPi(context.Background(), home, PiManifest{Packages: []string{"npm:desired"}}); err != nil {
		t.Fatal(err)
	}
	want := []string{"pnpm", "uninstall", "--prefix", filepath.Join(home, ".pi", "agent", "npm"), "--", "retired"}
	if !reflect.DeepEqual(calls, [][]string{want}) {
		t.Fatalf("package-manager calls = %#v, want %#v", calls, [][]string{want})
	}
}

func TestApplyPiRemovesOrphanedArtifactsBeforeReconcilingSettings(t *testing.T) {
	home := t.TempDir()
	settingsPath := filepath.Join(home, ".pi", "agent", "settings.json")
	writeFile(t, settingsPath, `{
  "packages": [
    {
      "source": "npm:@tintinweb/pi-subagents",
      "extensions": ["src/index.ts"]
    },
    "npm:pi-subagents",
    "https://github.com/example/desired",
    "https://github.com/example/retired"
  ]
}`)
	writeFile(t, filepath.Join(home, ".pi", "agent", "npm", "package.json"), `{
  "name": "pi-extensions",
  "private": true,
  "dependencies": {
    "@tintinweb/pi-subagents": "^0.13.0",
    "pi-subagents": "^0.34.0"
  }
}`)
	desiredGitPath := filepath.Join(home, ".pi", "agent", "git", "github.com", "example", "desired")
	retiredGitPath := filepath.Join(home, ".pi", "agent", "git", "github.com", "example", "retired")
	writeFile(t, filepath.Join(desiredGitPath, ".git", "config"), "")
	writeFile(t, filepath.Join(retiredGitPath, ".git", "config"), "")

	var calls [][]string
	oldRun := runCommand
	runCommand = func(_ context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{name}, args...))
		return nil, nil
	}
	t.Cleanup(func() { runCommand = oldRun })

	desired := PiManifest{
		Packages: []string{
			"npm:@tintinweb/pi-subagents",
			"https://github.com/example/desired",
		},
		NPMCommand: []string{"pnpm"},
	}
	if err := ApplyPi(context.Background(), home, desired); err != nil {
		t.Fatal(err)
	}

	wantCalls := [][]string{{"pnpm", "uninstall", "--prefix", filepath.Join(home, ".pi", "agent", "npm"), "--", "pi-subagents"}}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("package-manager calls = %#v, want %#v", calls, wantCalls)
	}
	if _, err := os.Stat(retiredGitPath); !os.IsNotExist(err) {
		t.Fatalf("retired Git package still exists at %s", retiredGitPath)
	}
	if _, err := os.Stat(desiredGitPath); err != nil {
		t.Fatalf("desired Git package was removed from %s: %v", desiredGitPath, err)
	}

	var settings map[string]any
	if err := json.Unmarshal(mustRead(t, settingsPath), &settings); err != nil {
		t.Fatal(err)
	}
	wantPackages := []string{"https://github.com/example/desired", "npm:@tintinweb/pi-subagents"}
	if got := packageValues(settings["packages"]); !reflect.DeepEqual(got, wantPackages) {
		t.Fatalf("settings packages = %#v, want %#v", got, wantPackages)
	}
	packages := packageArray(settings["packages"])
	filtered, ok := packages[0].(map[string]any)
	if !ok || objectSource(filtered) != "npm:@tintinweb/pi-subagents" {
		t.Fatalf("filtered desired package was not preserved: %#v", packages[0])
	}
}

func TestApplyPiUsesScopeSpecificPackageManager(t *testing.T) {
	manifest := &Manifest{Pi: PiManifest{
		Packages: []string{"npm:desired"},
		NPMCommandByScope: map[string][]string{
			"home": {"npm"},
			"work": {"pnpm"},
		},
	}}
	manifest.Normalize()

	var calls [][]string
	oldRun := runCommand
	runCommand = func(_ context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{name}, args...))
		return nil, nil
	}
	t.Cleanup(func() { runCommand = oldRun })

	for _, test := range []struct {
		scope   string
		command string
	}{
		{scope: "home", command: "npm"},
		{scope: "work", command: "pnpm"},
	} {
		t.Run(test.scope, func(t *testing.T) {
			home := t.TempDir()
			writeFile(t, filepath.Join(home, ".pi", "agent", "npm", "package.json"), `{
  "dependencies": {
    "desired": "1.0.0",
    "retired": "1.0.0"
  }
}`)
			if err := ApplyPi(context.Background(), home, manifest.ForScope(test.scope).Pi); err != nil {
				t.Fatal(err)
			}
			call := calls[len(calls)-1]
			if call[0] != test.command {
				t.Fatalf("package manager = %q, want %q", call[0], test.command)
			}
			wantSuffix := []string{"uninstall", "--prefix", filepath.Join(home, ".pi", "agent", "npm"), "--", "retired"}
			if !reflect.DeepEqual(call[1:], wantSuffix) {
				t.Fatalf("package-manager args = %#v, want %#v", call[1:], wantSuffix)
			}
		})
	}
}

func TestPiManagedPackageIdentityIgnoresNpmVersionsAndGitRefs(t *testing.T) {
	tests := map[string]string{
		"npm:pi-lens@3.8.69":                         "npm:pi-lens",
		"npm:@tintinweb/pi-subagents@^0.13.0":        "npm:@tintinweb/pi-subagents",
		"https://github.com/example/repo@main":       "git:github.com/example/repo",
		"git:git@github.com:example/repo.git@v1.2.3": "git:github.com/example/repo",
		"git:github.com/example/repo@feature/test":   "git:github.com/example/repo",
	}
	for source, want := range tests {
		t.Run(source, func(t *testing.T) {
			got, ok := piManagedPackageIdentity(source)
			if !ok || got != want {
				t.Fatalf("identity = %q, %v, want %q, true", got, ok, want)
			}
		})
	}
}

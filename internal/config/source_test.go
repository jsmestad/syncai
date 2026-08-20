package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultSourceUsesXDGConfigHome(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)

	got, err := DefaultSource()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "syncai", "ai-source")
	if got != want {
		t.Fatalf("DefaultSource() = %q, want %q", got, want)
	}
}

func TestDefaultSourceFallsBackToHomeConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", home)

	got, err := DefaultSource()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".config", "syncai", "ai-source")
	if got != want {
		t.Fatalf("DefaultSource() = %q, want %q", got, want)
	}
}

func TestResolveSourcePrecedence(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	if _, err := SaveSource(filepath.Join(root, "saved")); err != nil {
		t.Fatal(err)
	}
	t.Setenv(sourceEnvironment, filepath.Join(root, "environment"))

	got, err := ResolveSource(filepath.Join(root, "explicit"))
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "explicit"); got != want {
		t.Fatalf("explicit source = %q, want %q", got, want)
	}

	got, err = ResolveSource("")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "environment"); got != want {
		t.Fatalf("environment source = %q, want %q", got, want)
	}

	t.Setenv(sourceEnvironment, "")
	got, err = ResolveSource("")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "saved"); got != want {
		t.Fatalf("saved source = %q, want %q", got, want)
	}
}

func TestResolveSourceFallsBackToWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv(sourceEnvironment, "")
	working := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(working); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	got, err := ResolveSource("")
	if err != nil {
		t.Fatal(err)
	}
	resolvedWorking, err := filepath.EvalSymlinks(working)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(resolvedWorking, "ai-source"); got != want {
		t.Fatalf("fallback source = %q, want %q", got, want)
	}
}

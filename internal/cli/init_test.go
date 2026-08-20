package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jsmestad/syncai/internal/config"
)

func TestInitCreatesDefaultStarterAndSavesSource(t *testing.T) {
	configRoot := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configRoot)
	t.Setenv("SYNCAI_SOURCE", "")
	var output bytes.Buffer
	app := New(Streams{In: strings.NewReader(""), Out: &output, Err: &bytes.Buffer{}})

	if err := app.Execute(context.Background(), []string{"init"}); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(configRoot, "syncai", "ai-source")
	for _, relative := range []string{"agents/worker.md", "instructions/global.md", "model-profiles.json", "packages.json"} {
		if _, err := os.Stat(filepath.Join(source, relative)); err != nil {
			t.Errorf("starter file %s: %v", relative, err)
		}
	}
	saved, err := config.LoadSource()
	if err != nil {
		t.Fatal(err)
	}
	if saved != source {
		t.Fatalf("saved source = %q, want %q", saved, source)
	}
	if !strings.Contains(output.String(), "created starter source at "+source) {
		t.Fatalf("init output = %q", output.String())
	}
}

func TestInitRegistersExistingSourceWithoutChangingIt(t *testing.T) {
	configRoot := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configRoot)
	t.Setenv("SYNCAI_SOURCE", "")
	source := completeExampleSource(t)
	before := snapshotExample(t, source)
	app := New(Streams{In: strings.NewReader(""), Out: &bytes.Buffer{}, Err: &bytes.Buffer{}})

	if err := app.Execute(context.Background(), []string{"init", source}); err != nil {
		t.Fatal(err)
	}
	assertSnapshotsEqual(t, before, snapshotExample(t, source))
	saved, err := config.LoadSource()
	if err != nil {
		t.Fatal(err)
	}
	if saved != source {
		t.Fatalf("saved source = %q, want %q", saved, source)
	}
}

func TestSavedSourceWorksOutsideItsDirectory(t *testing.T) {
	configRoot := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configRoot)
	t.Setenv("SYNCAI_SOURCE", "")
	source := completeExampleSource(t)
	app := New(Streams{In: strings.NewReader(""), Out: &bytes.Buffer{}, Err: &bytes.Buffer{}})
	if err := app.Execute(context.Background(), []string{"init", source}); err != nil {
		t.Fatal(err)
	}

	working := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(working); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	var output bytes.Buffer
	app = New(Streams{In: strings.NewReader(""), Out: &output, Err: &bytes.Buffer{}})
	if err := app.Execute(context.Background(), []string{"validate"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "ok: parsed 2 agents") {
		t.Fatalf("validate output = %q", output.String())
	}
}

func TestInitRejectsInvalidExistingSourceWithoutSavingIt(t *testing.T) {
	configRoot := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configRoot)
	t.Setenv("SYNCAI_SOURCE", "")
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "notes.txt"), []byte("not a source"), 0o644); err != nil {
		t.Fatal(err)
	}
	app := New(Streams{In: strings.NewReader(""), Out: &bytes.Buffer{}, Err: &bytes.Buffer{}})

	err := app.Execute(context.Background(), []string{"init", source})
	if err == nil || !strings.Contains(err.Error(), "is not renderable") {
		t.Fatalf("init error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(configRoot, "syncai", "config.json")); !os.IsNotExist(err) {
		t.Fatalf("config written after invalid source: %v", err)
	}
}

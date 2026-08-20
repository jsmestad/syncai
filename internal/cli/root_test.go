package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRootHelpListsCommands(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := New(Streams{In: strings.NewReader(""), Out: &stdout, Err: &stderr})

	if err := app.Execute(context.Background(), []string{"--help"}); err != nil {
		t.Fatalf("executing help: %v", err)
	}

	help := stdout.String()
	for _, command := range []string{"render", "validate", "set-profile", "use-profile", "import", "status", "pull", "list", "packages"} {
		if !strings.Contains(help, command) {
			t.Errorf("help does not list %q:\n%s", command, help)
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("help wrote to stderr: %q", stderr.String())
	}
}

func TestExecuteReturnsUnknownCommandError(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := New(Streams{In: strings.NewReader(""), Out: &stdout, Err: &stderr})

	err := app.Execute(context.Background(), []string{"not-a-command"})
	if err == nil {
		t.Fatal("expected an unknown command error")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("unexpected error: %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("unknown command wrote to stderr: %q", stderr.String())
	}
}

func TestExecuteCanceledContextPreventsRenderMutation(t *testing.T) {
	app := New(Streams{In: strings.NewReader(""), Out: &bytes.Buffer{}, Err: &bytes.Buffer{}})
	out := filepath.Join(t.TempDir(), "rendered")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := app.Execute(ctx, []string{"render", "--source", completeExampleSource(t), "--out", out, "--profile", "balanced"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute error = %v, want context.Canceled", err)
	}
	if _, statErr := os.Stat(out); !os.IsNotExist(statErr) {
		t.Fatalf("canceled render mutated output %s: %v", out, statErr)
	}
}

func TestRootCommandsHonorCobraWriterOverride(t *testing.T) {
	var appOutput bytes.Buffer
	var commandOutput bytes.Buffer
	app := New(Streams{In: strings.NewReader(""), Out: &appOutput, Err: &bytes.Buffer{}})
	root := app.Root()
	root.SetOut(&commandOutput)
	root.SetArgs([]string{"validate", "--source", completeExampleSource(t), "--profile", "balanced"})

	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("executing validate: %v", err)
	}
	if !strings.Contains(commandOutput.String(), "ok: parsed") {
		t.Fatalf("command output override did not receive validate output: %q", commandOutput.String())
	}
	if appOutput.Len() != 0 {
		t.Fatalf("validate bypassed Cobra output inheritance: %q", appOutput.String())
	}
}

func TestRenderReturnsDefaultOutputRootError(t *testing.T) {
	t.Setenv("HOME", "")
	app := New(Streams{In: strings.NewReader(""), Out: &bytes.Buffer{}, Err: &bytes.Buffer{}})

	err := app.Execute(context.Background(), []string{"render", "--source", completeExampleSource(t), "--profile", "balanced"})
	if err == nil || !strings.Contains(err.Error(), "resolving default output root") {
		t.Fatalf("expected contextual home resolution error, got %v", err)
	}
}

func TestPackagesReturnsDefaultInstallRootError(t *testing.T) {
	t.Setenv("HOME", "")
	app := New(Streams{In: strings.NewReader(""), Out: &bytes.Buffer{}, Err: &bytes.Buffer{}})

	err := app.Execute(context.Background(), []string{"packages", "status", "--source", completeExampleSource(t)})
	if err == nil || !strings.Contains(err.Error(), "resolving default install root") {
		t.Fatalf("expected contextual home resolution error, got %v", err)
	}
}

func TestRepeatedExecuteDoesNotLeakRenderFlags(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := New(Streams{In: strings.NewReader(""), Out: &stdout, Err: &stderr})
	source := completeExampleSource(t)
	homeOutput := t.TempDir()
	allOutput := t.TempDir()

	if err := app.Execute(context.Background(), []string{"render", "--source", source, "--out", homeOutput, "--profile", "balanced", "--scope", "home"}); err != nil {
		t.Fatalf("executing scoped render: %v", err)
	}
	stdout.Reset()
	stderr.Reset()
	if err := app.Execute(context.Background(), []string{"render", "--source", source, "--out", allOutput, "--profile", "balanced"}); err != nil {
		t.Fatalf("executing unscoped render: %v", err)
	}

	assertPathAbsent(t, homeOutput, filepath.Join(".pi", "agent", "agents", "reviewer.md"))
	assertFileContains(t, allOutput, filepath.Join(".pi", "agent", "agents", "reviewer.md"), "model: example-lab/orbit-large\n")
}

func TestRootPreservesCommandFlagsAndRelationships(t *testing.T) {
	app := New(Streams{In: strings.NewReader(""), Out: &bytes.Buffer{}, Err: &bytes.Buffer{}})
	root := app.Root()
	tests := []struct {
		path     []string
		defaults map[string]string
	}{
		{path: []string{"render"}, defaults: map[string]string{"source": "ai-source", "out": "", "profile": "", "project": "", "scope": "", "check": "false", "dry-run": "false", "force": "false"}},
		{path: []string{"validate"}, defaults: map[string]string{"source": "ai-source", "profile": ""}},
		{path: []string{"use-profile"}, defaults: map[string]string{"source": "ai-source", "scope": "", "force": "false"}},
		{path: []string{"import"}, defaults: map[string]string{"source": "ai-source", "all": "false"}},
		{path: []string{"status"}, defaults: map[string]string{"source": "ai-source", "out": "", "scope": ""}},
		{path: []string{"pull"}, defaults: map[string]string{"source": "ai-source", "out": "", "scope": "", "all": "false"}},
		{path: []string{"list"}, defaults: map[string]string{"source": "ai-source", "scope": ""}},
		{path: []string{"packages", "status"}, defaults: map[string]string{"source": "ai-source", "out": "", "scope": ""}},
		{path: []string{"packages", "apply"}, defaults: map[string]string{"source": "ai-source", "out": "", "scope": ""}},
		{path: []string{"packages", "pull"}, defaults: map[string]string{"source": "ai-source", "out": "", "scope": ""}},
	}
	for _, test := range tests {
		command, _, err := root.Find(test.path)
		if err != nil {
			t.Errorf("finding %q: %v", strings.Join(test.path, " "), err)
			continue
		}
		for name, expected := range test.defaults {
			flag := command.Flags().Lookup(name)
			if flag == nil {
				t.Errorf("%q is missing --%s", strings.Join(test.path, " "), name)
				continue
			}
			if flag.DefValue != expected {
				t.Errorf("%q --%s default = %q, want %q", strings.Join(test.path, " "), name, flag.DefValue, expected)
			}
		}
	}
}

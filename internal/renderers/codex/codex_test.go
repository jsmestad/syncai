package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jsmestad/syncai/internal/renderers"
	"github.com/jsmestad/syncai/internal/schema"
)

func TestRenderAgentEmitsTOMLWithRequiredFields(t *testing.T) {
	a := &schema.Agent{
		Name:        "archie",
		Description: "Architectural thinking partner with quotes \"like this\".",
		Targets:     []string{"codex"},
		Path:        "test/archie.md",
		Body:        "You are an architectural advisor.\nYou help.\n",
		Fields: []schema.KV{
			{Key: "name", Value: "archie"},
			{Key: "description", Value: "Architectural thinking partner with quotes \"like this\"."},
			{Key: "targets", Value: "codex"},
			{Key: "tools", Value: "read, bash, grep, find, ls"},
			{Key: "modelRole", Value: "reasoning"},
		},
	}
	got, err := renderAgent(a, nil)
	if err != nil {
		t.Fatalf("renderAgent: %v", err)
	}
	out := string(got)

	if !strings.Contains(out, `name = "archie"`) {
		t.Errorf("missing TOML name line:\n%s", out)
	}
	if !strings.Contains(out, `developer_instructions = '''`) {
		t.Errorf("missing developer_instructions block:\n%s", out)
	}
	if !strings.Contains(out, `sandbox_mode = "read-only"`) {
		t.Errorf("expected sandbox_mode read-only when all tools are read-only, got:\n%s", out)
	}
	// Description quoting: %q escapes the embedded quotes.
	if !strings.Contains(out, `description = "Architectural thinking partner with quotes \"like this\"."`) {
		t.Errorf("description must be TOML-quoted with escaped inner quotes, got:\n%s", out)
	}
	// No model line without codex sub-target in profiles.
	if strings.Contains(out, "model =") {
		t.Errorf("model line should be absent without codex sub-target, got:\n%s", out)
	}
}

// Bodies legitimately contain `"""` (Kotlin/Python docstring examples) and
// backslashes (Swift KeyPaths like `\.dismiss`, regex examples). Both break
// TOML basic strings; the renderer must use a literal-string block so they
// pass through verbatim.
func TestRenderAgentBodyPreservesTripleQuoteAndBackslash(t *testing.T) {
	body := "Example call:\n\nserver.enqueue(MockResponse().setBody(\"\"\"{\"x\":1}\"\"\"))\n\nUse `@Environment(\\.dismiss)` not `presentationMode`.\n"
	a := &schema.Agent{
		Name:    "android-test-advisor",
		Targets: []string{"codex"},
		Body:    body,
		Fields:  []schema.KV{{Key: "name", Value: "android-test-advisor"}},
	}
	got, err := renderAgent(a, nil)
	if err != nil {
		t.Fatalf("renderAgent: %v", err)
	}
	out := string(got)
	if !strings.Contains(out, `"""{"x":1}"""`) {
		t.Errorf("triple-quoted Kotlin body did not survive rendering:\n%s", out)
	}
	if !strings.Contains(out, `\.dismiss`) {
		t.Errorf("backslash KeyPath did not survive rendering:\n%s", out)
	}
}

func TestRenderAgentRejectsLiteralTerminatorInBody(t *testing.T) {
	a := &schema.Agent{
		Name:    "broken",
		Targets: []string{"codex"},
		Body:    "Has '''triple-single''' which would close the block.\n",
		Fields:  []schema.KV{{Key: "name", Value: "broken"}},
	}
	if _, err := renderAgent(a, nil); err == nil {
		t.Fatalf("expected error when body contains ''', got nil")
	}
}

func TestInferSandboxRequiresAllReadOnly(t *testing.T) {
	if got := inferSandbox("read, bash, grep"); got != "read-only" {
		t.Errorf("read+bash+grep should be read-only, got %q", got)
	}
	if got := inferSandbox("read, edit"); got != "" {
		t.Errorf("any non-read-only tool should drop sandbox_mode, got %q", got)
	}
	if got := inferSandbox(""); got != "" {
		t.Errorf("empty tools should drop sandbox_mode, got %q", got)
	}
}

func TestRenderPreservesRTKInstructionsInclude(t *testing.T) {
	outRoot := t.TempDir()
	path := filepath.Join(outRoot, ".codex", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	include := "@/Users/example/.codex/RTK.md"
	if err := os.WriteFile(path, []byte("old instructions\n"+include+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := New().Render(renderers.Inputs{InstructionsGlobal: "canonical instructions\n"}, outRoot); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(got), include) != 1 {
		t.Fatalf("RTK include was not preserved exactly once:\n%s", got)
	}
	if strings.Contains(string(got), "old instructions") {
		t.Fatalf("unmanaged instructions were unexpectedly preserved:\n%s", got)
	}
}

func TestRenderCopiesSkillDirs(t *testing.T) {
	sourceRoot := t.TempDir()
	skillDir := filepath.Join(sourceRoot, "minga-ticket-runner")
	nestedDir := filepath.Join(skillDir, "references")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatalf("mkdir source skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("canonical skill\n"), 0o644); err != nil {
		t.Fatalf("write source skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nestedDir, "notes.md"), []byte("nested reference\n"), 0o644); err != nil {
		t.Fatalf("write nested skill file: %v", err)
	}

	outRoot := t.TempDir()
	written, err := New().Render(renderers.Inputs{SkillDirs: []string{skillDir}}, outRoot)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	target := filepath.Join(outRoot, ".codex", "skills", "minga-ticket-runner")
	if !containsPath(written, target) {
		t.Fatalf("written paths %v did not include copied skill dir %s", written, target)
	}
	assertFileContent(t, filepath.Join(target, "SKILL.md"), "canonical skill\n")
	assertFileContent(t, filepath.Join(target, "references", "notes.md"), "nested reference\n")
}

func containsPath(paths []string, want string) bool {
	for _, path := range paths {
		if path == want {
			return true
		}
	}
	return false
}

func assertFileContent(t *testing.T, path string, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("content of %s = %q, want %q", path, string(got), want)
	}
}

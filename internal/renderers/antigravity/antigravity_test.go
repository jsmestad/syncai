package antigravity

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jsmestad/syncai/internal/profiles"
	"github.com/jsmestad/syncai/internal/renderers"
	"github.com/jsmestad/syncai/internal/schema"
)

// Antigravity CLI is a fixed target (Google models only). These tests assert structural properties.

func TestRenderAgentResolvesFromFixedAntigravity(t *testing.T) {
	p := &profiles.File{
		ActiveProfile: "claude",
		Fixed: map[string]map[string]string{
			"antigravity": {"test": "gemini-2.5-flash"},
		},
	}
	a := &schema.Agent{
		Name:    "test-advisor",
		Path:    "agents/test-advisor.md",
		Targets: []string{string(schema.TargetAntigravity)},
		Fields: []schema.KV{
			{Key: "description", Value: "Architecture advisor: review systems"},
			{Key: "tools", Value: "bash, edit, find, grep, ls, read"},
			{Key: "modelRole", Value: "test"},
			{Key: "targets", Value: "antigravity"},
			{Key: "systemPromptMode", Value: "replace"},
			{Key: "fallbackRoles", Value: "fast"},
		},
		Body: "Review the design.\n",
	}
	got, err := renderAgent(a, p)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	out := string(got)

	// modelRole=test resolves under fixed.antigravity -> gemini-2.5-flash, regardless of activeProfile. Confirms the fixed-vs-toggleable split is honoured.
	if !strings.Contains(out, "model: gemini-2.5-flash\n") {
		t.Errorf("expected fixed.antigravity.test -> gemini-2.5-flash, got:\n%s", out)
	}
	if !strings.Contains(out, "description: \"") {
		t.Errorf("description must be quoted so colons parse as YAML string content, got:\n%s", out)
	}
	for _, tool := range []string{"grep_search", "glob", "list_directory", "read_file", "run_shell_command"} {
		if !strings.Contains(out, "  - \""+tool+"\"\n") {
			t.Errorf("tools must render as a YAML array with Antigravity tool names; missing %q in:\n%s", tool, out)
		}
	}
	if strings.Contains(out, "tools: read") {
		t.Errorf("tools must not render as a CSV string, got:\n%s", out)
	}
	if strings.Contains(out, "targets:") {
		t.Errorf("targets line must be stripped from antigravity output")
	}
	for _, dropped := range []string{"systemPromptMode", "fallbackRoles", "modelRole"} {
		if strings.Contains(out, dropped+":") {
			t.Errorf("Pi-only field %q must be stripped from antigravity output", dropped)
		}
	}
}

func TestRenderWritesAntigravityPluginLayout(t *testing.T) {
	out := t.TempDir()
	if _, err := New().Render(renderers.Inputs{}, out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, ".gemini", "antigravity-cli", "plugins", "dfiles", "agents")); err != nil {
		t.Fatal(err)
	}
}

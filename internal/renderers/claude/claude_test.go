package claude

import (
	"strings"
	"testing"

	"github.com/jsmestad/syncai/internal/schema"
)

func TestTranslateToolsCapitalisesAndCollapses(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"read, bash, grep, find, ls", "Read, Bash, Grep, Glob"},
		{"read, edit, write", "Read, Edit, Write"},
		{"grep", "Grep"},
		{"", ""},
		// Unknown tools pass through with first-letter capitalisation.
		{"read, mcp__custom", "Read, Mcp__custom"},
	}
	for _, c := range cases {
		if got := translateTools(c.in); got != c.want {
			t.Errorf("translateTools(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestRenderAgentDropsPiOnlyFields(t *testing.T) {
	a := &schema.Agent{
		Name:        "archie",
		Description: "Architectural advisor",
		Targets:     []string{"claude"},
		Path:        "test/archie.md",
		Body:        "You are an architectural advisor.\n",
		Fields: []schema.KV{
			{Key: "name", Value: "archie"},
			{Key: "description", Value: "Architectural advisor"},
			{Key: "targets", Value: "claude"},
			{Key: "tools", Value: "read, bash, grep, find, ls"},
			{Key: "modelRole", Value: "reasoning"},
			{Key: "fallbackRoles", Value: "code-high"},
			{Key: "systemPromptMode", Value: "replace"},
			{Key: "inheritProjectContext", Value: "true"},
			{Key: "inheritSkills", Value: "false"},
			{Key: "output", Value: "architecture.md"},
			{Key: "defaultProgress", Value: "true"},
			{Key: "maxSubagentDepth", Value: "1"},
		},
	}
	got, err := renderAgent(a, nil)
	if err != nil {
		t.Fatalf("renderAgent: %v", err)
	}
	out := string(got)

	if !strings.Contains(out, "name: archie\n") {
		t.Errorf("missing name line:\n%s", out)
	}
	if !strings.Contains(out, "tools: Read, Bash, Grep, Glob\n") {
		t.Errorf("expected translated tools line, got:\n%s", out)
	}
	if strings.Contains(out, "targets:") {
		t.Errorf("targets line must be stripped")
	}
	for _, dropped := range []string{
		"systemPromptMode", "inheritProjectContext", "inheritSkills",
		"output", "defaultProgress", "maxSubagentDepth",
		"fallbackRoles", "fallbackModels", "modelRole",
	} {
		if strings.Contains(out, dropped+":") {
			t.Errorf("Pi-only field %q must be stripped from Claude output", dropped)
		}
	}
	// No claude sub-target in profiles → model line should be absent (Claude
	// inherits session default).
	if strings.Contains(out, "model:") {
		t.Errorf("model line should be absent without claude sub-target, got:\n%s", out)
	}
}

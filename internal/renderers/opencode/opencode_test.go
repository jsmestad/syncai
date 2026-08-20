package opencode

import (
	"strings"
	"testing"

	"github.com/jsmestad/syncai/internal/schema"
)

func TestRenderAgentDropsNameAndEmitsPermissionMap(t *testing.T) {
	a := &schema.Agent{
		Name:        "archie",
		Description: "Architectural advisor",
		Targets:     []string{"opencode"},
		Path:        "test/archie.md",
		Body:        "You are an architectural advisor.\n",
		Fields: []schema.KV{
			{Key: "name", Value: "archie"},
			{Key: "description", Value: "Architectural advisor"},
			{Key: "targets", Value: "opencode"},
			{Key: "tools", Value: "read, bash, grep"},
			{Key: "modelRole", Value: "reasoning"},
		},
	}
	got, err := renderAgent(a, nil)
	if err != nil {
		t.Fatalf("renderAgent: %v", err)
	}
	out := string(got)

	// OpenCode uses the filename for the agent identifier — no `name:` line.
	if strings.Contains(out, "name:") {
		t.Errorf("OpenCode output must not contain name: line, got:\n%s", out)
	}
	if !strings.Contains(out, "description: Architectural advisor\n") {
		t.Errorf("missing description line:\n%s", out)
	}
	if !strings.Contains(out, "mode: subagent\n") {
		t.Errorf("missing mode: subagent line:\n%s", out)
	}
	if !strings.Contains(out, "permission:\n") {
		t.Errorf("missing permission: map:\n%s", out)
	}
	// read+grep are granted, bash is also granted, edit is not granted.
	if !strings.Contains(out, "  read: allow\n") {
		t.Errorf("read should be allow:\n%s", out)
	}
	if !strings.Contains(out, "  bash: allow\n") {
		t.Errorf("bash should be allow:\n%s", out)
	}
	if !strings.Contains(out, "  edit: deny\n") {
		t.Errorf("edit should default to deny:\n%s", out)
	}
}

func TestRenderAgentReadOnlyDeniesBash(t *testing.T) {
	a := &schema.Agent{
		Name:        "reader",
		Description: "Read-only agent",
		Targets:     []string{"opencode"},
		Path:        "test/reader.md",
		Body:        "Read.\n",
		Fields: []schema.KV{
			{Key: "name", Value: "reader"},
			{Key: "description", Value: "Read-only agent"},
			{Key: "tools", Value: "read, grep, find"},
		},
	}
	got, err := renderAgent(a, nil)
	if err != nil {
		t.Fatalf("renderAgent: %v", err)
	}
	out := string(got)
	if !strings.Contains(out, "  bash: deny\n") {
		t.Errorf("bash should be deny when not in source tools:\n%s", out)
	}
	if !strings.Contains(out, "  read: allow\n") {
		t.Errorf("read should be allow:\n%s", out)
	}
}

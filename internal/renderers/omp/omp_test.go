package omp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jsmestad/syncai/internal/profiles"
	"github.com/jsmestad/syncai/internal/renderers"
	"github.com/jsmestad/syncai/internal/schema"
)

func TestRenderWritesNativeOMPAgent(t *testing.T) {
	outRoot := t.TempDir()
	agent := &schema.Agent{
		Name:        "elixir-architect",
		Description: "Elixir advisor: OTP and Phoenix",
		Targets:     []string{string(schema.TargetOMP)},
		Path:        "agents/elixir-architect.md",
		Body:        "Review the Elixir shape.\n",
		Fields: []schema.KV{
			{Key: "description", Value: "Elixir advisor: OTP and Phoenix"},
			{Key: "tools", Value: "read, bash, grep, find, ls"},
			{Key: "modelRole", Value: "reasoning"},
			{Key: "output", Value: "elixir-craft.md"},
			{Key: "systemPromptMode", Value: "replace"},
			{Key: "inheritSkills", Value: "false"},
			{Key: "maxSubagentDepth", Value: "1"},
		},
	}
	profile := &profiles.File{
		ActiveProfile: "openai",
		Profiles: map[string]map[string]map[string]string{
			"openai": {"omp": {"reasoning": "openai-codex/gpt-5.6-sol:xhigh"}},
		},
	}

	written, err := New().Render(renderers.Inputs{Agents: []*schema.Agent{agent}, Profiles: profile}, outRoot)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	path := filepath.Join(outRoot, ".omp", "agent", "agents", "elixir-architect.md")
	if len(written) != 1 || written[0] != path {
		t.Fatalf("written = %v, want [%s]", written, path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	for _, want := range []string{
		"name: elixir-architect\n",
		"description: \"Elixir advisor: OTP and Phoenix\"\n",
		"tools: read, bash, grep, glob\n",
		"model: openai-codex/gpt-5.6-sol\n",
		"thinkingLevel: xhigh\n",
		"autoloadSkills: false\n",
		"Review the Elixir shape.\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered agent missing %q:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"targets:", "modelRole:", "output:", "systemPromptMode:", "maxSubagentDepth:", "spawns:"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("rendered agent contains unsupported field %q:\n%s", unwanted, got)
		}
	}
}

func TestRenderUsesProjectLayoutAndDelegationPolicy(t *testing.T) {
	outRoot := t.TempDir()
	agent := &schema.Agent{
		Name:        "archie",
		Description: "Architecture advisor",
		Targets:     []string{string(schema.TargetOMP)},
		Path:        "agents/archie.md",
		Body:        "Review architecture.\n",
		Fields: []schema.KV{
			{Key: "maxSubagentDepth", Value: "2"},
		},
	}

	written, err := New().Render(renderers.Inputs{Agents: []*schema.Agent{agent}, ProjectMode: true}, outRoot)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	path := filepath.Join(outRoot, ".omp", "agents", "archie.md")
	if len(written) != 1 || written[0] != path {
		t.Fatalf("written = %v, want [%s]", written, path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "spawns: *\n") {
		t.Fatalf("nested delegation policy missing:\n%s", raw)
	}
}

func TestRenderUsesExplicitOMPSpawnAllowlist(t *testing.T) {
	agent := &schema.Agent{
		Name:        "archie",
		Description: "Architecture advisor",
		Targets:     []string{string(schema.TargetOMP)},
		Path:        "agents/archie.md",
		Fields: []schema.KV{
			{Key: "maxSubagentDepth", Value: "2"},
			{Key: "ompSpawns", Value: "elixir-architect, go-architect, swift-expert"},
		},
	}

	got, err := renderAgent(agent, nil)
	if err != nil {
		t.Fatalf("renderAgent: %v", err)
	}
	if !strings.Contains(string(got), "spawns: elixir-architect, go-architect, swift-expert\n") {
		t.Fatalf("explicit spawn allowlist missing:\n%s", got)
	}
	if strings.Contains(string(got), "spawns: *\n") {
		t.Fatalf("wildcard spawn policy must not override explicit allowlist:\n%s", got)
	}
}

func TestRenderSkipsAgentsWithoutOMPTarget(t *testing.T) {
	outRoot := t.TempDir()
	agent := &schema.Agent{
		Name:        "pi-only",
		Description: "Pi only",
		Targets:     []string{string(schema.TargetPi)},
		Path:        "agents/pi-only.md",
	}

	written, err := New().Render(renderers.Inputs{Agents: []*schema.Agent{agent}}, outRoot)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if len(written) != 0 {
		t.Fatalf("written = %v, want none", written)
	}
}

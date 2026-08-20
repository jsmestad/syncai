package pi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jsmestad/syncai/internal/profiles"
	"github.com/jsmestad/syncai/internal/renderers"
	"github.com/jsmestad/syncai/internal/schema"
)

func TestRenderAgentResolvesModelAndFallbacks(t *testing.T) {
	p := &profiles.File{
		ActiveProfile: "openai",
		Profiles: map[string]map[string]map[string]string{
			"openai": {"pi": {"test": "openai-codex/gpt-5.6-sol:high"}},
		},
	}
	a := &schema.Agent{
		Name: "test-advisor",
		Path: "agents/test-advisor.md",
		Fields: []schema.KV{
			{Key: "description", Value: "Architecture advisor"},
			{Key: "modelRole", Value: "test"},
			{Key: "systemPromptMode", Value: "replace"},
			{Key: "inheritSkills", Value: "false"},
		},
		Body: "Review the design.\n",
	}
	got, err := renderAgent(a, p)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	out := string(got)

	// modelRole=test → openai-codex/gpt-5.6-sol:high under openai profile, pi target.
	if !strings.Contains(out, "model: openai-codex/gpt-5.6-sol\n") {
		t.Errorf("expected resolved model line, got:\n%s", out)
	}
	if !strings.Contains(out, "thinking: high\n") {
		t.Errorf("expected resolved thinking line, got:\n%s", out)
	}
	if !strings.Contains(out, "prompt_mode: replace\n") {
		t.Errorf("expected tintinweb prompt mode, got:\n%s", out)
	}
	if !strings.Contains(out, "skills: false\n") {
		t.Errorf("expected tintinweb skill setting, got:\n%s", out)
	}
	if !strings.Contains(out, "disallowed_tools: Agent, get_subagent_result, steer_subagent\n") {
		t.Errorf("expected nested delegation guard, got:\n%s", out)
	}
	for _, unsupported := range []string{"fallbackModels:", "systemPromptMode:", "maxSubagentDepth:"} {
		if strings.Contains(out, unsupported) {
			t.Errorf("unsupported tintinweb field %q must not be rendered", unsupported)
		}
	}
	// targets line stripped.
	if strings.Contains(out, "targets:") {
		t.Errorf("targets line must be stripped from Pi output")
	}
	// modelRole replaced with model, not retained.
	if strings.Contains(out, "modelRole:") {
		t.Errorf("modelRole must be replaced with resolved model")
	}
}

func TestRenderAgentDelegationGuardFollowsMaxDepth(t *testing.T) {
	for _, tc := range []struct {
		name        string
		depth       string
		wantGuarded bool
	}{
		{name: "missing depth", wantGuarded: true},
		{name: "depth zero", depth: "0", wantGuarded: true},
		{name: "depth one", depth: "1", wantGuarded: true},
		{name: "depth two", depth: "2", wantGuarded: false},
		{name: "malformed depth", depth: "many", wantGuarded: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fields := []schema.KV{{Key: "description", Value: "Architecture advisor"}}
			if tc.depth != "" {
				fields = append(fields, schema.KV{Key: "maxSubagentDepth", Value: tc.depth})
			}
			a := &schema.Agent{
				Name:    "architect",
				Targets: []string{string(schema.TargetPi)},
				Fields:  fields,
				Body:    "Review the design.\n",
			}

			got, err := renderAgent(a, nil)
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			guarded := strings.Contains(string(got), "disallowed_tools: Agent, get_subagent_result, steer_subagent\n")
			if guarded != tc.wantGuarded {
				t.Fatalf("guarded = %v, want %v; output:\n%s", guarded, tc.wantGuarded, got)
			}
		})
	}
}

func TestRenderWritesResolvedModelCatalog(t *testing.T) {
	out := t.TempDir()
	p := &profiles.File{
		ActiveProfile: "openai",
		Profiles: map[string]map[string]map[string]string{
			"openai": {"pi": {"code-fast": "beta-openai/luna"}},
		},
		Fixed: map[string]map[string]string{},
	}
	if _, err := New().Render(renderers.Inputs{Profiles: p}, out); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(out, ".pi", "agent", "model-profiles.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"code-fast": "beta-openai/luna"`) {
		t.Fatalf("resolved catalog missing environment override:\n%s", raw)
	}
}

func TestRenderAgentDefaultsTargetsToPi(t *testing.T) {
	// An agent without a targets field should still target Pi.
	raw := []byte("---\nname: reviewer\ndescription: Review code\n---\nReview the design.\n")
	a, err := schema.ParseAgent("agents/reviewer.md", raw)
	if err != nil {
		t.Fatalf("parsing source: %v", err)
	}
	if !a.HasTarget(schema.TargetPi) {
		t.Errorf("agent without explicit targets should default to [pi], got %v", a.Targets)
	}
}

func TestWriteAgentsSkipsAndRemovesLegacyChains(t *testing.T) {
	dir := t.TempDir()
	legacyPath := filepath.Join(dir, "plan-with-advice.chain.md")
	if err := os.WriteFile(legacyPath, []byte("legacy chain"), 0o644); err != nil {
		t.Fatal(err)
	}

	written, err := writeAgents(dir, renderers.Inputs{Agents: []*schema.Agent{
		{
			Name:    "plan-with-advice",
			Path:    "agents/plan-with-advice.chain.md",
			Targets: []string{string(schema.TargetPi)},
			Fields:  []schema.KV{{Key: "description", Value: "Legacy chain"}},
		},
		{
			Name:    "reviewer",
			Path:    "agents/reviewer.md",
			Targets: []string{string(schema.TargetPi)},
			Fields:  []schema.KV{{Key: "description", Value: "Review code"}},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(written) != 1 || written[0] != filepath.Join(dir, "reviewer.md") {
		t.Fatalf("written = %v, want only reviewer.md", written)
	}
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("legacy chain remains after render: %v", err)
	}
}

package load

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jsmestad/syncai/internal/schema"
)

// TestAgentsDualFileSameName verifies the dual-file scope-override pattern
// used for agents like ux-designer: two .md files share `name: ux-designer`
// but differ on `scope:`. load.Agents loads both; downstream filtering by
// scope yields exactly one variant per profile. This test pins the behavior
// so the override pattern can't silently break.
func TestAgentsDualFileSameName(t *testing.T) {
	dir := t.TempDir()
	mustWriteAgent(t, filepath.Join(dir, "ux-designer.md"), `---
name: ux-designer
description: generic UX advisor
targets: claude
scope: home
tools: read
modelRole: code-low
---
home body
`)
	mustWriteAgent(t, filepath.Join(dir, "ux-designer.work.md"), `---
name: ux-designer
description: work UX advisor
targets: claude
scope: work
tools: read
modelRole: code-low
---
work body
`)

	agents, err := Agents(dir)
	if err != nil {
		t.Fatalf("Agents: %v", err)
	}
	if len(agents) != 2 {
		t.Fatalf("want 2 agents loaded, got %d", len(agents))
	}

	scopes := map[string]string{}
	for _, a := range agents {
		if a.Name != "ux-designer" {
			t.Errorf("unexpected agent name: %q", a.Name)
		}
		scopes[a.ScopeString()] = a.Description
	}
	if scopes["home"] != "generic UX advisor" {
		t.Errorf("home variant missing or wrong: %q", scopes["home"])
	}
	if scopes["work"] != "work UX advisor" {
		t.Errorf("work variant missing or wrong: %q", scopes["work"])
	}

	homeOnly := matchScope(agents, "home")
	if len(homeOnly) != 1 || homeOnly[0].Description != "generic UX advisor" {
		t.Errorf("scope=home: expected only the home variant, got %+v", homeOnly)
	}
	workOnly := matchScope(agents, "work")
	if len(workOnly) != 1 || workOnly[0].Description != "work UX advisor" {
		t.Errorf("scope=work: expected only the work variant, got %+v", workOnly)
	}

	noFilter := matchScope(agents, "")
	if len(noFilter) != 2 {
		t.Errorf("no scope filter: expected both variants, got %d", len(noFilter))
	}
}

// matchScope mirrors cmd/syncai/main.go's filterByScope. The test package
// can't import from cmd, so this duplicates the trivial filtering logic.
func matchScope(agents []*schema.Agent, scope string) []*schema.Agent {
	if scope == "" {
		return agents
	}
	out := []*schema.Agent{}
	for _, a := range agents {
		if a.MatchesScope(scope) {
			out = append(out, a)
		}
	}
	return out
}

func mustWriteAgent(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile %s: %v", path, err)
	}
}

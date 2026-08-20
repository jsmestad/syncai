package pull

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jsmestad/syncai/internal/profiles"
)

// claudeProfiles returns a synthetic profile catalog with the Claude
// fixed-target sub-table populated, mirroring ai-source/model-profiles.json.
func claudeProfiles() *profiles.File {
	return &profiles.File{
		ActiveProfile: "openai",
		Profiles: map[string]map[string]map[string]string{
			"openai": {
				"pi": {"reasoning": "openai-codex/gpt-5.5:high"},
				"omp": {
					"code-fast": "openai-codex/gpt-5.6-luna:high",
					"reasoning": "openai-codex/gpt-5.6-sol:xhigh",
				},
			},
		},
		Fixed: map[string]map[string]string{
			"claude": {
				"code-fast":   "haiku",
				"code-medium": "sonnet",
				"code-high":   "opus",
				"reasoning":   "opus",
			},
			"codex": {
				"reasoning": "gpt-5.5:high",
				"design":    "gpt-5.4:medium",
			},
		},
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

const claudeFresh = `---
name: archie
description: Original description.
tools: Read, Bash, Grep, Glob
model: opus
---
Original body content.
`

// P1: Body-only edit in installed produces NewBody, no other fields.
func TestPlanForBodyOnlyEdit(t *testing.T) {
	d := t.TempDir()
	installed := filepath.Join(d, "claude/installed/archie.md")
	fresh := filepath.Join(d, "claude/fresh/archie.md")
	source := filepath.Join(d, "source/archie.md")
	writeFile(t, installed, strings.Replace(claudeFresh, "Original body content.", "Edited body content.", 1))
	writeFile(t, fresh, claudeFresh)
	writeFile(t, source, "---\nname: archie\ndescription: Original description.\ntargets: pi, claude\nscope: home\ntools: read, bash, grep, find\nmodelRole: reasoning\n---\nOriginal body content.\n")

	plan, err := PlanFor(installed, fresh, source, "claude", claudeProfiles())
	if err != nil {
		t.Fatalf("PlanFor: %v", err)
	}
	if plan.NewBody != "Edited body content.\n" {
		t.Errorf("NewBody: %q", plan.NewBody)
	}
	if plan.NewDescription != "" || plan.NewTools != "" || plan.NewModelRole != "" {
		t.Errorf("expected only body change, got plan=%+v", plan)
	}
	if plan.UnsupportedFrontmatter {
		t.Errorf("body-only edit should not be unsupported")
	}
}

// P2: Description-only edit in installed produces NewDescription.
func TestPlanForDescriptionOnlyEdit(t *testing.T) {
	d := t.TempDir()
	installed := filepath.Join(d, "claude/installed/archie.md")
	fresh := filepath.Join(d, "claude/fresh/archie.md")
	source := filepath.Join(d, "source/archie.md")
	writeFile(t, installed, strings.Replace(claudeFresh, "Original description.", "Updated description.", 1))
	writeFile(t, fresh, claudeFresh)
	writeFile(t, source, "---\nname: archie\ndescription: Original description.\ntargets: pi, claude\nscope: home\n---\nOriginal body content.\n")

	plan, err := PlanFor(installed, fresh, source, "claude", claudeProfiles())
	if err != nil {
		t.Fatalf("PlanFor: %v", err)
	}
	if plan.NewDescription != "Updated description." {
		t.Errorf("NewDescription: %q", plan.NewDescription)
	}
	if plan.NewBody != "" {
		t.Errorf("expected no body change, got NewBody=%q", plan.NewBody)
	}
	if plan.UnsupportedFrontmatter {
		t.Errorf("description-only edit should be supported")
	}
}

// Tools-list edit (Claude) reverse-translates to source vocabulary.
func TestPlanForToolsEditReversesClaudeVocabulary(t *testing.T) {
	d := t.TempDir()
	installed := filepath.Join(d, "claude/installed/archie.md")
	fresh := filepath.Join(d, "claude/fresh/archie.md")
	source := filepath.Join(d, "source/archie.md")
	writeFile(t, installed, strings.Replace(claudeFresh, "tools: Read, Bash, Grep, Glob", "tools: Read, Bash, Grep, Glob, Edit", 1))
	writeFile(t, fresh, claudeFresh)
	writeFile(t, source, "---\nname: archie\ndescription: Original description.\ntargets: pi, claude\nscope: home\ntools: read, bash, grep, find\n---\nbody.\n")

	plan, err := PlanFor(installed, fresh, source, "claude", claudeProfiles())
	if err != nil {
		t.Fatalf("PlanFor: %v", err)
	}
	if plan.NewTools != "read, bash, grep, find, edit" {
		t.Errorf("NewTools: %q (Glob should reverse to find, Edit lowercase)", plan.NewTools)
	}
	if plan.UnsupportedFrontmatter {
		t.Errorf("tools edit should reverse cleanly for Claude")
	}
}

// Model alias edit (Claude) reverses to modelRole via fixed.claude.
func TestPlanForModelEditReversesClaudeAlias(t *testing.T) {
	d := t.TempDir()
	installed := filepath.Join(d, "claude/installed/archie.md")
	fresh := filepath.Join(d, "claude/fresh/archie.md")
	source := filepath.Join(d, "source/archie.md")
	writeFile(t, installed, strings.Replace(claudeFresh, "model: opus", "model: haiku", 1))
	writeFile(t, fresh, claudeFresh)
	writeFile(t, source, "---\nname: archie\ndescription: Original description.\ntargets: pi, claude\nscope: home\nmodelRole: reasoning\n---\nbody.\n")

	plan, err := PlanFor(installed, fresh, source, "claude", claudeProfiles())
	if err != nil {
		t.Fatalf("PlanFor: %v", err)
	}
	if plan.NewModelRole != "code-fast" {
		t.Errorf("NewModelRole: got %q, want code-fast (haiku reverses to code-fast)", plan.NewModelRole)
	}
	if plan.UnsupportedFrontmatter {
		t.Errorf("model edit should reverse cleanly for Claude")
	}
}

func TestPlanForOMPToolsAndModelEdit(t *testing.T) {
	d := t.TempDir()
	installed := filepath.Join(d, "omp/installed/elixir-architect.md")
	fresh := filepath.Join(d, "omp/fresh/elixir-architect.md")
	source := filepath.Join(d, "source/elixir-architect.md")
	freshAgent := "---\nname: elixir-architect\ndescription: Elixir advisor.\ntools: read, bash, grep, glob\nmodel: openai-codex/gpt-5.6-sol\nthinkingLevel: xhigh\n---\nbody.\n"
	installedAgent := strings.Replace(freshAgent, "tools: read, bash, grep, glob", "tools: read, bash, grep, glob, edit", 1)
	installedAgent = strings.Replace(installedAgent, "model: openai-codex/gpt-5.6-sol\nthinkingLevel: xhigh", "model: openai-codex/gpt-5.6-luna\nthinkingLevel: high", 1)
	writeFile(t, installed, installedAgent)
	writeFile(t, fresh, freshAgent)
	writeFile(t, source, "---\nname: elixir-architect\ndescription: Elixir advisor.\ntargets: pi, omp\ntools: read, bash, grep, find\nmodelRole: reasoning\n---\nbody.\n")

	plan, err := PlanFor(installed, fresh, source, "omp", claudeProfiles())
	if err != nil {
		t.Fatalf("PlanFor: %v", err)
	}
	if plan.NewTools != "read, bash, grep, find, edit" {
		t.Errorf("NewTools = %q", plan.NewTools)
	}
	if plan.NewModelRole != "code-fast" {
		t.Errorf("NewModelRole = %q, want code-fast", plan.NewModelRole)
	}
	if plan.UnsupportedFrontmatter {
		t.Errorf("OMP tools and model edits should reverse cleanly")
	}
}

// P9: Malformed installed (missing closing ---) errors with file path in message.
func TestPlanForMalformedInstalledErrors(t *testing.T) {
	d := t.TempDir()
	installed := filepath.Join(d, "claude/installed/archie.md")
	fresh := filepath.Join(d, "claude/fresh/archie.md")
	source := filepath.Join(d, "source/archie.md")
	writeFile(t, installed, "---\nname: archie\ndescription: foo\nbody-without-closing-delimiter\n")
	writeFile(t, fresh, claudeFresh)
	writeFile(t, source, "---\nname: archie\ndescription: foo\n---\nbody.\n")

	_, err := PlanFor(installed, fresh, source, "claude", claudeProfiles())
	if err == nil {
		t.Fatal("expected error for malformed installed file")
	}
	if !strings.Contains(err.Error(), installed) {
		t.Errorf("error should name the installed path: %v", err)
	}
}

// P10: Apply with NewDescription rewrites only that line, preserving every
// other source field including order. THE load-bearing assertion.
func TestApplyDescriptionPreservesEverythingElse(t *testing.T) {
	d := t.TempDir()
	source := filepath.Join(d, "source/archie.md")
	original := `---
name: archie
description: Old description.
targets: pi, claude
scope: home
tools: read, bash
modelRole: reasoning
systemPromptMode: replace
output: false
---
Body content stays.
`
	writeFile(t, source, original)
	plan := Plan{SourcePath: source, NewDescription: "New description!"}
	if err := plan.Apply(); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	got, _ := os.ReadFile(source)
	want := strings.Replace(original, "description: Old description.", "description: New description!", 1)
	if string(got) != want {
		t.Errorf("Apply did not preserve source frontmatter exactly.\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// P11: Apply with NewBody replaces body, preserves frontmatter byte-exact.
func TestApplyBodyPreservesFrontmatter(t *testing.T) {
	d := t.TempDir()
	source := filepath.Join(d, "source/archie.md")
	writeFile(t, source, "---\nname: archie\ndescription: foo\ntargets: pi\nscope: home\n---\nold body.\n")
	plan := Plan{SourcePath: source, NewBody: "new body content.\n"}
	if err := plan.Apply(); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	got, _ := os.ReadFile(source)
	want := "---\nname: archie\ndescription: foo\ntargets: pi\nscope: home\n---\nnew body content.\n"
	if string(got) != want {
		t.Errorf("Apply output:\n%s\nwant:\n%s", got, want)
	}
}

// P12: Apply with both description and body applies both.
func TestApplyDescriptionAndBody(t *testing.T) {
	d := t.TempDir()
	source := filepath.Join(d, "source/archie.md")
	writeFile(t, source, "---\nname: archie\ndescription: old\n---\nold body.\n")
	plan := Plan{SourcePath: source, NewDescription: "new desc", NewBody: "new body.\n"}
	if err := plan.Apply(); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	got, _ := os.ReadFile(source)
	if !strings.Contains(string(got), "description: new desc") {
		t.Errorf("description not applied: %s", got)
	}
	if !strings.Contains(string(got), "new body.") {
		t.Errorf("body not applied: %s", got)
	}
}

// Codex TOML pull: body-only edit propagates.
func TestPlanForCodexBodyEdit(t *testing.T) {
	d := t.TempDir()
	installed := filepath.Join(d, "codex/installed/archie.toml")
	fresh := filepath.Join(d, "codex/fresh/archie.toml")
	source := filepath.Join(d, "source/archie.md")
	codexBase := `name = "archie"
description = "An advisor."
model = "gpt-5.5"
model_reasoning_effort = "high"
sandbox_mode = "read-only"
developer_instructions = '''
Original body.
'''
`
	writeFile(t, fresh, codexBase)
	writeFile(t, installed, strings.Replace(codexBase, "Original body.", "Edited body.", 1))
	writeFile(t, source, "---\nname: archie\ndescription: An advisor.\nmodelRole: reasoning\n---\nOriginal body.\n")

	plan, err := PlanFor(installed, fresh, source, "codex", claudeProfiles())
	if err != nil {
		t.Fatalf("PlanFor: %v", err)
	}
	if !strings.Contains(plan.NewBody, "Edited body.") {
		t.Errorf("NewBody: %q", plan.NewBody)
	}
	if plan.UnsupportedFrontmatter {
		t.Errorf("body-only Codex edit should be supported")
	}
}

// Codex model+effort change reverses to modelRole.
func TestPlanForCodexModelEditReverses(t *testing.T) {
	d := t.TempDir()
	installed := filepath.Join(d, "codex/installed/archie.toml")
	fresh := filepath.Join(d, "codex/fresh/archie.toml")
	source := filepath.Join(d, "source/archie.md")
	codexBase := `name = "archie"
description = "An advisor."
model = "gpt-5.5"
model_reasoning_effort = "high"
developer_instructions = '''
Body.
'''
`
	writeFile(t, fresh, codexBase)
	installedToml := strings.Replace(codexBase, `model = "gpt-5.5"`, `model = "gpt-5.4"`, 1)
	installedToml = strings.Replace(installedToml, `model_reasoning_effort = "high"`, `model_reasoning_effort = "medium"`, 1)
	writeFile(t, installed, installedToml)
	writeFile(t, source, "---\nname: archie\ndescription: An advisor.\nmodelRole: reasoning\n---\nBody.\n")

	plan, err := PlanFor(installed, fresh, source, "codex", claudeProfiles())
	if err != nil {
		t.Fatalf("PlanFor: %v", err)
	}
	// fixed.codex.design = "gpt-5.4:medium" → reverses to "design"
	if plan.NewModelRole != "design" {
		t.Errorf("NewModelRole: got %q, want design", plan.NewModelRole)
	}
	if plan.UnsupportedFrontmatter {
		t.Errorf("Codex model edit with catalog match should be supported")
	}
}

// HasChanges true iff any New* is non-empty.
func TestHasChanges(t *testing.T) {
	if (Plan{}).HasChanges() {
		t.Errorf("empty plan should have no changes")
	}
	if !(Plan{NewBody: "x"}).HasChanges() {
		t.Errorf("body change should count")
	}
	if !(Plan{NewModelRole: "x"}).HasChanges() {
		t.Errorf("model change should count")
	}
}

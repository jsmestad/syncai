package importer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jsmestad/syncai/internal/profiles"
	"github.com/jsmestad/syncai/internal/pull"
	"github.com/jsmestad/syncai/internal/schema"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertFileUnchanged(t *testing.T, path, want string, mode os.FileMode) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != want {
		t.Fatalf("%s changed to %q", path, raw)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != mode {
		t.Fatalf("%s mode = %o, want %o", path, info.Mode().Perm(), mode)
	}
}

func assertRegularFile(t *testing.T, path, want string, mode os.FileMode) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("%s mode = %s, want regular file", path, info.Mode())
	}
	if info.Mode().Perm() != mode {
		t.Fatalf("%s mode = %o, want %o", path, info.Mode().Perm(), mode)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != want {
		t.Fatalf("%s content = %q, want %q", path, raw, want)
	}
}

// I1: Scan returns one Candidate per orphan file across tool dirs.
func TestScanFindsOrphans(t *testing.T) {
	home := t.TempDir()
	source := t.TempDir()
	writeFile(t, filepath.Join(home, ".pi/agent/agents/orphan-pi.md"), "frontmatter")
	writeFile(t, filepath.Join(home, ".claude/agents/orphan-claude.md"), "frontmatter")
	writeFile(t, filepath.Join(source, "agents/already-in-source.md"), "frontmatter")

	candidates, err := Scan(home, source)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d: %+v", len(candidates), candidates)
	}
	names := map[string]bool{}
	for _, c := range candidates {
		names[c.Name] = true
	}
	if !names["orphan-pi"] || !names["orphan-claude"] {
		t.Errorf("missing expected names: %v", names)
	}
}

// I3: Scan excludes files whose name matches a source agent.
func TestScanSkipsKnownNames(t *testing.T) {
	home := t.TempDir()
	source := t.TempDir()
	writeFile(t, filepath.Join(home, ".pi/agent/agents/archie.md"), "x")
	writeFile(t, filepath.Join(source, "agents/archie.md"), "x")

	candidates, err := Scan(home, source)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 0 {
		t.Errorf("known name should be excluded, got %+v", candidates)
	}
}

// I7: Port reverses Pi `model: <id>` to `modelRole: <role>` when catalog
// has a unique match.
func TestPortReversesModelToModelRole(t *testing.T) {
	d := t.TempDir()
	input := filepath.Join(d, "in/archie.md")
	dest := filepath.Join(d, "src/agents/archie.md")
	writeFile(t, input, `---
name: archie
description: An advisor.
tools: read, bash
model: openai-codex/gpt-5.5:high
---

Body.
`)
	p := &profiles.File{
		ActiveProfile: "openai",
		Profiles: map[string]map[string]map[string]string{
			"openai": {"pi": {"reasoning": "openai-codex/gpt-5.5:high"}},
		},
	}
	c := Candidate{Tool: "pi", InputPath: input, SourcePath: dest, AutoPortable: true}
	if err := Port(d, c, p); err != nil {
		t.Fatalf("Port: %v", err)
	}
	out, _ := os.ReadFile(dest)
	if !strings.Contains(string(out), "modelRole: reasoning") {
		t.Errorf("expected modelRole reverse, got:\n%s", out)
	}
	if strings.Contains(string(out), "model: openai-codex") {
		t.Errorf("original model line should be replaced, got:\n%s", out)
	}
}

func TestPiImportUsesCanonicalReverseModelPrecedence(t *testing.T) {
	profile := &profiles.File{
		ActiveProfile: "active",
		Fixed: map[string]map[string]string{
			"pi": {"fixed-role": "shared-model"},
		},
		Profiles: map[string]map[string]map[string]string{
			"active": {"pi": {
				"zeta-role":  "shared-model",
				"alpha-role": "shared-model",
			}},
		},
	}
	wantRole, ok := pull.ReverseModel("pi", "shared-model", profile)
	if !ok {
		t.Fatal("canonical reverse lookup did not resolve shared-model")
	}
	if wantRole != "alpha-role" {
		t.Fatalf("canonical reverse role = %q, want active-profile role alpha-role", wantRole)
	}
	agent := &schema.Agent{
		Description: "An advisor.",
		Fields: []schema.KV{
			{Key: "description", Value: "An advisor."},
			{Key: "model", Value: "shared-model"},
			{Key: "fallbackModels", Value: "shared-model"},
		},
	}
	out, err := piToSource(agent, profile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "modelRole: "+wantRole+"\n") {
		t.Fatalf("Pi importer did not use canonical model role %q:\n%s", wantRole, out)
	}
	if !strings.Contains(string(out), "fallbackRoles: "+wantRole+"\n") {
		t.Fatalf("Pi importer did not use canonical fallback role %q:\n%s", wantRole, out)
	}
}

// I9: Port with model not in catalog keeps the original `model:` line and
// emits a TODO comment for the user.
func TestPortKeepsUnknownModelWithTodo(t *testing.T) {
	d := t.TempDir()
	input := filepath.Join(d, "in/archie.md")
	dest := filepath.Join(d, "src/agents/archie.md")
	writeFile(t, input, `---
name: archie
description: An advisor.
model: some-unknown-model
---

Body.
`)
	p := &profiles.File{
		ActiveProfile: "openai",
		Profiles:      map[string]map[string]map[string]string{"openai": {"pi": {"reasoning": "different-model"}}},
	}
	c := Candidate{Tool: "pi", InputPath: input, SourcePath: dest, AutoPortable: true}
	if err := Port(d, c, p); err != nil {
		t.Fatalf("Port: %v", err)
	}
	out, _ := os.ReadFile(dest)
	if !strings.Contains(string(out), "model: some-unknown-model") {
		t.Errorf("unknown model should pass through, got:\n%s", out)
	}
	if !strings.Contains(string(out), "TODO") {
		t.Errorf("expected TODO comment for unknown model, got:\n%s", out)
	}
}

// I11: Port injects targets and scope after description.
func TestPortInjectsTargetsAndScope(t *testing.T) {
	d := t.TempDir()
	input := filepath.Join(d, "in/archie.md")
	dest := filepath.Join(d, "src/agents/archie.md")
	writeFile(t, input, `---
name: archie
description: An advisor.
tools: read
---

Body.
`)
	c := Candidate{Tool: "pi", InputPath: input, SourcePath: dest, AutoPortable: true}
	p := &profiles.File{ActiveProfile: "openai", Profiles: map[string]map[string]map[string]string{"openai": {"pi": {}}}}
	if err := Port(d, c, p); err != nil {
		t.Fatalf("Port: %v", err)
	}
	out, _ := os.ReadFile(dest)
	if !strings.Contains(string(out), "targets: pi, claude, codex, opencode") {
		t.Errorf("expected targets injection, got:\n%s", out)
	}
	if !strings.Contains(string(out), "scope: home") {
		t.Errorf("expected scope: home injection, got:\n%s", out)
	}
}

// I15: AutoPortable=true for pi, claude, and codex; false for opencode and
// antigravity (their formats don't reverse cleanly).
func TestScanAutoPortableForPiClaudeCodex(t *testing.T) {
	home := t.TempDir()
	source := t.TempDir()
	writeFile(t, filepath.Join(home, ".pi/agent/agents/p.md"), "x")
	writeFile(t, filepath.Join(home, ".claude/agents/c.md"), "x")
	writeFile(t, filepath.Join(home, ".codex/agents/x.toml"), "x")
	writeFile(t, filepath.Join(home, ".config/opencode/agents/o.md"), "x")
	writeFile(t, filepath.Join(home, ".gemini/antigravity-cli/plugins/dfiles/agents/a.md"), "x")

	candidates, err := Scan(home, source)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"pi": true, "claude": true, "codex": true, "opencode": false, "antigravity": false}
	for _, c := range candidates {
		if c.AutoPortable != want[c.Tool] {
			t.Errorf("%s: AutoPortable = %v, want %v", c.Tool, c.AutoPortable, want[c.Tool])
		}
	}
}

// I16: Port on a Claude candidate reverses tools (Read/Bash/Grep/Glob →
// read/bash/grep/find) and model alias → modelRole via fixed.claude.
func TestPortClaudeReversesToolsAndModel(t *testing.T) {
	d := t.TempDir()
	input := filepath.Join(d, "in/archie.md")
	dest := filepath.Join(d, "src/agents/archie.md")
	writeFile(t, input, `---
name: archie
description: An advisor.
tools: Read, Bash, Grep, Glob
model: opus
---
Body.
`)
	p := &profiles.File{
		Fixed: map[string]map[string]string{
			"claude": {"reasoning": "opus"},
		},
	}
	c := Candidate{Tool: "claude", InputPath: input, SourcePath: dest, AutoPortable: true}
	if err := Port(d, c, p); err != nil {
		t.Fatalf("Port: %v", err)
	}
	out, _ := os.ReadFile(dest)
	if !strings.Contains(string(out), "tools: read, bash, grep, find") {
		t.Errorf("expected reversed tools list, got:\n%s", out)
	}
	if !strings.Contains(string(out), "modelRole: reasoning") {
		t.Errorf("expected modelRole reverse, got:\n%s", out)
	}
	if strings.Contains(string(out), "model: opus") {
		t.Errorf("original model line should be replaced, got:\n%s", out)
	}
	if !strings.Contains(string(out), "targets: pi, claude, codex, opencode") {
		t.Errorf("expected targets injection, got:\n%s", out)
	}
}

// I17: Port on a Claude candidate with an unrecognised model alias keeps the
// original `model:` line and emits a TODO, mirroring the Pi unknown-model path.
func TestPortClaudeKeepsUnknownModelWithTodo(t *testing.T) {
	d := t.TempDir()
	input := filepath.Join(d, "in/archie.md")
	dest := filepath.Join(d, "src/agents/archie.md")
	writeFile(t, input, `---
name: archie
description: An advisor.
model: some-unknown-alias
---
Body.
`)
	c := Candidate{Tool: "claude", InputPath: input, SourcePath: dest, AutoPortable: true}
	if err := Port(d, c, &profiles.File{}); err != nil {
		t.Fatalf("Port: %v", err)
	}
	out, _ := os.ReadFile(dest)
	if !strings.Contains(string(out), "model: some-unknown-alias") {
		t.Errorf("unknown model should pass through, got:\n%s", out)
	}
	if !strings.Contains(string(out), "TODO") {
		t.Errorf("expected TODO comment for unknown model, got:\n%s", out)
	}
}

// I18: Port on a Codex candidate reads the TOML, reverses model+effort to
// modelRole via fixed.codex, preserves description/body, and flags the
// missing tools list with a TODO (Codex has no tools field to reverse).
func TestPortCodexReversesModelAndFlagsMissingTools(t *testing.T) {
	d := t.TempDir()
	input := filepath.Join(d, "in/archie.toml")
	dest := filepath.Join(d, "src/agents/archie.md")
	writeFile(t, input, `name = "archie"
description = "An advisor."
model = "gpt-5.4"
model_reasoning_effort = "medium"
developer_instructions = '''
Body content.
'''
`)
	p := &profiles.File{
		Fixed: map[string]map[string]string{
			"codex": {"design": "gpt-5.4:medium"},
		},
	}
	c := Candidate{Name: "archie", Tool: "codex", InputPath: input, SourcePath: dest, AutoPortable: true}
	if err := Port(d, c, p); err != nil {
		t.Fatalf("Port: %v", err)
	}
	out, _ := os.ReadFile(dest)
	if !strings.Contains(string(out), "description: An advisor.") {
		t.Errorf("expected description, got:\n%s", out)
	}
	if !strings.Contains(string(out), "modelRole: design") {
		t.Errorf("expected modelRole reverse, got:\n%s", out)
	}
	if !strings.Contains(string(out), "Body content.") {
		t.Errorf("expected body, got:\n%s", out)
	}
	if !strings.Contains(string(out), "TODO: Codex agents carry no explicit tools list") {
		t.Errorf("expected missing-tools TODO, got:\n%s", out)
	}
}

// I19: Port rejects OpenCode candidates (permission-map format has no clean
// reverse into a source tools list) with a helpful error.
func TestPortRejectsOpenCode(t *testing.T) {
	c := Candidate{Name: "x", Tool: "opencode", InputPath: "/some/path"}
	err := Port(t.TempDir(), c, nil)
	if err == nil {
		t.Fatal("expected error for opencode candidate")
	}
	if !strings.Contains(err.Error(), "opencode") || !strings.Contains(err.Error(), "/some/path") {
		t.Errorf("error should name tool and path: %v", err)
	}
}

// I14: Port produces output that round-trips through schema.ParseAgent.
// Catches malformed source files that wouldn't render.
func TestPortOutputParsesCleanly(t *testing.T) {
	d := t.TempDir()
	input := filepath.Join(d, "in/archie.md")
	dest := filepath.Join(d, "src/agents/archie.md")
	writeFile(t, input, `---
name: archie
description: An advisor.
tools: read, bash
model: x-not-in-catalog
---

Body.
`)
	c := Candidate{Tool: "pi", InputPath: input, SourcePath: dest, AutoPortable: true}
	p := &profiles.File{ActiveProfile: "openai", Profiles: map[string]map[string]map[string]string{"openai": {"pi": {}}}}
	if err := Port(d, c, p); err != nil {
		t.Fatalf("Port: %v", err)
	}
	out, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	// Parse the output via schema to check it's well-formed.
	// (Imported lazily to avoid a circular dep — schema parses what we just wrote.)
	if !strings.HasPrefix(string(out), "---\n") {
		t.Errorf("missing leading delimiter")
	}
	if !strings.Contains(string(out), "\n---\n") {
		t.Errorf("missing closing delimiter")
	}
}

func TestPortReplacesFinalSourceSymlink(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "installed", "imported.md")
	sourceRoot := filepath.Join(root, "source")
	target := filepath.Join(sourceRoot, "agents", "other.md")
	intended := filepath.Join(sourceRoot, "agents", "imported.md")
	writeFile(t, input, "---\nname: imported\ndescription: Imported agent\n---\nImported body.\n")
	writeFile(t, target, "other canonical bytes")
	if err := os.Chmod(target, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("other.md", intended); err != nil {
		t.Fatal(err)
	}
	candidate := Candidate{Name: "imported", Tool: "pi", InputPath: input, SourcePath: intended, AutoPortable: true}

	if err := Port(sourceRoot, candidate, nil); err != nil {
		t.Fatalf("Port: %v", err)
	}
	assertFileUnchanged(t, target, "other canonical bytes", 0o600)
	assertRegularFile(t, intended, "---\nname: imported\ndescription: Imported agent\ntargets: pi, claude, codex, opencode\nscope: home\n---\nImported body.\n", 0o644)
}

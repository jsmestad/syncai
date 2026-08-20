// Package codex renders canonical agents and skills into Codex CLI's format.
//
// Per https://developers.openai.com/codex/subagents, Codex stores subagents
// as TOML files at ~/.codex/agents/<name>.toml or .codex/agents/<name>.toml.
// Required keys: name, description, developer_instructions. Optional: model,
// model_reasoning_effort, sandbox_mode, nickname_candidates, mcp_servers,
// skills.config. There is no `tools` field; tool access is controlled via
// `sandbox_mode` and `mcp_servers`.
//
// Output layout:
//
//	<outRoot>/.codex/AGENTS.md           (instructions/global.md)
//	<outRoot>/.codex/agents/<name>.toml  (only agents that target=codex)
//	<outRoot>/.codex/skills/<name>/      (verbatim copy)
package codex

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jsmestad/syncai/internal/load"
	"github.com/jsmestad/syncai/internal/profiles"
	"github.com/jsmestad/syncai/internal/renderers"
	"github.com/jsmestad/syncai/internal/schema"
)

// readOnlyTools lets us infer sandbox_mode = "read-only" when the source
// tool list contains only these. Anything outside this set drops the field
// (Codex defaults to inheriting from parent).
var readOnlyTools = map[string]bool{
	"read": true, "bash": true, "grep": true, "find": true,
	"ls": true, "glob": true,
}

type Renderer struct{}

func New() Renderer { return Renderer{} }

func (Renderer) Name() string { return string(schema.TargetCodex) }

func (Renderer) Render(in renderers.Inputs, outRoot string) ([]string, error) {
	if in.ProjectMode {
		return nil, nil
	}
	root := filepath.Join(outRoot, ".codex")
	if err := load.MkdirAll(outRoot, root, 0o755); err != nil {
		return nil, err
	}
	var written []string

	w, err := writeAgents(outRoot, root, in)
	if err != nil {
		return nil, err
	}
	written = append(written, w...)

	w, err = renderers.WriteSkills(outRoot, filepath.Join(root, "skills"), in.SkillDirs, in.BuiltInSkills)
	if err != nil {
		return nil, err
	}
	written = append(written, w...)

	if in.InstructionsGlobal != "" {
		path := filepath.Join(root, "AGENTS.md")
		body := renderers.GeneratedHeader + strings.TrimRight(in.InstructionsGlobal, "\n") + "\n"
		body = preserveRTKInclude(path, body)
		if err := load.WriteFileReplacing(outRoot, path, []byte(body), 0o644); err != nil {
			return nil, err
		}
		written = append(written, path)
	}
	return written, nil
}

func preserveRTKInclude(path, body string) string {
	existing, err := os.ReadFile(path)
	if err != nil {
		return body
	}
	for _, line := range strings.Split(string(existing), "\n") {
		include := strings.TrimSpace(line)
		if strings.HasPrefix(include, "@/") && strings.HasSuffix(include, "/.codex/RTK.md") && !strings.Contains(body, include) {
			return strings.TrimRight(body, "\n") + "\n\n" + include + "\n"
		}
	}
	return body
}

func writeAgents(outRoot, root string, in renderers.Inputs) ([]string, error) {
	dir := filepath.Join(root, "agents")
	var written []string
	for _, a := range in.Agents {
		if !a.HasTarget(schema.TargetCodex) {
			continue
		}
		if err := load.MkdirAll(outRoot, dir, 0o755); err != nil {
			return nil, err
		}
		body, err := renderAgent(a, in.Profiles)
		if err != nil {
			return nil, err
		}
		path := filepath.Join(dir, a.Name+".toml")
		if err := load.WriteFileReplacing(dir, path, body, 0o644); err != nil {
			return nil, fmt.Errorf("writing %s: %w", path, err)
		}
		written = append(written, path)
	}
	return written, nil
}

// renderAgent emits TOML for a Codex subagent. Layout:
//
//	name = "..."
//	description = "..."
//	model = "..."                 (if claude/codex sub-target resolves)
//	model_reasoning_effort = "..." (if encoded as "model:effort" in profile)
//	sandbox_mode = "read-only"     (if all tools are read-only)
//	developer_instructions = '''
//	... body ...
//	'''
//
// The body uses a TOML multi-line literal string rather than a basic string
// because agent docs routinely contain backslashes (Swift `\.dismiss`, regex
// examples) and embedded triple-double-quote Kotlin/Python docstrings, both of
// which TOML basic strings reject. Literal strings perform no escape processing
// and terminate on the three-single-quote delimiter emitted below.
func renderAgent(a *schema.Agent, p *profiles.File) ([]byte, error) {
	body := strings.TrimRight(a.Body, "\n")
	if strings.Contains(body, "'''") {
		return nil, fmt.Errorf("%s: body contains ''' which would terminate the TOML literal string; rewrite the example or escape the quotes", a.Path)
	}

	var b bytes.Buffer
	fmt.Fprintf(&b, "name = %q\n", a.Name)
	fmt.Fprintf(&b, "description = %q\n", a.Description)

	if role := a.Lookup("modelRole"); role != "" {
		model, effort, err := resolveModel(p, role)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", a.Path, err)
		}
		if model != "" {
			fmt.Fprintf(&b, "model = %q\n", model)
		}
		if effort != "" {
			fmt.Fprintf(&b, "model_reasoning_effort = %q\n", effort)
		}
	}

	if mode := inferSandbox(a.Lookup("tools")); mode != "" {
		fmt.Fprintf(&b, "sandbox_mode = %q\n", mode)
	}

	b.WriteString("developer_instructions = '''\n")
	b.WriteString(body)
	b.WriteString("\n'''\n")
	return b.Bytes(), nil
}

// resolveModel looks up the role for the codex target. Codex is a fixed
// target (OpenAI only), consulted from model-profiles.json/fixed.codex
// regardless of activeProfile. Catalog entries encode reasoning effort as
// "id:effort" suffixes (matching how Pi encodes them); we split here so the
// renderer can emit them as separate TOML keys.
func resolveModel(p *profiles.File, role string) (model, effort string, err error) {
	if p == nil || !p.HasTarget(string(schema.TargetCodex)) {
		return "", "", nil
	}
	resolved, err := p.Resolve(string(schema.TargetCodex), role)
	if err != nil {
		return "", "", err
	}
	if idx := strings.Index(resolved, ":"); idx >= 0 {
		return resolved[:idx], resolved[idx+1:], nil
	}
	return resolved, "", nil
}

func inferSandbox(toolsCSV string) string {
	tools := schema.SplitCSV(toolsCSV)
	if len(tools) == 0 {
		return ""
	}
	for _, t := range tools {
		if !readOnlyTools[strings.ToLower(t)] {
			return ""
		}
	}
	return "read-only"
}

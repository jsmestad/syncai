// Package antigravity renders canonical agents and skills into an Antigravity CLI plugin.
// Layout: <outRoot>/.gemini/antigravity-cli/plugins/dfiles/{agents,skills}/...
// Frontmatter: YAML-safe name, description, tools array, model. Body is verbatim.
package antigravity

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/jsmestad/syncai/internal/load"
	"github.com/jsmestad/syncai/internal/profiles"
	"github.com/jsmestad/syncai/internal/renderers"
	"github.com/jsmestad/syncai/internal/schema"
)

type Renderer struct{}

func New() Renderer { return Renderer{} }

func (Renderer) Name() string { return string(schema.TargetAntigravity) }

func (Renderer) Render(in renderers.Inputs, outRoot string) ([]string, error) {
	outDir := filepath.Join(outRoot, ".gemini", "antigravity-cli", "plugins", "dfiles", "agents")
	if err := load.MkdirAll(outRoot, outDir, 0o755); err != nil {
		return nil, err
	}
	var written []string
	for _, a := range in.Agents {
		if !a.HasTarget(schema.TargetAntigravity) {
			continue
		}
		out, err := renderAgent(a, in.Profiles)
		if err != nil {
			return nil, err
		}
		path := filepath.Join(outDir, a.Name+".md")
		if err := load.WriteFileReplacing(outDir, path, out, 0o644); err != nil {
			return nil, fmt.Errorf("writing %s: %w", path, err)
		}
		written = append(written, path)
	}
	if len(in.SkillDirs) > 0 || len(in.BuiltInSkills) > 0 {
		skillDir := filepath.Join(outRoot, ".gemini", "antigravity-cli", "plugins", "dfiles", "skills")
		paths, err := renderers.WriteSkills(outRoot, skillDir, in.SkillDirs, in.BuiltInSkills)
		if err != nil {
			return nil, err
		}
		written = append(written, paths...)
	}
	return written, nil
}

func renderAgent(a *schema.Agent, p *profiles.File) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString("---\n")
	for _, kv := range a.Fields {
		switch kv.Key {
		case "targets", "scope":
			continue
		case "modelRole":
			model, err := p.Resolve(string(schema.TargetAntigravity), kv.Value)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", a.Path, err)
			}
			fmt.Fprintf(&b, "model: %s\n", model)
			continue
		case "model", "fallbackModels":
			continue
		}
		if _, ok := schema.PiOnlyFields[kv.Key]; ok {
			continue
		}
		if kv.Key == "tools" {
			writeTools(&b, kv.Value)
			continue
		}
		writeScalar(&b, kv.Key, kv.Value)
	}
	b.WriteString("---\n")
	b.WriteString(a.Body)
	return b.Bytes(), nil
}

func writeScalar(b *bytes.Buffer, key, value string) {
	fmt.Fprintf(b, "%s: %s\n", key, yamlString(value))
}

func writeTools(b *bytes.Buffer, value string) {
	tools := antigravityTools(value)
	if len(tools) == 0 {
		return
	}
	b.WriteString("tools:\n")
	for _, tool := range tools {
		fmt.Fprintf(b, "  - %s\n", yamlString(tool))
	}
}

func antigravityTools(value string) []string {
	mapped := map[string]string{
		"bash":  "run_shell_command",
		"edit":  "replace",
		"find":  "glob",
		"grep":  "grep_search",
		"ls":    "list_directory",
		"read":  "read_file",
		"write": "write_file",
	}
	seen := map[string]struct{}{}
	var out []string
	for _, raw := range schema.SplitCSV(value) {
		tool := raw
		if m, ok := mapped[raw]; ok {
			tool = m
		}
		if _, ok := seen[tool]; ok {
			continue
		}
		seen[tool] = struct{}{}
		out = append(out, tool)
	}
	sort.Strings(out)
	return out
}

func yamlString(value string) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return `""`
	}
	return string(raw)
}

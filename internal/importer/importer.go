// Package importer scans installed agent directories and finds files that
// aren't represented in the canonical ai-source/agents/ tree, so the user
// can pull a hand-rolled or UI-created agent into the syncai-managed set.
//
// Pi, Claude, and Codex auto-port: their on-disk formats carry enough
// information (tools list or sandbox_mode, model alias, description, body)
// to reverse-translate into source frontmatter, reusing the same reverse
// mappings package pull applies to drifted files. OpenCode and Antigravity
// are detected but require manual port — OpenCode replaces tools with a
// permission map that has no clean reverse, and Antigravity candidates
// aren't scanned for at all today.
package importer

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jsmestad/syncai/internal/load"
	"github.com/jsmestad/syncai/internal/profiles"
	"github.com/jsmestad/syncai/internal/pull"
	"github.com/jsmestad/syncai/internal/schema"
)

// Candidate is one installed agent file that has no equivalent in source.
type Candidate struct {
	Name         string // "archie"
	Tool         string // "pi", "omp", "claude", "codex", "opencode", "antigravity"
	InputPath    string // /Users/x/.claude/agents/archie.md
	SourcePath   string // /Users/x/code/.../ai-source/agents/archie.md
	AutoPortable bool   // true if Tool format converts cleanly (Pi only for now)
}

// ScanRoots returns the list of (tool, dir) pairs to scan, expanded to the
// absolute paths under the user's home directory. Missing dirs are skipped.
func ScanRoots(homeDir string) []struct{ Tool, Dir string } {
	return []struct{ Tool, Dir string }{
		{"pi", filepath.Join(homeDir, ".pi", "agent", "agents")},
		{"omp", filepath.Join(homeDir, ".omp", "agent", "agents")},
		{"claude", filepath.Join(homeDir, ".claude", "agents")},
		{"codex", filepath.Join(homeDir, ".codex", "agents")},
		{"opencode", filepath.Join(homeDir, ".config", "opencode", "agents")},
		{"antigravity", filepath.Join(homeDir, ".gemini", "antigravity-cli", "plugins", "dfiles", "agents")},
	}
}

// Scan walks every installed agent dir, compares each file's name against
// what's already in <sourceRoot>/agents/, and returns the orphans.
func Scan(homeDir, sourceRoot string) ([]Candidate, error) {
	known, err := nameSet(filepath.Join(sourceRoot, "agents"))
	if err != nil {
		return nil, err
	}
	var out []Candidate
	for _, root := range ScanRoots(homeDir) {
		entries, err := os.ReadDir(root.Dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("reading %s: %w", root.Dir, err)
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := stripExt(e.Name())
			if name == "" || known[name] {
				continue
			}
			out = append(out, Candidate{
				Name:         name,
				Tool:         root.Tool,
				InputPath:    filepath.Join(root.Dir, e.Name()),
				SourcePath:   filepath.Join(sourceRoot, "agents", name+".md"),
				AutoPortable: root.Tool == "pi" || root.Tool == "claude" || root.Tool == "codex",
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Tool < out[j].Tool
	})
	return out, nil
}

// Port reads the installed file, transforms it to source frontmatter (adds
// targets/scope, reverses tool-specific vocabulary back to source terms when
// the reverse mapping is unambiguous), and writes it to SourcePath.
// Supports Pi, Claude, and Codex; other tools' candidates get an error
// directing the user to port manually.
func Port(sourceRoot string, c Candidate, p *profiles.File) error {
	switch c.Tool {
	case "pi", "claude", "codex":
	default:
		return fmt.Errorf("auto-import doesn't support %s; requires manual port: see %s", c.Tool, c.InputPath)
	}
	raw, err := os.ReadFile(c.InputPath)
	if err != nil {
		return err
	}
	var out []byte
	switch c.Tool {
	case "pi":
		a, err := schema.ParseAgent(c.InputPath, raw)
		if err != nil {
			return err
		}
		out, err = piToSource(a, p)
		if err != nil {
			return err
		}
	case "claude":
		a, err := schema.ParseAgent(c.InputPath, raw)
		if err != nil {
			return err
		}
		out, err = claudeToSource(a, p)
		if err != nil {
			return err
		}
	case "codex":
		out, err = codexToSource(c.Name, raw, p)
		if err != nil {
			return err
		}
	}
	return load.WriteFileReplacing(sourceRoot, c.SourcePath, out, 0o644)
}

// piToSource synthesises a source-format markdown file from a Pi-installed
// agent. Most fields pass through; we add targets and scope, and reverse
// the model/fallbackModels lookup back to roles when possible.
func piToSource(a *schema.Agent, p *profiles.File) ([]byte, error) {
	var b strings.Builder
	b.WriteString("---\n")
	wroteTargets := false
	wroteScope := false
	for _, kv := range a.Fields {
		switch kv.Key {
		case "model":
			if role, ok := pull.ReverseModel("pi", kv.Value, p); ok {
				fmt.Fprintf(&b, "modelRole: %s\n", role)
			} else {
				// No clean reverse — pass through verbatim and note via a
				// separate modelRole-required-todo line so users see it.
				fmt.Fprintf(&b, "model: %s\n", kv.Value)
				b.WriteString("# TODO: replace `model:` with `modelRole:` for cross-tool support\n")
			}
		case "fallbackModels":
			roles := reversePiModels(kv.Value, p)
			if len(roles) > 0 {
				fmt.Fprintf(&b, "fallbackRoles: %s\n", strings.Join(roles, ", "))
			} else {
				fmt.Fprintf(&b, "fallbackModels: %s\n", kv.Value)
			}
		case "description":
			b.WriteString(kv.Key + ": " + kv.Value + "\n")
			if !wroteTargets {
				b.WriteString("targets: pi, claude, codex, opencode\n")
				wroteTargets = true
			}
			if !wroteScope {
				b.WriteString("scope: home\n")
				wroteScope = true
			}
		default:
			fmt.Fprintf(&b, "%s: %s\n", kv.Key, kv.Value)
		}
	}
	if !wroteTargets {
		b.WriteString("targets: pi, claude, codex, opencode\n")
	}
	if !wroteScope {
		b.WriteString("scope: home\n")
	}
	b.WriteString("---\n")
	b.WriteString(a.Body)
	return []byte(b.String()), nil
}

// claudeToSource synthesises a source-format markdown file from a
// Claude-installed agent. Reuses the same tool-list and model reverse
// mappings package pull applies when pulling drifted Claude agents back into
// source (Read/Bash/Grep/Glob → lowercase, model alias → modelRole via
// fixed.claude).
func claudeToSource(a *schema.Agent, p *profiles.File) ([]byte, error) {
	var b strings.Builder
	b.WriteString("---\n")
	wroteTargets := false
	wroteScope := false
	for _, kv := range a.Fields {
		switch kv.Key {
		case "tools":
			if reversed, ok := pull.ReverseToolList("claude", kv.Value); ok {
				fmt.Fprintf(&b, "tools: %s\n", reversed)
			} else {
				fmt.Fprintf(&b, "tools: %s\n", kv.Value)
			}
		case "model":
			if role, ok := pull.ReverseModel("claude", kv.Value, p); ok {
				fmt.Fprintf(&b, "modelRole: %s\n", role)
			} else {
				fmt.Fprintf(&b, "model: %s\n", kv.Value)
				b.WriteString("# TODO: replace `model:` with `modelRole:` for cross-tool support\n")
			}
		case "description":
			b.WriteString(kv.Key + ": " + kv.Value + "\n")
			if !wroteTargets {
				b.WriteString("targets: pi, claude, codex, opencode\n")
				wroteTargets = true
			}
			if !wroteScope {
				b.WriteString("scope: home\n")
				wroteScope = true
			}
		default:
			fmt.Fprintf(&b, "%s: %s\n", kv.Key, kv.Value)
		}
	}
	if !wroteTargets {
		b.WriteString("targets: pi, claude, codex, opencode\n")
	}
	if !wroteScope {
		b.WriteString("scope: home\n")
	}
	b.WriteString("---\n")
	b.WriteString(a.Body)
	return []byte(b.String()), nil
}

// codexToSource synthesises a source-format markdown file from a
// Codex-installed agent (.toml). Codex has no tools list — sandbox_mode is
// derived from source `tools` at render time, not the reverse — so ported
// files carry a TODO prompting the user to add one explicitly. Model+effort
// reverse to modelRole via fixed.codex when the catalog lookup is
// unambiguous, same as pull does for drifted Codex agents.
func codexToSource(name string, raw []byte, p *profiles.File) ([]byte, error) {
	description, model, effort, body, err := pull.ParseCodexAgent(raw)
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "name: %s\n", name)
	fmt.Fprintf(&b, "description: %s\n", description)
	b.WriteString("targets: pi, claude, codex, opencode\n")
	b.WriteString("scope: home\n")
	b.WriteString("# TODO: Codex agents carry no explicit tools list (sandbox_mode only) — add tools: if this agent should be restricted\n")
	if model != "" {
		encoded := model
		if effort != "" {
			encoded = model + ":" + effort
		}
		if role, ok := pull.ReverseModel("codex", encoded, p); ok {
			fmt.Fprintf(&b, "modelRole: %s\n", role)
		} else {
			fmt.Fprintf(&b, "model: %s\n", encoded)
			b.WriteString("# TODO: replace `model:` with `modelRole:` for cross-tool support\n")
		}
	}
	b.WriteString("---\n")
	if body != "" {
		b.WriteString(body)
		b.WriteString("\n")
	}
	return []byte(b.String()), nil
}

func reversePiModels(csv string, p *profiles.File) []string {
	parts := schema.SplitCSV(csv)
	out := make([]string, 0, len(parts))
	for _, model := range parts {
		role, ok := pull.ReverseModel("pi", model, p)
		if !ok {
			return nil // any miss aborts — caller falls back to keeping fallbackModels verbatim
		}
		out = append(out, role)
	}
	return out
}

// nameSet returns the basenames (without .md) of every regular file in dir.
func nameSet(dir string) (map[string]bool, error) {
	out := map[string]bool{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := stripExt(e.Name())
		if name != "" {
			out[name] = true
		}
	}
	return out, nil
}

func stripExt(name string) string {
	for _, suffix := range []string{".md", ".toml"} {
		if strings.HasSuffix(name, suffix) {
			return strings.TrimSuffix(name, suffix)
		}
	}
	return ""
}

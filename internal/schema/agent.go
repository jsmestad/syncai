package schema

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
)

// Target identifies a downstream AI tool.
type Target string

const (
	TargetPi          Target = "pi"
	TargetOMP         Target = "omp"
	TargetClaude      Target = "claude"
	TargetCodex       Target = "codex"
	TargetOpenCode    Target = "opencode"
	TargetAntigravity Target = "antigravity"
)

// PiOnlyFields are agent fields the Pi renderer keeps but other targets drop.
// Mirrors PI_ONLY_AGENT_FIELDS in the legacy scripts/sync-ai-config.py.
var PiOnlyFields = map[string]struct{}{
	"fallbackRoles":         {},
	"fallbackModels":        {},
	"systemPromptMode":      {},
	"inheritProjectContext": {},
	"inheritSkills":         {},
	"output":                {},
	"defaultReads":          {},
	"defaultProgress":       {},
	"extensions":            {},
	"maxSubagentDepth":      {},
	"ompSpawns":             {},
}

// KV is one frontmatter line, preserving source order.
type KV struct{ Key, Value string }

// Agent is the canonical shape for a definition in ai-source/agents/.
// Fields holds the raw line-by-line frontmatter in source order so renderers
// that need to preserve order (Pi) can iterate it directly. The typed
// accessors are derived from those fields and exist for type-safe lookups.
type Agent struct {
	Name        string
	Description string
	Targets     []string

	// Scope controls which dotfiles install profiles install the agent.
	// nil/empty = universal (installs everywhere). A list like
	// ["home", "work"] installs only under those profiles. Driven by
	// `make ai-sync PROFILE=<home|work>` via syncai's --scope flag, and
	// matched against the agent's `scope:` frontmatter field which accepts
	// CSV (e.g. `scope: home, work`) for backward and forward compat with
	// future profile names.
	Scope []string

	// Fields is the raw frontmatter in source order. Renderers that care
	// about order iterate this; renderers that pick a fixed layout consume
	// the typed fields above.
	Fields []KV

	// Body is the markdown content after the closing `---`.
	Body string

	// Path is the source file the agent came from. Useful for error messages.
	Path string
}

// Validate enforces required fields and known target values.
// Targets default to [pi] when the source omits the field, matching the
// legacy Python renderer's behaviour.
func (a *Agent) Validate() error {
	if a.Name == "" {
		return fmt.Errorf("%s: name is required", a.Path)
	}
	if a.Name == "." || a.Name == ".." || filepath.IsAbs(a.Name) || strings.ContainsAny(a.Name, `/\`) {
		return fmt.Errorf("%s: name %q must be one safe filename component", a.Path, a.Name)
	}
	if a.Description == "" {
		return fmt.Errorf("%s: description is required", a.Path)
	}
	for _, t := range a.Targets {
		switch Target(t) {
		case TargetPi, TargetOMP, TargetAntigravity, TargetClaude, TargetCodex, TargetOpenCode:
		default:
			return fmt.Errorf("%s: unknown target %q", a.Path, t)
		}
	}
	for _, s := range a.Scope {
		if !ValidScope(s) {
			return fmt.Errorf("%s: unknown scope %q (must be \"home\" or \"work\")", a.Path, s)
		}
	}
	return nil
}

// KnownScopes is the closed set of profile names syncai understands today.
// Sources outside this set are rejected at parse time so typos can't
// silently disable an artifact (e.g. `scope: wrok` rendering nowhere).
var KnownScopes = []string{"home", "work"}

// ValidScope reports whether s is one of KnownScopes.
func ValidScope(s string) bool {
	for _, k := range KnownScopes {
		if k == s {
			return true
		}
	}
	return false
}

// HasTarget reports whether the agent should render to t.
func (a *Agent) HasTarget(t Target) bool {
	for _, raw := range a.Targets {
		if Target(raw) == t {
			return true
		}
	}
	return false
}

// MatchesScope reports whether the agent applies to the requested install
// scope ("home" or "work"). An empty filter matches everything (no filter).
// An empty/nil agent scope is universal and matches every filter.
func (a *Agent) MatchesScope(filter string) bool {
	if filter == "" || len(a.Scope) == 0 {
		return true
	}
	for _, s := range a.Scope {
		if s == filter {
			return true
		}
	}
	return false
}

// ScopeString returns the comma-joined scope list for display.
// Empty/nil scopes render as "" (callers can substitute "universal").
func (a *Agent) ScopeString() string {
	return strings.Join(a.Scope, ", ")
}

// Lookup returns the raw value for the named frontmatter field, or "" if absent.
func (a *Agent) Lookup(key string) string {
	for _, kv := range a.Fields {
		if kv.Key == key {
			return kv.Value
		}
	}
	return ""
}

// ParseAgent reads a markdown file with `---` frontmatter and returns an Agent.
// Frontmatter is parsed line-by-line (key: value), not as YAML, to match the
// legacy Python parser. Values are stored verbatim as strings.
func ParseAgent(path string, raw []byte) (*Agent, error) {
	front, body, err := splitFrontmatter(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	a := &Agent{Path: path, Body: body}
	for _, line := range strings.Split(front, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])
		a.Fields = append(a.Fields, KV{Key: key, Value: value})
		switch key {
		case "name":
			a.Name = value
		case "description":
			a.Description = value
		case "targets":
			a.Targets = splitCSV(value)
		case "scope":
			a.Scope = splitCSV(value)
		}
	}
	if len(a.Targets) == 0 {
		a.Targets = []string{string(TargetPi)}
	}
	if err := a.Validate(); err != nil {
		return nil, err
	}
	return a, nil
}

func splitFrontmatter(raw []byte) (front, body string, err error) {
	const sep = "---\n"
	if !bytes.HasPrefix(raw, []byte(sep)) {
		return "", "", fmt.Errorf("missing leading frontmatter delimiter")
	}
	rest := raw[len(sep):]
	end := bytes.Index(rest, []byte("\n"+sep))
	if end < 0 {
		return "", "", fmt.Errorf("missing closing frontmatter delimiter")
	}
	return string(rest[:end]), string(rest[end+len("\n"+sep):]), nil
}

// SplitCSV is exported for renderers that need to split a CSV value.
func SplitCSV(s string) []string { return splitCSV(s) }

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

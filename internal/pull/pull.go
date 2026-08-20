// Package pull propagates locally-edited installed files back into the
// canonical ai-source/ tree. The common case is a Claude Code session where
// the user revises a subagent's system prompt — pull copies the new body
// back into the source agent so the next render distributes it to every
// other tool.
//
// Body and description edits propagate cleanly (verbatim across formats).
// Tools-list and model edits are reverse-translated per-tool: Claude's
// `tools: Read, Bash, Glob` reverses to source `read, bash, find`; Claude's
// `model: opus` reverses through the fixed.claude profile to a modelRole
// like `reasoning`. OpenCode permission maps and Codex TOML changes still
// require manual edit (their forward conversion loses information).
package pull

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jsmestad/syncai/internal/load"
	"github.com/jsmestad/syncai/internal/pathguard"
	"github.com/jsmestad/syncai/internal/profiles"
	"github.com/jsmestad/syncai/internal/schema"
)

// Plan describes a single pull operation. PlanFor computes a Plan per
// drifted file; Apply rewrites the source file when the change is supported.
type Plan struct {
	Name          string
	Tool          string
	InstalledPath string
	FreshPath     string
	SourcePath    string

	NewDescription string // non-empty when description changed
	NewBody        string // non-empty when body changed
	NewTools       string // non-empty when tools list changed (source-format CSV)
	NewModelRole   string // non-empty when modelRole reverse-resolved from a model alias

	UnsupportedFrontmatter bool // true when a frontmatter field changed that we can't auto-port
}

// HasChanges reports whether Apply would write anything.
func (p Plan) HasChanges() bool {
	return p.NewDescription != "" || p.NewBody != "" || p.NewTools != "" || p.NewModelRole != ""
}

// Apply rewrites the source file with the pulled changes. Other source
// frontmatter (targets, scope, Pi-only fields, body when unchanged) is
// preserved.
func (p Plan) Apply(sourceRoot string) error {
	sourcePath, err := pathguard.Resolve(sourceRoot, p.SourcePath)
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(sourcePath)
	if err != nil {
		return err
	}
	frontMatter, body, err := splitFrontmatter(raw)
	if err != nil {
		return err
	}
	if p.NewDescription != "" {
		frontMatter = replaceField(frontMatter, "description", p.NewDescription)
	}
	if p.NewTools != "" {
		frontMatter = replaceField(frontMatter, "tools", p.NewTools)
	}
	if p.NewModelRole != "" {
		frontMatter = replaceField(frontMatter, "modelRole", p.NewModelRole)
	}
	out := []byte("---\n")
	out = append(out, frontMatter...)
	out = append(out, []byte("---\n")...)
	if p.NewBody != "" {
		out = append(out, []byte(p.NewBody)...)
	} else {
		out = append(out, []byte(body)...)
	}
	return load.WriteFileReplacing(sourceRoot, p.SourcePath, out, 0o644)
}

// PlanFor compares the installed file against the freshly-rendered file
// (both in the tool's native format) to decide what change the user made
// and how to project it back into source.
func PlanFor(installedPath, freshPath, sourcePath, tool string, p *profiles.File) (Plan, error) {
	plan := Plan{
		Name:          stripExt(filepath.Base(installedPath)),
		Tool:          tool,
		InstalledPath: installedPath,
		FreshPath:     freshPath,
		SourcePath:    sourcePath,
	}
	switch tool {
	case "pi", "omp", "claude", "antigravity":
		return planMarkdown(plan, p)
	case "opencode":
		// OpenCode replaces tools with a permission map. Body and
		// description still propagate; permission-map → tools-list reverse
		// is left manual.
		return planMarkdown(plan, p)
	case "codex":
		return planCodex(plan, p)
	default:
		plan.UnsupportedFrontmatter = true
		return plan, nil
	}
}

// planMarkdown is the common Pi/Claude/Antigravity/OpenCode path. The renderer
// for each tool emits markdown with frontmatter, so we can split installed
// vs fresh and compare line-by-line.
func planMarkdown(plan Plan, p *profiles.File) (Plan, error) {
	installRaw, err := os.ReadFile(plan.InstalledPath)
	if err != nil {
		return plan, err
	}
	installFM, installBody, err := splitFrontmatter(installRaw)
	if err != nil {
		return plan, fmt.Errorf("%s: %w", plan.InstalledPath, err)
	}
	freshRaw, err := os.ReadFile(plan.FreshPath)
	if err != nil {
		return plan, err
	}
	freshFM, freshBody, err := splitFrontmatter(freshRaw)
	if err != nil {
		return plan, fmt.Errorf("%s: %w", plan.FreshPath, err)
	}

	if installBody != freshBody {
		plan.NewBody = installBody
	}

	// Track per-field equality so we can decide UnsupportedFrontmatter
	// based on what's STILL different after computing reversals.
	installDesc := fieldValue(installFM, "description")
	freshDesc := fieldValue(freshFM, "description")
	descChanged := installDesc != freshDesc && installDesc != ""
	if descChanged {
		plan.NewDescription = installDesc
	}

	installTools := fieldValue(installFM, "tools")
	freshTools := fieldValue(freshFM, "tools")
	toolsChanged := installTools != freshTools && installTools != ""
	toolsResolved := false
	if toolsChanged {
		if reversed, ok := reverseToolList(plan.Tool, installTools); ok {
			plan.NewTools = reversed
			toolsResolved = true
		}
	}

	installModel := fieldValue(installFM, "model")
	freshModel := fieldValue(freshFM, "model")
	installThinking := fieldValue(installFM, "thinkingLevel")
	freshThinking := fieldValue(freshFM, "thinkingLevel")
	thinkingChanged := plan.Tool == "omp" && installThinking != freshThinking
	modelChanged := (installModel != freshModel || thinkingChanged) && installModel != ""
	modelResolved := false
	if modelChanged && p != nil {
		modelForReverse := installModel
		if plan.Tool == "omp" && installThinking != "" {
			modelForReverse += ":" + installThinking
		}
		if role, ok := reverseModel(plan.Tool, modelForReverse, p); ok {
			plan.NewModelRole = role
			modelResolved = true
		}
	}

	// Compute "remaining" by stripping every line we know we handled.
	// If the rest still differs, surface UnsupportedFrontmatter.
	stripped := func(fm []byte) string {
		drop := []string{"description"}
		if toolsChanged && toolsResolved {
			drop = append(drop, "tools")
		}
		if modelChanged && modelResolved {
			drop = append(drop, "model")
			if plan.Tool == "omp" {
				drop = append(drop, "thinkingLevel")
			}
		}
		return stripLines(fm, drop)
	}
	if stripped(installFM) != stripped(freshFM) {
		plan.UnsupportedFrontmatter = true
	}
	if (toolsChanged && !toolsResolved) || (modelChanged && !modelResolved) {
		plan.UnsupportedFrontmatter = true
	}
	return plan, nil
}

// ReverseToolList exposes reverseToolList to callers outside package pull.
// The importer package reuses this to auto-port previously-untracked
// Claude-origin agents, the same reverse translation pull already does for
// drifted ones.
func ReverseToolList(tool, csv string) (string, bool) { return reverseToolList(tool, csv) }

// ReverseModel exposes reverseModel to callers outside package pull, for the
// same reason as ReverseToolList.
func ReverseModel(tool, model string, p *profiles.File) (string, bool) {
	return reverseModel(tool, model, p)
}

// reverseToolList maps a tool's installed-format tool list back to source
// vocabulary. Returns ok=false when the tool's reverse isn't supported
// (e.g. OpenCode uses a permission map, not a list).
func reverseToolList(tool, csv string) (string, bool) {
	switch tool {
	case "pi", "antigravity":
		// These tools render the source tools list verbatim. Already source format.
		return csv, true
	case "omp":
		out := make([]string, 0)
		seen := map[string]bool{}
		for _, tool := range schema.SplitCSV(csv) {
			mapped := strings.ToLower(tool)
			if mapped == "glob" {
				mapped = "find"
			}
			if !seen[mapped] {
				seen[mapped] = true
				out = append(out, mapped)
			}
		}
		return strings.Join(out, ", "), true
	case "claude":
		out := make([]string, 0)
		seen := map[string]bool{}
		for _, t := range schema.SplitCSV(csv) {
			lower := strings.ToLower(t)
			// Claude's Glob covers both find and ls in source; we pick "find"
			// because it's the broader "list/search files" semantic. Users
			// who specifically need ls in source can edit by hand.
			if lower == "glob" {
				lower = "find"
			}
			if !seen[lower] {
				seen[lower] = true
				out = append(out, lower)
			}
		}
		return strings.Join(out, ", "), true
	case "opencode":
		// OpenCode replaces tools with a permission map; reverse from a map
		// back to a list is lossy enough to skip in v1.
		return "", false
	default:
		return "", false
	}
}

// reverseModel maps a tool's installed model identifier back to a source
// modelRole by inverting the matching profile catalog table. Pi uses the
// active profile's pi sub-target; Claude uses fixed.claude; etc.
func reverseModel(tool, model string, p *profiles.File) (string, bool) {
	candidates := map[string]map[string]string{}
	if p != nil {
		if m, ok := p.Fixed[tool]; ok {
			candidates[tool] = m
		}
		if prof, ok := p.Profiles[p.ActiveProfile]; ok {
			if m, ok := prof[tool]; ok {
				candidates[tool] = m
			}
		}
	}
	target, ok := candidates[tool]
	if !ok {
		return "", false
	}
	for role, id := range target {
		if id == model {
			return role, true
		}
	}
	return "", false
}

func splitFrontmatter(raw []byte) (front []byte, body string, err error) {
	const sep = "---\n"
	if !bytes.HasPrefix(raw, []byte(sep)) {
		return nil, "", fmt.Errorf("missing leading frontmatter delimiter")
	}
	rest := raw[len(sep):]
	end := bytes.Index(rest, []byte("\n"+sep))
	if end < 0 {
		return nil, "", fmt.Errorf("missing closing frontmatter delimiter")
	}
	return rest[:end+1], string(rest[end+len("\n"+sep):]), nil
}

func fieldValue(frontMatter []byte, key string) string {
	for _, line := range strings.Split(string(frontMatter), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		if strings.TrimSpace(line[:idx]) == key {
			return strings.TrimSpace(line[idx+1:])
		}
	}
	return ""
}

func replaceField(frontMatter []byte, key, value string) []byte {
	lines := strings.Split(string(frontMatter), "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		if strings.TrimSpace(line[:idx]) == key {
			lines[i] = key + ": " + value
			return []byte(strings.Join(lines, "\n"))
		}
	}
	return append(frontMatter, []byte(key+": "+value+"\n")...)
}

func stripLines(frontMatter []byte, dropKeys []string) string {
	drop := map[string]bool{}
	for _, k := range dropKeys {
		drop[k] = true
	}
	var keep []string
	for _, line := range strings.Split(string(frontMatter), "\n") {
		if strings.TrimSpace(line) == "" {
			keep = append(keep, line)
			continue
		}
		idx := strings.Index(line, ":")
		if idx < 0 {
			keep = append(keep, line)
			continue
		}
		if drop[strings.TrimSpace(line[:idx])] {
			continue
		}
		keep = append(keep, line)
	}
	return strings.Join(keep, "\n")
}

func stripExt(name string) string {
	for _, suffix := range []string{".md", ".toml"} {
		if strings.HasSuffix(name, suffix) {
			return strings.TrimSuffix(name, suffix)
		}
	}
	return name
}

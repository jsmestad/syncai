package pull

import (
	"fmt"
	"os"
	"strings"

	"github.com/jsmestad/syncai/internal/profiles"
)

// codexFields is the subset of Codex TOML we read for pull. Codex agents
// declare more (sandbox_mode, mcp_servers, nickname_candidates) but pull
// only needs the bits that map back to source frontmatter.
type codexFields struct {
	Description           string
	Model                 string
	ModelReasoningEffort  string
	DeveloperInstructions string
}

// ParseCodexAgent exposes the fields parseCodexTOML extracts to callers
// outside package pull. The importer package reuses this to auto-port
// previously-untracked Codex-origin agents into ai-source markdown.
func ParseCodexAgent(raw []byte) (description, model, modelReasoningEffort, developerInstructions string, err error) {
	f, err := parseCodexTOML(raw)
	if err != nil {
		return "", "", "", "", err
	}
	return f.Description, f.Model, f.ModelReasoningEffort, f.DeveloperInstructions, nil
}

// parseCodexTOML extracts the fields we care about from a Codex agent file.
// This is not a full TOML parser — it handles the shape syncai itself
// emits: top-level scalar lines and a triple-quoted developer_instructions
// block. The renderer emits literal-string blocks (”'...”'); we also
// accept basic-string blocks ("""...""") so already-installed files from
// older renders still parse. Tables and arrays are ignored.
func parseCodexTOML(raw []byte) (codexFields, error) {
	var out codexFields
	lines := strings.Split(string(raw), "\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		eq := strings.Index(trimmed, "=")
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(trimmed[:eq])
		rest := strings.TrimSpace(trimmed[eq+1:])

		// Triple-quoted block: read until the matching delimiter. Match the
		// opening delimiter so a body containing `"""` doesn't accidentally
		// terminate a `'''` block (or vice versa).
		if rest == "'''" || rest == "\"\"\"" {
			delim := rest
			var bodyLines []string
			i++
			for ; i < len(lines); i++ {
				if strings.TrimSpace(lines[i]) == delim {
					break
				}
				bodyLines = append(bodyLines, lines[i])
			}
			value := strings.Join(bodyLines, "\n")
			if key == "developer_instructions" {
				out.DeveloperInstructions = value
			}
			continue
		}

		// Scalar string: surrounded by double quotes; unescape inner quotes.
		if strings.HasPrefix(rest, "\"") && strings.HasSuffix(rest, "\"") && len(rest) >= 2 {
			value := strings.ReplaceAll(rest[1:len(rest)-1], `\"`, `"`)
			switch key {
			case "description":
				out.Description = value
			case "model":
				out.Model = value
			case "model_reasoning_effort":
				out.ModelReasoningEffort = value
			}
		}
	}
	return out, nil
}

// planCodex compares an installed Codex .toml file against the freshly-
// rendered baseline and produces a Plan. Body and description propagate;
// model+effort changes reverse to modelRole via fixed.codex; sandbox_mode
// and other Codex-only fields are ignored (they're auto-derived from
// source `tools` at render time).
func planCodex(plan Plan, p *profiles.File) (Plan, error) {
	installRaw, err := os.ReadFile(plan.InstalledPath)
	if err != nil {
		return plan, err
	}
	installed, err := parseCodexTOML(installRaw)
	if err != nil {
		return plan, fmt.Errorf("%s: %w", plan.InstalledPath, err)
	}
	freshRaw, err := os.ReadFile(plan.FreshPath)
	if err != nil {
		return plan, err
	}
	fresh, err := parseCodexTOML(freshRaw)
	if err != nil {
		return plan, fmt.Errorf("%s: %w", plan.FreshPath, err)
	}

	// Body comparison: Codex's developer_instructions block IS the body.
	// The Codex renderer trims trailing newlines on the way in, so equality
	// here is reliable.
	if installed.DeveloperInstructions != fresh.DeveloperInstructions {
		// Source body keeps the trailing newline that the renderer strips,
		// so we add it back when projecting into source.
		plan.NewBody = installed.DeveloperInstructions + "\n"
	}
	if installed.Description != fresh.Description && installed.Description != "" {
		plan.NewDescription = installed.Description
	}

	// Model + effort reverse: Codex catalog encodes effort as "id:effort";
	// reconstruct the encoded form from the installed pair, then look up.
	if installed.Model != fresh.Model || installed.ModelReasoningEffort != fresh.ModelReasoningEffort {
		encoded := installed.Model
		if installed.ModelReasoningEffort != "" {
			encoded = installed.Model + ":" + installed.ModelReasoningEffort
		}
		if role, ok := reverseModel("codex", encoded, p); ok {
			plan.NewModelRole = role
		} else {
			plan.UnsupportedFrontmatter = true
		}
	}
	return plan, nil
}

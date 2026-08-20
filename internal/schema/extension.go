package schema

import "strings"

// Extension is a Pi-only verbatim artifact: a single .ts file or a directory
// of code that lives at <sourceRoot>/extensions/<name>(.ts) and is rendered
// (verbatim copied) into <outRoot>/.pi/agent/extensions/<name>(.ts).
//
// Unlike Agents and Skills there is no transformation: Pi loads the
// extension as runtime code, and other tools (Claude, Codex, etc.) have no
// equivalent. The renderer's job is therefore lifecycle (manifest tracking,
// scope filtering, drift detection), not format conversion.
//
// Source layout:
//
//	ai-source/extensions/
//	  btw.ts                        # universal single-file extension
//	  commit-gate.ts                # work-only single-file extension
//	  commit-gate.toml              # sidecar: scope = "work"
//	  review-loop/                  # directory extension
//	    extension.toml              # sidecar: scope = "work"
//	    index.ts
//	    flows.ts
//	    ...
//
// Sidecars are optional. Missing sidecar = empty Scope = universal (renders
// for both home and work profiles).
type Extension struct {
	// Name is the install name without extension. For "btw.ts" → "btw".
	// For directory "review-loop/" → "review-loop".
	Name string

	// SourcePath is the absolute path to the source artifact. For single-
	// file extensions this is the .ts file; for directory extensions it's
	// the directory.
	SourcePath string

	// IsDirectory is true when SourcePath is a directory.
	IsDirectory bool

	// Scope is the parsed list from the extension sidecar. nil/empty means
	// universal (matches every profile filter). Same semantics as
	// Agent.Scope: a list of one or more known profile names.
	Scope []string
}

// MatchesScope reports whether the extension should render under the given
// profile filter. Empty filter or empty extension scope = always matches.
func (e *Extension) MatchesScope(filter string) bool {
	if filter == "" || len(e.Scope) == 0 {
		return true
	}
	for _, s := range e.Scope {
		if s == filter {
			return true
		}
	}
	return false
}

// ScopeString returns the comma-joined scope list for display.
func (e *Extension) ScopeString() string {
	return strings.Join(e.Scope, ", ")
}

// InstallName returns the on-disk name under ~/.pi/agent/extensions/.
// Adds the .ts suffix back for single-file extensions; directories use
// their bare name.
func (e *Extension) InstallName() string {
	if e.IsDirectory {
		return e.Name
	}
	return e.Name + ".ts"
}

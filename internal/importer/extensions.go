package importer

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jsmestad/syncai/internal/load"
)

// ExtensionCandidate is one installed Pi extension that has no equivalent
// in ai-source/extensions/. The user prototypes a new extension by dropping
// it under ~/.pi/agent/extensions/; this lets them promote it into source
// without manually arranging the file copy.
type ExtensionCandidate struct {
	Name        string // "btw" or "review-loop"
	InputPath   string // /Users/x/.pi/agent/extensions/btw.ts
	SourcePath  string // /Users/x/code/.../ai-source/extensions/btw.ts
	IsDirectory bool
}

// ExtensionDirectory is a directory under ~/.pi/agent/extensions/ that
// isn't tracked in source. Directory extensions are NOT auto-imported
// because we can't reliably distinguish a user-prototyped directory from
// an external pi extension installed via `pi extension install` or pnpm
// (both produce real directories with their own package.json). The Hint
// field lets the CLI flag obvious externals so users can ignore them and
// focus on the ones that look like their own work.
type ExtensionDirectory struct {
	Name        string // "review-loop" or "buildkite"
	InputPath   string // /Users/x/.pi/agent/extensions/review-loop
	SourcePath  string // /Users/x/code/.../ai-source/extensions/review-loop
	HasPackage  bool   // true if package.json exists at the root
	PackageName string // contents of "name" field in package.json ("" if missing)
}

// Hint returns a short label classifying the directory by how likely it is
// to be an external pi extension vs a user prototype:
//   - "external" — has a package.json with a name (probably installed by pi/pnpm)
//   - "prototype" — no package.json (probably hand-rolled by the user)
//   - "check" — has a package.json without a name (ambiguous)
func (d ExtensionDirectory) Hint() string {
	if !d.HasPackage {
		return "prototype"
	}
	if d.PackageName != "" {
		return "external"
	}
	return "check"
}

// ScanExtensions walks ~/.pi/agent/extensions/ and returns single-file .ts
// extensions that don't have a matching name in ai-source/extensions/.
//
// Directory extensions are intentionally skipped because we can't
// reliably tell a user-prototyped directory from an external pi extension
// (e.g. buildkite, observe, slack — all installed by pi as real
// directories with package.json and runtime deps). Auto-importing those
// would vendor third-party code into the user's source tree. Vendoring a
// directory extension is a deliberate decision: do it by hand with
// `cp -R`, then add an extension.toml sidecar.
//
// Filtering rules:
//   - Symlinks (anywhere) are skipped: pi may also install via symlinks
//     into the nix store or shop-pi-fy clones.
//   - Directories are skipped (per the rationale above).
//   - Anything in <sourceRoot>/extensions/ already is skipped.
//   - Hidden files / dot-prefixed entries are skipped.
//   - Non-.ts files (.md, .json, etc.) are skipped.
//
// The result is the user's hand-rolled single-file prototypes ready to
// be promoted into source.
func ScanExtensions(homeDir, sourceRoot string) ([]ExtensionCandidate, error) {
	installDir := filepath.Join(homeDir, ".pi", "agent", "extensions")
	known, err := extensionNameSet(filepath.Join(sourceRoot, "extensions"))
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(installDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading %s: %w", installDir, err)
	}
	var out []ExtensionCandidate
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), ".ts") {
			continue
		}
		// Use Lstat (not Stat) so we see symlinks as symlinks, not as
		// the resolved target.
		info, err := os.Lstat(filepath.Join(installDir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", e.Name(), err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".ts")
		if known[name] {
			continue
		}
		out = append(out, ExtensionCandidate{
			Name:        name,
			InputPath:   filepath.Join(installDir, e.Name()),
			SourcePath:  filepath.Join(sourceRoot, "extensions", name+".ts"),
			IsDirectory: false,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// PortExtension copies the installed single-file extension into the source
// tree verbatim. It does NOT generate a sidecar TOML — scope is universal
// by default, and the user can add a sidecar manually if a different scope
// is needed. (Auto-detecting scope is impossible without user intent.)
//
// Directory candidates are not produced by ScanExtensions, so this only
// handles the single-file case.
func PortExtension(sourceRoot string, c ExtensionCandidate) error {
	if c.IsDirectory {
		return fmt.Errorf("directory extensions cannot be auto-imported; copy by hand and add a sidecar")
	}
	return load.CopyFileReplacing(sourceRoot, c.InputPath, c.SourcePath)
}

// extensionNameSet returns the set of extension names already present in
// ai-source/extensions/. Sidecar TOMLs are not counted as extensions; only
// .ts files and subdirectories are.
func extensionNameSet(dir string) (map[string]bool, error) {
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
			out[e.Name()] = true
			continue
		}
		if strings.HasSuffix(e.Name(), ".ts") {
			out[strings.TrimSuffix(e.Name(), ".ts")] = true
		}
	}
	return out, nil
}

// ScanExtensionDirectories returns directory entries under
// ~/.pi/agent/extensions/ that don't have a name match in source. The
// caller renders these as a separate "manual vendoring" section because
// auto-import is unsafe (see the Hint comments).
//
// Symlinked directories (pi packages installed via the nix store or
// shop-pi-fy clones) are skipped — those are managed elsewhere.
func ScanExtensionDirectories(homeDir, sourceRoot string) ([]ExtensionDirectory, error) {
	installDir := filepath.Join(homeDir, ".pi", "agent", "extensions")
	known, err := extensionNameSet(filepath.Join(sourceRoot, "extensions"))
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(installDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading %s: %w", installDir, err)
	}
	var out []ExtensionDirectory
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if !e.IsDir() {
			continue
		}
		info, err := os.Lstat(filepath.Join(installDir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", e.Name(), err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if known[e.Name()] {
			continue
		}
		dirPath := filepath.Join(installDir, e.Name())
		hasPkg, pkgName := readPackageMetadata(dirPath)
		out = append(out, ExtensionDirectory{
			Name:        e.Name(),
			InputPath:   dirPath,
			SourcePath:  filepath.Join(sourceRoot, "extensions", e.Name()),
			HasPackage:  hasPkg,
			PackageName: pkgName,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// readPackageMetadata reports whether <dir>/package.json exists and, if so,
// the value of its top-level "name" field. Failures and missing fields
// degrade silently to (false, "") / (true, "") respectively — we never
// fail the scan over a malformed package.json.
func readPackageMetadata(dir string) (bool, string) {
	raw, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return false, ""
	}
	// Tiny ad-hoc decoder: just look for `"name":"value"` in the first 4KB.
	// We don't want a full JSON dependency for one field, and a malformed
	// package.json shouldn't crash the importer.
	limit := len(raw)
	if limit > 4096 {
		limit = 4096
	}
	body := string(raw[:limit])
	idx := strings.Index(body, "\"name\"")
	if idx < 0 {
		return true, ""
	}
	after := body[idx+len("\"name\""):]
	colon := strings.Index(after, ":")
	if colon < 0 {
		return true, ""
	}
	rest := strings.TrimSpace(after[colon+1:])
	if !strings.HasPrefix(rest, "\"") {
		return true, ""
	}
	rest = rest[1:]
	end := strings.Index(rest, "\"")
	if end < 0 {
		return true, ""
	}
	return true, rest[:end]
}

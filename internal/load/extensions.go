package load

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/jsmestad/syncai/internal/schema"
)

// Extensions discovers every Pi extension under <sourceRoot>/extensions/.
// Each entry is one of:
//   - a single ".ts" file (e.g. "btw.ts")
//   - a directory (e.g. "review-loop/")
//
// Optional sidecars declare metadata. For "<name>.ts" the sidecar is
// "<name>.toml" next to it; for a directory it's "<dir>/extension.toml".
//
// When scopeFilter is non-empty, extensions whose sidecar declares a
// different scope are excluded. Extensions without a sidecar are universal
// and always included.
func Extensions(sourceRoot, scopeFilter string) ([]*schema.Extension, error) {
	dir := filepath.Join(sourceRoot, "extensions")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading %s: %w", dir, err)
	}
	var out []*schema.Extension
	for _, e := range entries {
		// Sidecar TOMLs are paired with their owning extension. Skip them
		// at the directory level so they don't show up as extensions.
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".toml") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		ext, err := loadExtension(path, e.IsDir())
		if err != nil {
			return nil, err
		}
		if ext == nil {
			continue
		}
		if scopeFilter != "" && !ext.MatchesScope(scopeFilter) {
			continue
		}
		out = append(out, ext)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// loadExtension constructs an Extension from a single source path. Returns
// (nil, nil) for entries that aren't valid extensions (e.g. a top-level
// non-.ts file that we should silently ignore — though there are none today).
func loadExtension(path string, isDir bool) (*schema.Extension, error) {
	base := filepath.Base(path)
	if isDir {
		scope, err := readSidecarScope(filepath.Join(path, "extension.toml"))
		if err != nil {
			return nil, err
		}
		return &schema.Extension{
			Name:        base,
			SourcePath:  path,
			IsDirectory: true,
			Scope:       scope,
		}, nil
	}
	if !strings.HasSuffix(base, ".ts") {
		// Future: support .mjs, .js. For now Pi extensions are TypeScript
		// modules; anything else is probably misplaced.
		return nil, nil
	}
	name := strings.TrimSuffix(base, ".ts")
	sidecar := strings.TrimSuffix(path, ".ts") + ".toml"
	scope, err := readSidecarScope(sidecar)
	if err != nil {
		return nil, err
	}
	return &schema.Extension{
		Name:        name,
		SourcePath:  path,
		IsDirectory: false,
		Scope:       scope,
	}, nil
}

// extensionSidecar is the TOML schema for ai-source/extensions/<x>.toml.
// Scope is decoded as interface{} so we can accept both string
// (`scope = "work"`) and array (`scope = ["home", "work"]`) forms. Older
// sidecars use the string form; new ones can use the array form.
type extensionSidecar struct {
	Scope interface{} `toml:"scope"`
}

// readSidecarScope returns the parsed scope list from a sidecar TOML, or
// nil if the file does not exist or has no scope field. Each list entry
// is validated against schema.ValidScope so typos surface immediately.
//
// Accepted forms:
//
//	scope = "home"             # single string → ["home"]
//	scope = "home, work"       # CSV string  → ["home", "work"]
//	scope = ["home", "work"]   # array       → ["home", "work"]
//	# (no scope line)          → nil (universal)
func readSidecarScope(path string) ([]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var meta extensionSidecar
	if _, err := toml.Decode(string(raw), &meta); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	list, err := normaliseScope(meta.Scope)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	for _, s := range list {
		if !schema.ValidScope(s) {
			return nil, fmt.Errorf("%s: unknown scope %q (must be \"home\" or \"work\")", path, s)
		}
	}
	return list, nil
}

// normaliseScope converts the loose interface{} from the TOML decoder into
// a clean []string. Strings are CSV-split for parity with the markdown
// frontmatter parser. Arrays are validated to contain only strings.
func normaliseScope(v interface{}) ([]string, error) {
	switch x := v.(type) {
	case nil:
		return nil, nil
	case string:
		return schema.SplitCSV(x), nil
	case []interface{}:
		out := make([]string, 0, len(x))
		for _, item := range x {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("scope array entry must be a string, got %T", item)
			}
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, s)
			}
		}
		return out, nil
	default:
		return nil, fmt.Errorf("scope must be a string or array of strings, got %T", v)
	}
}

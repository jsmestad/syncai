package profiles

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// File mirrors model-profiles.json on disk.
//
// Targets fall in two buckets:
//   - "Toggleable" targets (pi, omp, opencode) live under Profiles[<active>].
//     Their model selection switches with activeProfile.
//   - "Fixed" targets (claude, codex, antigravity) live under Fixed and ignore
//     the active profile because each tool only supports one provider.
type File struct {
	ActiveProfile string                                  `json:"activeProfile"`
	Profiles      map[string]map[string]map[string]string `json:"profiles"`
	Fixed         map[string]map[string]string            `json:"fixed"`

	// Path is where the base file was loaded from. Environment identifies the
	// optional home/work override merged into this in-memory catalog.
	Path        string `json:"-"`
	Environment string `json:"-"`
}

// Load reads model-profiles.json from disk.
func Load(path string) (*File, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var f File
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if f.ActiveProfile == "" {
		return nil, fmt.Errorf("%s: activeProfile is empty", path)
	}
	if _, ok := f.Profiles[f.ActiveProfile]; !ok && len(f.Profiles) > 0 {
		return nil, fmt.Errorf("%s: activeProfile %q not present in profiles", path, f.ActiveProfile)
	}
	f.Path = path
	return &f, nil
}

// LoadWithProfile reads model-profiles.json and applies the model-profile
// precedence chain without an environment override. Kept for project-local
// catalogs and callers that do not distinguish home from work.
func LoadWithProfile(path, override string) (*File, error) {
	return LoadWithEnvironment(path, override, "")
}

// LoadWithEnvironment resolves two independent axes:
//
//   - environment (home|work) selects model-overrides/<environment>.json
//   - profile (openai|claude|mixed) selects the strategy inside the merged catalog
//
// The environment override deep-merges role mappings without creating a
// home-openai/work-openai cross-product. Model-profile precedence remains:
//
//  1. explicit override (e.g. --profile flag) — empty string means skip
//  2. AI_MODEL_PROFILE env var
//  3. PI_MODEL_PROFILE env var
//  4. ~/.pi/agent/active-model-profile.json
//  5. activeProfile in the base catalog
func LoadWithEnvironment(path, profileOverride, environment string) (*File, error) {
	f, err := Load(path)
	if err != nil {
		return nil, err
	}
	if environment != "" {
		overlayPath := filepath.Join(filepath.Dir(path), "model-overrides", environment+".json")
		if err := f.mergeOverlay(overlayPath); err != nil {
			return nil, err
		}
		f.Environment = environment
	}
	if chosen := chooseProfile(profileOverride); chosen != "" {
		if _, ok := f.Profiles[chosen]; !ok {
			return nil, fmt.Errorf("requested profile %q not present in %s", chosen, path)
		}
		f.ActiveProfile = chosen
	}
	return f, nil
}

type overlay struct {
	Profiles map[string]map[string]map[string]string `json:"profiles"`
	Fixed    map[string]map[string]string            `json:"fixed"`
}

func (f *File) mergeOverlay(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading %s: %w", path, err)
	}
	var o overlay
	if err := json.Unmarshal(raw, &o); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}
	if f.Profiles == nil {
		f.Profiles = map[string]map[string]map[string]string{}
	}
	for profile, targets := range o.Profiles {
		if f.Profiles[profile] == nil {
			f.Profiles[profile] = map[string]map[string]string{}
		}
		for target, roles := range targets {
			if f.Profiles[profile][target] == nil {
				f.Profiles[profile][target] = map[string]string{}
			}
			for role, model := range roles {
				f.Profiles[profile][target][role] = model
			}
		}
	}
	if f.Fixed == nil {
		f.Fixed = map[string]map[string]string{}
	}
	for target, roles := range o.Fixed {
		if f.Fixed[target] == nil {
			f.Fixed[target] = map[string]string{}
		}
		for role, model := range roles {
			f.Fixed[target][role] = model
		}
	}
	return nil
}

func chooseProfile(override string) string {
	if override != "" {
		return override
	}
	if v := os.Getenv("AI_MODEL_PROFILE"); v != "" {
		return v
	}
	if v := os.Getenv("PI_MODEL_PROFILE"); v != "" {
		return v
	}
	if home, err := os.UserHomeDir(); err == nil {
		path := filepath.Join(home, ".pi", "agent", "active-model-profile.json")
		if raw, err := os.ReadFile(path); err == nil {
			var doc struct {
				ActiveProfile string `json:"activeProfile"`
			}
			if json.Unmarshal(raw, &doc) == nil && doc.ActiveProfile != "" {
				return doc.ActiveProfile
			}
		}
	}
	return ""
}

// SetActiveProfile persists the user's preferred profile into
// ~/.pi/agent/active-model-profile.json. Used by `syncai set-profile`.
func SetActiveProfile(profile string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(home, ".pi", "agent", "active-model-profile.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	body, _ := json.MarshalIndent(map[string]string{"activeProfile": profile}, "", "  ")
	body = append(body, '\n')
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// Resolve maps (target, role) → concrete model id.
//
// For fixed targets, the active profile is ignored. For toggleable targets,
// resolution walks the active profile. Falls back through the supplied
// fallback roles in order before returning an error.
func (f *File) Resolve(target, role string, fallbacks ...string) (string, error) {
	tries := append([]string{role}, fallbacks...)
	prof := f.lookupTarget(target)
	if prof == nil {
		return "", fmt.Errorf("no entry for target %q (active profile %q, fixed targets %v)",
			target, f.ActiveProfile, fixedKeys(f.Fixed))
	}
	for _, r := range tries {
		if r == "" {
			continue
		}
		if model, ok := prof[r]; ok {
			return model, nil
		}
	}
	return "", fmt.Errorf("no model for target=%s role=%s (fallbacks=%v)", target, role, fallbacks)
}

// HasTarget reports whether the catalog declares mappings for the target.
// Used by renderers to decide whether to emit a model: line at all.
func (f *File) HasTarget(target string) bool {
	return f.lookupTarget(target) != nil
}

// ResolvedTarget returns a copy of the active role-to-model map for target.
// Callers use this for status output without gaining mutation access to the
// loaded catalog.
func (f *File) ResolvedTarget(target string) map[string]string {
	resolved := f.lookupTarget(target)
	out := make(map[string]string, len(resolved))
	for role, model := range resolved {
		out[role] = model
	}
	return out
}

func (f *File) lookupTarget(target string) map[string]string {
	if m, ok := f.Fixed[target]; ok {
		return m
	}
	if prof, ok := f.Profiles[f.ActiveProfile]; ok {
		if m, ok := prof[target]; ok {
			return m
		}
	}
	return nil
}

func fixedKeys(m map[string]map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

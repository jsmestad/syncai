package packages

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

type Manifest struct {
	Pi          PiManifest          `json:"pi"`
	Claude      ClaudeManifest      `json:"claude"`
	Codex       CodexManifest       `json:"codex"`
	Antigravity AntigravityManifest `json:"antigravity"`
}

type PiManifest struct {
	Packages          []string            `json:"packages"`
	PackagesByScope   map[string][]string `json:"packagesByScope,omitempty"`
	NPMCommand        []string            `json:"npmCommand,omitempty"`
	NPMCommandByScope map[string][]string `json:"npmCommandByScope,omitempty"`
}

type ClaudeManifest struct {
	Marketplaces []string `json:"marketplaces"`
	Plugins      []string `json:"plugins"`
}

type CodexManifest struct {
	Plugins []string `json:"plugins"`
}

type AntigravityManifest struct {
	Plugins []string `json:"plugins"`
}

type Status struct {
	Pi          ResourceStatus `json:"pi"`
	Claude      ResourceStatus `json:"claude"`
	Codex       ResourceStatus `json:"codex"`
	Antigravity ResourceStatus `json:"antigravity"`
}

type ResourceStatus struct {
	OK        []string `json:"ok"`
	Missing   []string `json:"missing"`
	Untracked []string `json:"untracked"`
	Orphaned  []string `json:"orphaned"`
}

func DefaultPath(sourceRoot string) string {
	return filepath.Join(sourceRoot, "packages.json")
}

func Load(path string) (*Manifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Manifest{}, nil
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	m.Normalize()
	return &m, nil
}

func Save(path string, m *Manifest) error {
	m.Normalize()
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func (m *Manifest) Normalize() {
	m.Pi.Packages = normalizeStrings(m.Pi.Packages)
	m.Pi.NPMCommand = normalizeCommand(m.Pi.NPMCommand)
	for scope, command := range m.Pi.NPMCommandByScope {
		m.Pi.NPMCommandByScope[scope] = normalizeCommand(command)
	}
	for scope, values := range m.Pi.PackagesByScope {
		normalized := normalizeStrings(values)
		if len(normalized) == 0 {
			delete(m.Pi.PackagesByScope, scope)
			continue
		}
		m.Pi.PackagesByScope[scope] = normalized
	}
	m.Claude.Marketplaces = normalizeStrings(m.Claude.Marketplaces)
	m.Claude.Plugins = normalizeStrings(m.Claude.Plugins)
	m.Codex.Plugins = normalizeStrings(m.Codex.Plugins)
	m.Antigravity.Plugins = normalizeStrings(m.Antigravity.Plugins)
}

func normalizeCommand(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return out
}

func normalizeStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func (m *Manifest) ForScope(scope string) *Manifest {
	resolved := *m
	resolved.Pi = m.Pi
	resolved.Pi.Packages = union(m.Pi.Packages, m.Pi.PackagesByScope[scope])
	if command := m.Pi.NPMCommandByScope[scope]; len(command) > 0 {
		resolved.Pi.NPMCommand = append([]string{}, command...)
	}
	resolved.Pi.PackagesByScope = nil
	resolved.Pi.NPMCommandByScope = nil
	return &resolved
}

func MergeInstalled(path string, installed Status) error {
	return MergeInstalledForScope(path, installed, "")
}

func MergeInstalledForScope(path string, installed Status, scope string) error {
	m, err := Load(path)
	if err != nil {
		return err
	}
	if scope == "" {
		m.Pi.Packages = union(m.Pi.Packages, installed.Pi.Untracked)
	} else {
		if m.Pi.PackagesByScope == nil {
			m.Pi.PackagesByScope = map[string][]string{}
		}
		m.Pi.PackagesByScope[scope] = union(m.Pi.PackagesByScope[scope], installed.Pi.Untracked)
	}
	m.Claude.Plugins = union(m.Claude.Plugins, installed.Claude.Untracked)
	m.Codex.Plugins = union(m.Codex.Plugins, installed.Codex.Untracked)
	m.Antigravity.Plugins = union(m.Antigravity.Plugins, installed.Antigravity.Untracked)
	return Save(path, m)
}

func union(a, b []string) []string {
	return normalizeStrings(append(append([]string{}, a...), b...))
}

func ApplyPiSettings(ctx context.Context, home string, desired PiManifest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(desired.Packages) == 0 && len(desired.NPMCommand) == 0 {
		return nil
	}
	path := filepath.Join(home, ".pi", "agent", "settings.json")
	settings := map[string]any{}
	if raw, err := os.ReadFile(path); err == nil && len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &settings); err != nil {
			return fmt.Errorf("parsing %s: %w", path, err)
		}
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	settings["packages"] = reconcilePackageValues(settings["packages"], desired.Packages)
	if len(desired.NPMCommand) > 0 {
		settings["npmCommand"] = desired.NPMCommand
	}
	raw, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func ApplyClaude(ctx context.Context, home string, desired ClaudeManifest) []error {
	var errs []error
	if err := ctx.Err(); err != nil {
		return []error{err}
	}
	if _, err := exec.LookPath("claude"); err != nil {
		if len(desired.Marketplaces) > 0 || len(desired.Plugins) > 0 {
			errs = append(errs, fmt.Errorf("claude command not found"))
		}
		return errs
	}
	installed, _ := Discover(home)
	installedMarketplaces := set(installedClaudeMarketplaces(home))
	for _, source := range desired.Marketplaces {
		if installedMarketplaces[source] {
			continue
		}
		out, err := runCommand(ctx, "claude", "plugin", "marketplace", "add", source)
		if err != nil {
			errs = append(errs, fmt.Errorf("claude marketplace add %s: %w: %s", source, err, strings.TrimSpace(string(out))))
		}
	}
	installedPlugins := set(installed.Claude.OK)
	for _, plugin := range desired.Plugins {
		if installedPlugins[plugin] {
			continue
		}
		out, err := runCommand(ctx, "claude", "plugin", "install", "--scope", "user", plugin)
		if err != nil {
			errs = append(errs, fmt.Errorf("claude plugin install %s: %w: %s", plugin, err, strings.TrimSpace(string(out))))
		}
	}
	return errs
}

var lookPath = exec.LookPath

var runCommand = func(ctx context.Context, name string, args ...string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil && ctx.Err() != nil {
		return out, errors.Join(err, ctx.Err())
	}
	return out, err
}

func ApplyCodex(ctx context.Context, home string, desired CodexManifest) []error {
	var errs []error
	if err := ctx.Err(); err != nil {
		return []error{err}
	}
	if _, err := lookPath("codex"); err != nil {
		if len(desired.Plugins) > 0 {
			errs = append(errs, fmt.Errorf("codex command not found"))
		}
		return errs
	}
	installed := set(installedCodexPlugins(home))
	for _, plugin := range normalizeStrings(desired.Plugins) {
		if installed[plugin] {
			continue
		}
		out, err := runCommand(ctx, "codex", "plugin", "add", plugin)
		if err != nil {
			errs = append(errs, fmt.Errorf("codex plugin add %s: %w: %s", plugin, err, strings.TrimSpace(string(out))))
		}
	}
	return errs
}

func Discover(home string) (Status, error) {
	pi := installedPiPackages(home)
	claude := installedClaudePlugins(home)
	codex := installedCodexPlugins(home)
	antigravity := installedAntigravityPlugins(home)
	return Status{
		Pi:          ResourceStatus{OK: pi},
		Claude:      ResourceStatus{OK: claude, Untracked: installedClaudeMarketplaces(home)},
		Codex:       ResourceStatus{OK: codex},
		Antigravity: ResourceStatus{OK: antigravity},
	}, nil
}

func Compare(home string, desired *Manifest) (Status, error) {
	installed, err := Discover(home)
	if err != nil {
		return Status{}, err
	}
	pi := compareList(desired.Pi.Packages, installed.Pi.OK)
	pi.Orphaned, err = orphanedPiArtifactSources(home, desired.Pi.Packages)
	if err != nil {
		return Status{}, err
	}
	return Status{
		Pi:          pi,
		Claude:      compareList(desired.Claude.Plugins, installed.Claude.OK),
		Codex:       compareList(desired.Codex.Plugins, installed.Codex.OK),
		Antigravity: compareList(desired.Antigravity.Plugins, installed.Antigravity.OK),
	}, nil
}

func compareList(desired, installed []string) ResourceStatus {
	d := set(desired)
	i := set(installed)
	var ok, missing, untracked []string
	for v := range d {
		if i[v] {
			ok = append(ok, v)
		} else {
			missing = append(missing, v)
		}
	}
	for v := range i {
		if !d[v] {
			untracked = append(untracked, v)
		}
	}
	return ResourceStatus{OK: normalizeStrings(ok), Missing: normalizeStrings(missing), Untracked: normalizeStrings(untracked)}
}

func HasFindings(s Status) bool {
	return len(s.Pi.Missing)+len(s.Pi.Untracked)+len(s.Pi.Orphaned)+len(s.Claude.Missing)+len(s.Claude.Untracked)+len(s.Codex.Missing)+len(s.Codex.Untracked)+len(s.Antigravity.Missing)+len(s.Antigravity.Untracked) > 0
}

func HasUntracked(s Status) bool {
	return len(s.Pi.Untracked)+len(s.Claude.Untracked)+len(s.Codex.Untracked)+len(s.Antigravity.Untracked) > 0
}

func installedPiPackages(home string) []string {
	path := filepath.Join(home, ".pi", "agent", "settings.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var settings map[string]any
	if json.Unmarshal(raw, &settings) != nil {
		return nil
	}
	return packageValues(settings["packages"])
}

func reconcilePackageValues(existing any, desired []string) []any {
	wanted := set(desired)
	seen := map[string]bool{}
	var out []any
	for _, item := range packageArray(existing) {
		switch v := item.(type) {
		case string:
			if wanted[v] {
				seen[v] = true
				out = append(out, v)
			}
		case map[string]any:
			if source := objectSource(v); source != "" && wanted[source] {
				seen[source] = true
				out = append(out, v)
			}
		}
	}
	for _, v := range desired {
		if !seen[v] {
			out = append(out, v)
		}
	}
	return out
}

func packageValues(value any) []string {
	var out []string
	for _, item := range packageArray(value) {
		switch v := item.(type) {
		case string:
			out = append(out, v)
		case map[string]any:
			if source := objectSource(v); source != "" {
				out = append(out, source)
			}
		}
	}
	return normalizeStrings(out)
}

func packageArray(value any) []any {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	return items
}

func objectSource(value map[string]any) string {
	if source, ok := value["source"].(string); ok {
		return source
	}
	return ""
}

func installedClaudePlugins(home string) []string {
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		return nil
	}
	var settings struct {
		EnabledPlugins map[string]bool `json:"enabledPlugins"`
	}
	if json.Unmarshal(raw, &settings) != nil {
		return nil
	}
	var out []string
	for plugin, enabled := range settings.EnabledPlugins {
		if enabled {
			out = append(out, plugin)
		}
	}
	return normalizeStrings(out)
}

func installedClaudeMarketplaces(home string) []string {
	path := filepath.Join(home, ".claude", "plugins", "known_marketplaces.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var data map[string]struct {
		Source any `json:"source"`
	}
	if json.Unmarshal(raw, &data) != nil {
		return nil
	}
	var out []string
	for name, entry := range data {
		if source := claudeMarketplaceSource(name, entry.Source); source != "" {
			out = append(out, source)
		}
	}
	return normalizeStrings(out)
}

func claudeMarketplaceSource(name string, source any) string {
	m, ok := source.(map[string]any)
	if !ok {
		return name
	}
	if repo, ok := m["repo"].(string); ok && repo != "" {
		return "github:" + repo
	}
	if path, ok := m["path"].(string); ok && path != "" {
		return path
	}
	if url, ok := m["url"].(string); ok && url != "" {
		return url
	}
	return name
}

func installedCodexPlugins(home string) []string {
	path := filepath.Join(home, ".codex", "config.toml")
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var config struct {
		Plugins map[string]struct {
			Enabled bool `toml:"enabled"`
		} `toml:"plugins"`
	}
	if _, err := toml.Decode(string(raw), &config); err != nil {
		return nil
	}
	var out []string
	for plugin, settings := range config.Plugins {
		if settings.Enabled {
			out = append(out, plugin)
		}
	}
	return normalizeStrings(out)
}

func installedAntigravityPlugins(home string) []string {
	root := filepath.Join(home, ".gemini", "antigravity-cli", "plugins")
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out []string
	for _, entry := range entries {
		if entry.IsDir() {
			out = append(out, entry.Name())
		}
	}
	return normalizeStrings(out)
}

func set(values []string) map[string]bool {
	out := map[string]bool{}
	for _, v := range values {
		out[v] = true
	}
	return out
}

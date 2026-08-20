package packages

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type piArtifact struct {
	source  string
	npmName string
	path    string
}

// ApplyPi reconciles package artifacts before updating Pi's configured package list.
// Pi installs missing configured packages on startup, but settings edits alone do not
// uninstall packages that were removed from the manifest.
func ApplyPi(ctx context.Context, home string, desired PiManifest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(desired.Packages) == 0 && len(desired.NPMCommand) == 0 {
		return nil
	}
	if err := reconcilePiArtifacts(ctx, home, desired); err != nil {
		return fmt.Errorf("reconciling Pi package artifacts: %w", err)
	}
	if err := ApplyPiSettings(ctx, home, desired); err != nil {
		return fmt.Errorf("reconciling Pi package settings: %w", err)
	}
	return nil
}

func orphanedPiArtifactSources(home string, desired []string) ([]string, error) {
	artifacts, err := orphanedPiArtifacts(home, desired)
	if err != nil {
		return nil, fmt.Errorf("finding orphaned Pi package artifacts: %w", err)
	}
	sources := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		sources = append(sources, artifact.source)
	}
	return normalizeStrings(sources), nil
}

func orphanedPiArtifacts(home string, desired []string) ([]piArtifact, error) {
	artifacts, err := installedPiArtifacts(home)
	if err != nil {
		return nil, err
	}
	wanted := map[string]bool{}
	for _, source := range desired {
		if identity, ok := piManagedPackageIdentity(source); ok {
			wanted[identity] = true
		}
	}
	orphaned := make([]piArtifact, 0)
	for _, artifact := range artifacts {
		identity, ok := piManagedPackageIdentity(artifact.source)
		if ok && !wanted[identity] {
			orphaned = append(orphaned, artifact)
		}
	}
	sort.Slice(orphaned, func(i, j int) bool { return orphaned[i].source < orphaned[j].source })
	return orphaned, nil
}

func reconcilePiArtifacts(ctx context.Context, home string, desired PiManifest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	orphaned, err := orphanedPiArtifacts(home, desired.Packages)
	if err != nil {
		return err
	}
	var npmNames []string
	var gitArtifacts []piArtifact
	for _, artifact := range orphaned {
		if artifact.npmName != "" {
			npmNames = append(npmNames, artifact.npmName)
		}
		if artifact.path != "" {
			gitArtifacts = append(gitArtifacts, artifact)
		}
	}
	if len(npmNames) > 0 {
		if err := uninstallPiNpmArtifacts(ctx, home, desired.NPMCommand, npmNames); err != nil {
			return err
		}
	}
	gitRoot := filepath.Join(home, ".pi", "agent", "git")
	for _, artifact := range gitArtifacts {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := os.RemoveAll(artifact.path); err != nil {
			return fmt.Errorf("removing orphaned Pi package %s: %w", artifact.source, err)
		}
		pruneEmptyParents(filepath.Dir(artifact.path), gitRoot)
	}
	return nil
}

func uninstallPiNpmArtifacts(ctx context.Context, home string, configuredCommand, names []string) error {
	command, err := piNPMCommand(home, configuredCommand)
	if err != nil {
		return err
	}
	sort.Strings(names)
	installRoot := filepath.Join(home, ".pi", "agent", "npm")
	args := append([]string{}, command[1:]...)
	args = append(args, "uninstall")
	if piPackageManagerName(command) == "bun" {
		args = append(args, "--cwd", installRoot)
	} else {
		args = append(args, "--prefix", installRoot)
	}
	args = append(args, "--")
	args = append(args, names...)
	out, err := runCommand(ctx, command[0], args...)
	if err != nil {
		return fmt.Errorf("uninstalling orphaned Pi npm packages %s: %w: %s", strings.Join(names, ", "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func piNPMCommand(home string, configuredCommand []string) ([]string, error) {
	if command := normalizeCommand(configuredCommand); len(command) > 0 {
		return command, nil
	}
	path := filepath.Join(home, ".pi", "agent", "settings.json")
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return []string{"npm"}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading Pi package manager configuration %s: %w", path, err)
	}
	var settings struct {
		NPMCommand []string `json:"npmCommand"`
	}
	if err := json.Unmarshal(raw, &settings); err != nil {
		return nil, fmt.Errorf("parsing Pi package manager configuration %s: %w", path, err)
	}
	if command := normalizeCommand(settings.NPMCommand); len(command) > 0 {
		return command, nil
	}
	return []string{"npm"}, nil
}

func piPackageManagerName(command []string) string {
	for i := len(command) - 1; i >= 0; i-- {
		name := strings.TrimSuffix(filepath.Base(command[i]), ".cmd")
		switch name {
		case "npm", "pnpm", "bun", "yarn":
			return name
		}
	}
	return strings.TrimSuffix(filepath.Base(command[0]), ".cmd")
}

func installedPiArtifacts(home string) ([]piArtifact, error) {
	npmArtifacts, err := installedPiNpmArtifacts(home)
	if err != nil {
		return nil, err
	}
	gitArtifacts, err := installedPiGitArtifacts(home)
	if err != nil {
		return nil, err
	}
	artifacts := append(npmArtifacts, gitArtifacts...)
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].source < artifacts[j].source })
	return artifacts, nil
}

func installedPiNpmArtifacts(home string) ([]piArtifact, error) {
	path := filepath.Join(home, ".pi", "agent", "npm", "package.json")
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading Pi npm package inventory %s: %w", path, err)
	}
	var manifest struct {
		Dependencies map[string]string `json:"dependencies"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, fmt.Errorf("parsing Pi npm package inventory %s: %w", path, err)
	}
	artifacts := make([]piArtifact, 0, len(manifest.Dependencies))
	for name := range manifest.Dependencies {
		artifacts = append(artifacts, piArtifact{source: "npm:" + name, npmName: name})
	}
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].source < artifacts[j].source })
	return artifacts, nil
}

func installedPiGitArtifacts(home string) ([]piArtifact, error) {
	root := filepath.Join(home, ".pi", "agent", "git")
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("reading Pi git package inventory %s: %w", root, err)
	}
	var artifacts []piArtifact
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() || path == root {
			return nil
		}
		if entry.Name() == ".git" || entry.Name() == "node_modules" {
			return filepath.SkipDir
		}
		if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
			relativePath, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			artifacts = append(artifacts, piArtifact{source: "git:" + filepath.ToSlash(relativePath), path: path})
			return filepath.SkipDir
		} else if !os.IsNotExist(err) {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking Pi git package inventory %s: %w", root, err)
	}
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].source < artifacts[j].source })
	return artifacts, nil
}

func piManagedPackageIdentity(source string) (string, bool) {
	if name, ok := piNpmPackageName(source); ok {
		return "npm:" + name, true
	}
	return piGitPackageIdentity(source)
}

func piNpmPackageName(source string) (string, bool) {
	if !strings.HasPrefix(source, "npm:") {
		return "", false
	}
	spec := strings.TrimSpace(strings.TrimPrefix(source, "npm:"))
	if spec == "" {
		return "", false
	}
	if strings.HasPrefix(spec, "@") {
		slash := strings.Index(spec, "/")
		if slash < 2 || slash == len(spec)-1 {
			return "", false
		}
		if version := strings.Index(spec[slash+1:], "@"); version >= 0 {
			spec = spec[:slash+1+version]
		}
		return spec, true
	}
	if version := strings.Index(spec, "@"); version >= 0 {
		spec = spec[:version]
	}
	return spec, spec != ""
}

func piGitPackageIdentity(source string) (string, bool) {
	raw := strings.TrimSpace(source)
	if strings.HasPrefix(raw, "git:") {
		raw = strings.TrimSpace(strings.TrimPrefix(raw, "git:"))
	} else if !hasGitProtocol(raw) {
		return "", false
	}
	var host, repoPath string
	if strings.HasPrefix(raw, "git@") && strings.Contains(raw, ":") {
		hostAndPath := strings.TrimPrefix(raw, "git@")
		separator := strings.Index(hostAndPath, ":")
		host = hostAndPath[:separator]
		repoPath = hostAndPath[separator+1:]
	} else if strings.Contains(raw, "://") {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Hostname() == "" {
			return "", false
		}
		host = parsed.Hostname()
		repoPath = strings.TrimPrefix(parsed.Path, "/")
	} else {
		separator := strings.Index(raw, "/")
		if separator <= 0 || separator == len(raw)-1 {
			return "", false
		}
		host = raw[:separator]
		repoPath = raw[separator+1:]
	}
	if ref := strings.Index(repoPath, "@"); ref >= 0 {
		repoPath = repoPath[:ref]
	}
	repoPath = strings.TrimSuffix(strings.Trim(repoPath, "/"), ".git")
	if host == "" || repoPath == "" || strings.Contains(repoPath, "\\") {
		return "", false
	}
	for _, part := range strings.Split(repoPath, "/") {
		if part == "" || part == "." || part == ".." {
			return "", false
		}
	}
	return "git:" + strings.ToLower(host) + "/" + repoPath, true
}

func hasGitProtocol(source string) bool {
	lower := strings.ToLower(source)
	return strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "ssh://") || strings.HasPrefix(lower, "git://")
}

func pruneEmptyParents(start, root string) {
	root = filepath.Clean(root)
	for current := filepath.Clean(start); current != root; current = filepath.Dir(current) {
		relativePath, err := filepath.Rel(root, current)
		if err != nil || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
			return
		}
		entries, err := os.ReadDir(current)
		if err != nil || len(entries) > 0 {
			return
		}
		if err := os.Remove(current); err != nil {
			return
		}
	}
}

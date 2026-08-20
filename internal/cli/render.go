package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jsmestad/syncai/internal/check"
	"github.com/jsmestad/syncai/internal/load"
	"github.com/jsmestad/syncai/internal/manifest"
	"github.com/jsmestad/syncai/internal/pathguard"
	"github.com/jsmestad/syncai/internal/profiles"
	"github.com/jsmestad/syncai/internal/renderers"
	"github.com/jsmestad/syncai/internal/schema"
	"github.com/spf13/cobra"
)

type renderOptions struct {
	source  string
	out     string
	profile string
	project string
	scope   string
	check   bool
	dryRun  bool
	force   bool
}

func (a *App) renderCommand() *cobra.Command {
	var options renderOptions
	command := &cobra.Command{
		Use:   "render",
		Short: "Render canonical AI source directly into the install target ($HOME by default)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runRender(cmd.Context(), a.renderers, cmd.OutOrStdout(), cmd.ErrOrStderr(), options)
		},
	}
	command.Flags().StringVar(&options.source, "source", "ai-source", "canonical source directory (ignored when --project is set)")
	command.Flags().StringVar(&options.out, "out", "", "output root (default: $HOME). Use to render into a temp dir for inspection.")
	command.Flags().StringVar(&options.profile, "profile", "", "active model profile override (default: env AI_MODEL_PROFILE / PI_MODEL_PROFILE / ~/.pi/agent/active-model-profile.json / activeProfile field)")
	command.Flags().StringVar(&options.project, "project", "", "render project-local: source from <path>/.pi/agent-source, output under <path>")
	command.Flags().StringVar(&options.scope, "scope", "", "install scope filter (home|work). Agents with `scope:` frontmatter are skipped unless they match. Empty = render everything.")
	command.Flags().BoolVar(&options.check, "check", false, "compare what would be rendered against what's currently installed; exit 1 if any drift")
	command.Flags().BoolVar(&options.dryRun, "dry-run", false, "preview what render would write (same diff as --check) but exit 0 regardless. Read-only.")
	command.Flags().BoolVar(&options.force, "force", false, "overwrite locally-edited (drifted) files. Without --force, render aborts when manifest-tracked files have local edits.")
	return command
}

func runRender(ctx context.Context, available []renderers.Renderer, outWriter, errWriter io.Writer, options renderOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateScope(options.scope); err != nil {
		return err
	}
	source, out, projectMode, err := resolvePaths(options)
	if err != nil {
		return err
	}

	in, err := loadInputs(source, options.profile, projectMode, options.scope)
	if err != nil {
		return err
	}
	totalAgents := len(in.Agents)
	in.Agents = filterByScope(in.Agents, options.scope)
	warnIfEmptyRender(errWriter, options.scope, totalAgents, len(in.Agents))

	if options.check || options.dryRun {
		err := runCheckMode(ctx, available, outWriter, in, out, options.scope)
		if options.dryRun && err != nil {
			fmt.Fprintln(outWriter, "\n(dry-run: no files written)")
			return nil
		}
		return err
	}

	scopeNote := "(no scope filter)"
	if options.scope != "" {
		scopeNote = fmt.Sprintf("(scope=%s, %d/%d agents match)", options.scope, len(in.Agents), totalAgents)
	}
	fmt.Fprintf(outWriter, "rendering to %s %s\n", out, scopeNote)

	if options.out != "" || projectMode {
		for _, renderer := range available {
			if err := ctx.Err(); err != nil {
				return err
			}
			written, err := renderer.Render(in, out)
			if err != nil {
				return fmt.Errorf("%s: %w", renderer.Name(), err)
			}
			fmt.Fprintf(outWriter, "%-9s wrote %d files\n", renderer.Name(), len(written))
		}
		return nil
	}

	manifestPath, err := manifest.DefaultPath()
	if err != nil {
		return err
	}
	old, err := manifest.Load(manifestPath)
	if err != nil {
		return err
	}

	if !options.force && len(old.Files) > 0 {
		drifted, _, err := manifest.Drifted(old)
		if err != nil {
			return fmt.Errorf("drift check: %w", err)
		}
		unreconciled, err := unreconciledDrift(ctx, in, out, drifted, available)
		if err != nil {
			return fmt.Errorf("reconciling drift: %w", err)
		}
		if len(unreconciled) > 0 {
			fmt.Fprintln(errWriter, "drift detected — refusing to overwrite locally-edited files:")
			for _, path := range unreconciled {
				fmt.Fprintf(errWriter, "  ~ %s\n", path)
			}
			fmt.Fprintln(errWriter)
			fmt.Fprintln(errWriter, "Run `syncai pull` to propagate the edits into source,")
			fmt.Fprintln(errWriter, "or pass --force to overwrite (discarding the edits).")
			return fmt.Errorf("%d drifted file(s)", len(unreconciled))
		}
	}

	next := &manifest.Manifest{Scope: options.scope}
	for _, renderer := range available {
		if err := ctx.Err(); err != nil {
			return err
		}
		written, err := renderer.Render(in, out)
		if err != nil {
			return fmt.Errorf("%s: %w", renderer.Name(), err)
		}
		fmt.Fprintf(outWriter, "%-9s wrote %d files\n", renderer.Name(), len(written))
		if err := classifyInto(next, written); err != nil {
			return fmt.Errorf("classifying %s output: %w", renderer.Name(), err)
		}
	}

	filesToRemove, dirsToRemove := manifest.Diff(old, next)
	if err := ctx.Err(); err != nil {
		return err
	}
	if count := len(filesToRemove) + len(dirsToRemove); count > 0 {
		fmt.Fprintf(outWriter, "pruning %d stale entr%s from previous render\n", count, plural(count))
		for _, path := range filesToRemove {
			fmt.Fprintf(outWriter, "  - %s\n", path)
		}
		for _, path := range dirsToRemove {
			fmt.Fprintf(outWriter, "  - %s/\n", path)
		}
		for _, pruneErr := range manifest.Prune(out, filesToRemove, dirsToRemove) {
			fmt.Fprintf(errWriter, "warn: %v\n", pruneErr)
		}
	}

	if err := manifest.Save(manifestPath, next); err != nil {
		return fmt.Errorf("saving manifest: %w", err)
	}
	if err := applyPiPackagesFromSource(ctx, source, out, options.scope); err != nil {
		return fmt.Errorf("applying package manifest: %w", err)
	}
	return nil
}

func unreconciledDrift(ctx context.Context, in renderers.Inputs, out string, drifted []string, available []renderers.Renderer) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(drifted) == 0 {
		return nil, nil
	}

	shadow, err := os.MkdirTemp("", "syncai-drift-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(shadow)

	for _, path := range drifted {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		sourcePath, err := pathguard.Resolve(out, path)
		if err != nil {
			return nil, err
		}
		relative, err := filepath.Rel(out, sourcePath)
		if err != nil {
			return nil, err
		}
		data, err := os.ReadFile(sourcePath)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(sourcePath)
		if err != nil {
			return nil, err
		}
		target, err := pathguard.Resolve(shadow, relative)
		if err != nil {
			return nil, err
		}
		if err := load.WriteFileReplacing(shadow, target, data, info.Mode().Perm()); err != nil {
			return nil, err
		}
	}

	for _, renderer := range available {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if _, err := renderer.Render(in, shadow); err != nil {
			return nil, fmt.Errorf("%s: %w", renderer.Name(), err)
		}
	}

	var unreconciled []string
	for _, path := range drifted {
		relative, err := filepath.Rel(out, path)
		if err != nil {
			return nil, err
		}
		actual, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		expectedPath, err := pathguard.Resolve(shadow, relative)
		if err != nil {
			return nil, err
		}
		expected, err := os.ReadFile(expectedPath)
		if err != nil {
			return nil, fmt.Errorf("reading expected render for %s: %w", path, err)
		}
		if !bytes.Equal(actual, expected) {
			unreconciled = append(unreconciled, path)
		}
	}
	return unreconciled, nil
}

func classifyInto(output *manifest.Manifest, paths []string) error {
	for _, path := range paths {
		info, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("inspecting rendered path %s: %w", path, err)
		}
		if info.IsDir() {
			output.Directories = append(output.Directories, path)
			continue
		}
		hash, err := manifest.HashFile(path)
		if err != nil {
			return fmt.Errorf("hashing rendered file %s: %w", path, err)
		}
		output.Files = append(output.Files, manifest.FileEntry{Path: path, Hash: hash})
	}
	return nil
}

func plural(count int) string {
	if count == 1 {
		return "y"
	}
	return "ies"
}

func seedCodexInstructions(out, shadow string) error {
	source := filepath.Join(out, ".codex", "AGENTS.md")
	data, err := os.ReadFile(source)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading %s: %w", source, err)
	}
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("stating %s: %w", source, err)
	}
	target := filepath.Join(shadow, ".codex", "AGENTS.md")
	if err := load.WriteFileReplacing(shadow, target, data, info.Mode().Perm()); err != nil {
		return fmt.Errorf("writing %s: %w", target, err)
	}
	return nil
}

func runCheckMode(ctx context.Context, available []renderers.Renderer, outWriter io.Writer, in renderers.Inputs, out, scope string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	temporary, err := os.MkdirTemp("", "syncai-check-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary)

	if err := seedCodexInstructions(out, temporary); err != nil {
		return err
	}
	for _, renderer := range available {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := renderer.Render(in, temporary); err != nil {
			return fmt.Errorf("%s: %w", renderer.Name(), err)
		}
	}

	diffs, err := check.Trees(temporary, out)
	if err != nil {
		return err
	}
	scopeNote := ""
	if scope != "" {
		scopeNote = fmt.Sprintf(" (scope=%s)", scope)
	}
	if len(diffs) == 0 {
		fmt.Fprintf(outWriter, "ok: install at %s%s is current\n", out, scopeNote)
		return nil
	}
	fmt.Fprintf(outWriter, "install at %s%s is stale:\n", out, scopeNote)
	for _, diff := range diffs {
		fmt.Fprintf(outWriter, "  - %s\n", diff)
	}
	fmt.Fprintln(outWriter, "\nRun: make ai-sync (or `syncai render` directly)")
	return fmt.Errorf("%d file(s) drifted", len(diffs))
}

func resolvePaths(options renderOptions) (source, out string, projectMode bool, err error) {
	if options.project != "" {
		project, err := filepath.Abs(options.project)
		if err != nil {
			return "", "", false, fmt.Errorf("resolving project path %s: %w", options.project, err)
		}
		return filepath.Join(project, ".pi", "agent-source"), project, true, nil
	}
	source, err = filepath.Abs(options.source)
	if err != nil {
		return "", "", false, fmt.Errorf("resolving source path %s: %w", options.source, err)
	}
	out = options.out
	if out == "" {
		out, err = os.UserHomeDir()
		if err != nil {
			return "", "", false, fmt.Errorf("resolving default output root: %w", err)
		}
	}
	out, err = filepath.Abs(out)
	if err != nil {
		return "", "", false, fmt.Errorf("resolving output root %s: %w", out, err)
	}
	return source, out, false, nil
}

func loadInputs(sourceRoot, profileOverride string, projectMode bool, scope string) (renderers.Inputs, error) {
	profile, err := profiles.LoadWithEnvironment(filepath.Join(sourceRoot, "model-profiles.json"), profileOverride, scope)
	if err != nil {
		return renderers.Inputs{}, err
	}
	agents, err := load.Agents(filepath.Join(sourceRoot, "agents"))
	if err != nil {
		return renderers.Inputs{}, err
	}
	skills, err := load.SkillDirs(sourceRoot, scope)
	if err != nil {
		return renderers.Inputs{}, err
	}
	extensions, err := load.Extensions(sourceRoot, scope)
	if err != nil {
		return renderers.Inputs{}, err
	}
	global, _, err := load.ReadInstructions(sourceRoot, "")
	if err != nil {
		return renderers.Inputs{}, err
	}

	prefixes := map[string]string{}
	for _, target := range []string{"pi", "omp", "claude", "antigravity", "codex", "opencode"} {
		_, prefix, err := load.ReadInstructions(sourceRoot, target+"-prefix")
		if err != nil {
			return renderers.Inputs{}, err
		}
		if prefix != "" {
			prefixes[target] = prefix
		}
	}

	return renderers.Inputs{
		Agents:              agents,
		Profiles:            profile,
		SkillDirs:           skills,
		Extensions:          extensions,
		InstructionsGlobal:  global,
		InstructionPrefixes: prefixes,
		SourceRoot:          sourceRoot,
		ProjectMode:         projectMode,
	}, nil
}

func filterByScope(agents []*schema.Agent, scope string) []*schema.Agent {
	if scope == "" {
		return agents
	}
	filtered := make([]*schema.Agent, 0, len(agents))
	for _, agent := range agents {
		if agent.MatchesScope(scope) {
			filtered = append(filtered, agent)
		}
	}
	return filtered
}

func validateScope(scope string) error {
	switch scope {
	case "", "home", "work":
		return nil
	default:
		return fmt.Errorf("invalid --scope %q: must be empty, \"home\", or \"work\"", scope)
	}
}

func warnIfEmptyRender(errWriter io.Writer, scope string, total, filtered int) {
	if scope == "" || total == 0 || filtered != 0 {
		return
	}
	fmt.Fprintf(errWriter, "\nwarning: scope=%s filtered out every agent (0/%d match)\n", scope, total)
	fmt.Fprintln(errWriter, "this profile will render only skills/instructions, no agents.")
	fmt.Fprintln(errWriter, "if that's not intentional, drop `scope: home` from agents that apply at work.")
	fmt.Fprintln(errWriter)
}

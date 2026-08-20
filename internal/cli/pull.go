package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jsmestad/syncai/internal/manifest"
	"github.com/jsmestad/syncai/internal/pull"
	"github.com/jsmestad/syncai/internal/renderers"
	"github.com/jsmestad/syncai/internal/status"
	"github.com/spf13/cobra"
)

type pullOptions struct {
	source string
	out    string
	scope  string
	all    bool
}

func (a *App) pullCommand() *cobra.Command {
	var options pullOptions
	command := &cobra.Command{
		Use:   "pull [name...]",
		Short: "Propagate locally-edited installed files back into ai-source/",
		Long: `Detect drifted files (manifest-tracked files with local edits since the
last render) and copy the changes back into ai-source/ so the next render
distributes them to every tool. Body and description edits propagate
cleanly; other frontmatter changes (tools list, model alias) are reported
and require manual editing of the source file.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPull(cmd.Context(), a.renderers, cmd.OutOrStdout(), cmd.ErrOrStderr(), options, args)
		},
	}
	command.Flags().StringVar(&options.source, "source", "", "canonical source directory (default: SYNCAI_SOURCE, saved init source, or ./ai-source)")
	command.Flags().StringVar(&options.out, "out", "", "install root (default: $HOME)")
	command.Flags().StringVar(&options.scope, "scope", "", "install scope filter (home|work)")
	command.Flags().BoolVar(&options.all, "all", false, "pull every drifted file")
	return command
}

func runPull(ctx context.Context, available []renderers.Renderer, outWriter, errWriter io.Writer, options pullOptions, names []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateScope(options.scope); err != nil {
		return err
	}
	source, installRoot, _, err := resolvePaths(renderOptions{source: options.source, out: options.out, scope: options.scope})
	if err != nil {
		return err
	}
	in, err := loadInputs(source, "", false, options.scope)
	if err != nil {
		return err
	}
	in.Agents = filterByScope(in.Agents, options.scope)

	manifestPath, err := manifest.DefaultPath()
	if err != nil {
		return err
	}
	old, err := manifest.Load(manifestPath)
	if err != nil {
		return err
	}

	temporary, err := os.MkdirTemp("", "syncai-pull-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary)
	for _, renderer := range available {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := renderer.Render(in, temporary); err != nil {
			return fmt.Errorf("%s: %w", renderer.Name(), err)
		}
	}
	report, err := status.CompareWithRoots(old, temporary, installRoot)
	if err != nil {
		return err
	}

	extensionPlans, err := planExtensionPulls(in, installRoot)
	if err != nil {
		return err
	}
	extensionDrift := []pull.ExtPlan{}
	for _, plan := range extensionPlans {
		if plan.HasChanges() {
			extensionDrift = append(extensionDrift, plan)
		}
	}

	if len(report.Drifted) == 0 && len(extensionDrift) == 0 {
		fmt.Fprintln(outWriter, "ok: nothing to pull (no drifted files)")
		return nil
	}
	if !options.all && len(names) == 0 {
		printPullCandidates(outWriter, report.Drifted, extensionDrift)
		return nil
	}

	wanted := map[string]bool{}
	for _, name := range names {
		wanted[name] = true
	}
	pulledNames := map[string]bool{}
	var unsupported []pull.Plan
	for _, path := range report.Drifted {
		if err := ctx.Err(); err != nil {
			return err
		}
		if strings.Contains(path, "/.pi/agent/extensions/") {
			continue
		}
		tool := toolForPath(path)
		name := basename(path)
		if (!options.all && !wanted[name]) || pulledNames[name] {
			continue
		}
		sourcePath := filepath.Join(source, "agents", name+".md")
		if _, err := os.Stat(sourcePath); err != nil {
			if os.IsNotExist(err) {
				fmt.Fprintf(errWriter, "warn: drifted %s has no matching source at %s, skipping\n", path, sourcePath)
				continue
			}
			return fmt.Errorf("inspecting source agent %s: %w", sourcePath, err)
		}
		relative, err := filepath.Rel(installRoot, path)
		if err != nil {
			return fmt.Errorf("resolving %s relative to %s: %w", path, installRoot, err)
		}
		plan, err := pull.PlanFor(path, filepath.Join(temporary, relative), sourcePath, tool, in.Profiles)
		if err != nil {
			return fmt.Errorf("planning pull for %s: %w", path, err)
		}
		if plan.UnsupportedFrontmatter {
			unsupported = append(unsupported, plan)
			continue
		}
		if !plan.HasChanges() {
			continue
		}
		if err := plan.Apply(source); err != nil {
			return fmt.Errorf("applying pull for %s: %w", path, err)
		}
		printAppliedPull(outWriter, plan)
		pulledNames[name] = true
	}
	printUnsupportedPulls(outWriter, unsupported)

	extensionsPulled := 0
	for _, plan := range extensionDrift {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !options.all && !wanted[plan.Name] {
			continue
		}
		if err := plan.Apply(source); err != nil {
			return fmt.Errorf("applying extension pull for %s: %w", plan.Name, err)
		}
		fmt.Fprintf(outWriter, "pulled extension: %s ← %s (%s)\n", plan.SourcePath, plan.InstalledPath, pull.SummariseChanges(plan.Changes))
		for _, change := range plan.Changes {
			if change.Kind == "removed" {
				fmt.Fprintf(outWriter, "  note: %s exists in source but not install (skipped — remove manually if intended)\n", change.RelPath)
			}
		}
		extensionsPulled++
	}

	if len(pulledNames) > 0 || extensionsPulled > 0 {
		fmt.Fprintln(outWriter)
		fmt.Fprintln(outWriter, "redistributing pulled changes to all targets…")
		if err := runRender(ctx, available, outWriter, errWriter, renderOptions{source: source, out: options.out, scope: options.scope, force: true}); err != nil {
			return fmt.Errorf("post-pull render: %w", err)
		}
	}
	return nil
}

func printPullCandidates(outWriter io.Writer, drifted []string, extensionDrift []pull.ExtPlan) {
	if len(drifted) > 0 {
		fmt.Fprintf(outWriter, "Found %d drifted agent file(s):\n\n", len(drifted))
		for _, path := range drifted {
			fmt.Fprintf(outWriter, "  ~ %s [%s]\n", path, toolForPath(path))
		}
		fmt.Fprintln(outWriter)
	}
	if len(extensionDrift) > 0 {
		fmt.Fprintf(outWriter, "Found %d drifted extension(s):\n\n", len(extensionDrift))
		for _, plan := range extensionDrift {
			fmt.Fprintf(outWriter, "  ~ %s [extension] %s\n", plan.InstalledPath, pull.SummariseChanges(plan.Changes))
		}
		fmt.Fprintln(outWriter)
	}
	fmt.Fprintln(outWriter, "Run `syncai pull --all` to pull every drift,")
	fmt.Fprintln(outWriter, "or `syncai pull <name>` to pull specific entries.")
}

func printAppliedPull(outWriter io.Writer, plan pull.Plan) {
	fmt.Fprintf(outWriter, "pulled: %s ← %s\n", plan.SourcePath, plan.InstalledPath)
	if plan.NewBody != "" {
		fmt.Fprintln(outWriter, "  body updated")
	}
	if plan.NewDescription != "" {
		fmt.Fprintln(outWriter, "  description updated")
	}
	if plan.NewTools != "" {
		fmt.Fprintf(outWriter, "  tools updated → %s\n", plan.NewTools)
	}
	if plan.NewModelRole != "" {
		fmt.Fprintf(outWriter, "  modelRole updated → %s\n", plan.NewModelRole)
	}
}

func printUnsupportedPulls(outWriter io.Writer, plans []pull.Plan) {
	if len(plans) == 0 {
		return
	}
	fmt.Fprintln(outWriter)
	fmt.Fprintf(outWriter, "%d drifted file(s) need manual port (frontmatter changed beyond description):\n", len(plans))
	for _, plan := range plans {
		fmt.Fprintf(outWriter, "  ~ %s [%s] → %s\n", plan.InstalledPath, plan.Tool, plan.SourcePath)
	}
	fmt.Fprintln(outWriter, "Edit the source files by hand, then `syncai render` again.")
}

func planExtensionPulls(in renderers.Inputs, installRoot string) ([]pull.ExtPlan, error) {
	extensionInstallRoot := filepath.Join(installRoot, ".pi", "agent", "extensions")
	var plans []pull.ExtPlan
	for _, extension := range in.Extensions {
		installedPath := filepath.Join(extensionInstallRoot, extension.InstallName())
		if _, err := os.Stat(installedPath); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		plan, err := pull.PlanExtension(extension.Name, installedPath, extension.SourcePath, extension.IsDirectory)
		if err != nil {
			return nil, fmt.Errorf("planning extension pull for %s: %w", extension.Name, err)
		}
		plans = append(plans, plan)
	}
	return plans, nil
}

func toolForPath(path string) string {
	switch {
	case strings.Contains(path, "/.pi/agent/agents/"):
		return "pi"
	case strings.Contains(path, "/.omp/agent/agents/"):
		return "omp"
	case strings.Contains(path, "/.claude/agents/"):
		return "claude"
	case strings.Contains(path, "/.codex/agents/"):
		return "codex"
	case strings.Contains(path, "/.config/opencode/agents/"):
		return "opencode"
	case strings.Contains(path, "/.gemini/antigravity-cli/plugins/dfiles/agents/"):
		return "antigravity"
	default:
		return ""
	}
}

func basename(path string) string {
	name := filepath.Base(path)
	for _, suffix := range []string{".md", ".toml"} {
		if strings.HasSuffix(name, suffix) {
			return strings.TrimSuffix(name, suffix)
		}
	}
	return name
}

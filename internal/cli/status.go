package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/jsmestad/syncai/internal/importer"
	"github.com/jsmestad/syncai/internal/manifest"
	"github.com/jsmestad/syncai/internal/pull"
	"github.com/jsmestad/syncai/internal/renderers"
	"github.com/jsmestad/syncai/internal/status"
	"github.com/spf13/cobra"
)

type statusOptions struct {
	source string
	out    string
	scope  string
}

func (a *App) statusCommand() *cobra.Command {
	var options statusOptions
	command := &cobra.Command{
		Use:   "status",
		Short: "Show drift between source and the installed render",
		Long: `Reports manifest-tracked files that have been edited locally (drifted),
deleted locally (missing), or are no longer rendered (stale), plus installed
agents that have no equivalent in source (untracked). Read-only — does not
modify any files. Run before 'syncai render' to make sure local edits will
not be silently overwritten.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runStatus(cmd.Context(), a.renderers, cmd.OutOrStdout(), options)
		},
	}
	command.Flags().StringVar(&options.source, "source", "ai-source", "canonical source directory")
	command.Flags().StringVar(&options.out, "out", "", "install root (default: $HOME)")
	command.Flags().StringVar(&options.scope, "scope", "", "install scope filter (home|work)")
	return command
}

func runStatus(ctx context.Context, available []renderers.Renderer, outWriter io.Writer, options statusOptions) error {
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
	temporary, err := os.MkdirTemp("", "syncai-status-")
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

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	untracked, err := importer.Scan(home, options.source)
	if err != nil {
		return err
	}
	extensionUntracked, err := importer.ScanExtensions(home, options.source)
	if err != nil {
		return err
	}
	skillUntracked, err := importer.ScanSkills(home, options.source)
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

	if !report.HasChanges() && len(untracked) == 0 && len(extensionUntracked) == 0 && len(skillUntracked) == 0 && len(extensionDrift) == 0 {
		fmt.Fprintf(outWriter, "ok: install at %s matches source (manifest %d files)\n", installRoot, len(old.Files))
		return nil
	}
	printStatus(outWriter, report, untracked, extensionUntracked, skillUntracked, extensionDrift)
	return nil
}

func printStatus(outWriter io.Writer, report status.Report, untracked []importer.Candidate, extensionUntracked []importer.ExtensionCandidate, skillUntracked []importer.SkillCandidate, extensionDrift []pull.ExtPlan) {
	agentDrifted := []string{}
	for _, path := range report.Drifted {
		if !strings.Contains(path, "/.pi/agent/extensions/") {
			agentDrifted = append(agentDrifted, path)
		}
	}
	if len(agentDrifted) > 0 {
		fmt.Fprintln(outWriter, "drifted (locally edited, will be overwritten without --force):")
		for _, path := range agentDrifted {
			fmt.Fprintf(outWriter, "  ~ %s\n", path)
		}
		fmt.Fprintln(outWriter)
	}
	if len(report.Missing) > 0 {
		fmt.Fprintln(outWriter, "missing (locally deleted, will be re-created on next render):")
		for _, path := range report.Missing {
			fmt.Fprintf(outWriter, "  ! %s\n", path)
		}
		fmt.Fprintln(outWriter)
	}
	if len(report.Stale) > 0 {
		fmt.Fprintln(outWriter, "stale (no longer rendered, will be pruned on next render):")
		for _, path := range report.Stale {
			fmt.Fprintf(outWriter, "  - %s\n", path)
		}
		fmt.Fprintln(outWriter)
	}
	if len(untracked) > 0 {
		fmt.Fprintln(outWriter, "untracked agents (installed but not in source — run `syncai import`):")
		for _, candidate := range untracked {
			fmt.Fprintf(outWriter, "  ? %s [from %s] (%s)\n", candidate.Name, candidate.Tool, candidate.InputPath)
		}
		fmt.Fprintln(outWriter)
	}
	if len(extensionUntracked) > 0 {
		fmt.Fprintln(outWriter, "untracked extensions (installed but not in source — run `syncai import`):")
		for _, candidate := range extensionUntracked {
			fmt.Fprintf(outWriter, "  ? %s [extension] (%s)\n", candidate.Name, candidate.InputPath)
		}
		fmt.Fprintln(outWriter)
	}
	if len(skillUntracked) > 0 {
		fmt.Fprintln(outWriter, "untracked skills (installed but not in source — run `syncai import`):")
		for _, candidate := range skillUntracked {
			fmt.Fprintf(outWriter, "  ? %s [from %s] (%s)\n", candidate.Name, candidate.Tool, candidate.InputPath)
		}
		fmt.Fprintln(outWriter)
	}
	if len(extensionDrift) > 0 {
		fmt.Fprintln(outWriter, "drifted extensions (installed bytes differ from source — run `syncai pull`):")
		for _, plan := range extensionDrift {
			fmt.Fprintf(outWriter, "  ~ %s [extension] %s\n", plan.InstalledPath, pull.SummariseChanges(plan.Changes))
		}
		fmt.Fprintln(outWriter)
	}
	if len(report.Drifted) > 0 || len(extensionDrift) > 0 {
		fmt.Fprintln(outWriter, "Run `syncai pull` to propagate drifts back into source,")
		fmt.Fprintln(outWriter, "or `syncai render --force` to discard them.")
	}
}

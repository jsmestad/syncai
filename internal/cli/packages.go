package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jsmestad/syncai/internal/config"
	aipackages "github.com/jsmestad/syncai/internal/packages"
	"github.com/spf13/cobra"
)

type packagesOptions struct {
	source string
	out    string
	scope  string
}

func packagesCommand() *cobra.Command {
	command := &cobra.Command{Use: "packages", Short: "Manage AI tool package and plugin manifests"}
	command.AddCommand(
		packagesSubcommand("status", "Show package/plugin drift against ai-source/packages.json", runPackagesStatus),
		packagesSubcommand("apply", "Apply desired packages/plugins from ai-source/packages.json", runPackagesApply),
		packagesSubcommand("pull", "Pull installed packages/plugins into ai-source/packages.json", runPackagesPull),
	)
	return command
}

func packagesSubcommand(use, short string, run func(context.Context, io.Writer, io.Writer, packagesOptions) error) *cobra.Command {
	var options packagesOptions
	command := &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return run(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), options)
		},
	}
	command.Flags().StringVar(&options.source, "source", "", "canonical source directory (default: SYNCAI_SOURCE, saved init source, or ./ai-source)")
	command.Flags().StringVar(&options.out, "out", "", "install root (default: $HOME)")
	command.Flags().StringVar(&options.scope, "scope", "", "machine scope (home|work); scoped Pi packages are excluded when empty")
	return command
}

func runPackagesStatus(ctx context.Context, outWriter, _ io.Writer, options packagesOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateScope(options.scope); err != nil {
		return err
	}
	source, installRoot, err := packageRoots(options.source, options.out)
	if err != nil {
		return err
	}
	desired, err := aipackages.Load(aipackages.DefaultPath(source))
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	status, err := aipackages.Compare(installRoot, desired.ForScope(options.scope))
	if err != nil {
		return err
	}
	printPackageStatus(outWriter, status)
	return nil
}

func runPackagesApply(ctx context.Context, outWriter, errWriter io.Writer, options packagesOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateScope(options.scope); err != nil {
		return err
	}
	source, installRoot, err := packageRoots(options.source, options.out)
	if err != nil {
		return err
	}
	desired, err := aipackages.Load(aipackages.DefaultPath(source))
	if err != nil {
		return err
	}
	if err := aipackages.ApplyPi(ctx, installRoot, desired.ForScope(options.scope).Pi); err != nil {
		return err
	}
	for _, applyErr := range aipackages.ApplyClaude(ctx, installRoot, desired.Claude) {
		if isCancellationError(ctx, applyErr) {
			return applyErr
		}
		fmt.Fprintf(errWriter, "warn: %v\n", applyErr)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, applyErr := range aipackages.ApplyCodex(ctx, installRoot, desired.Codex) {
		if isCancellationError(ctx, applyErr) {
			return applyErr
		}
		fmt.Fprintf(errWriter, "warn: %v\n", applyErr)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	fmt.Fprintln(outWriter, "applied package manifest")
	return runPackagesStatus(ctx, outWriter, errWriter, options)
}

func runPackagesPull(ctx context.Context, outWriter, _ io.Writer, options packagesOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateScope(options.scope); err != nil {
		return err
	}
	source, installRoot, err := packageRoots(options.source, options.out)
	if err != nil {
		return err
	}
	desired, err := aipackages.Load(aipackages.DefaultPath(source))
	if err != nil {
		return err
	}
	status, err := aipackages.Compare(installRoot, desired.ForScope(options.scope))
	if err != nil {
		return err
	}
	if !aipackages.HasFindings(status) {
		fmt.Fprintln(outWriter, "ok: packages manifest is current")
		return nil
	}
	if !aipackages.HasUntracked(status) {
		fmt.Fprintln(outWriter, "ok: no untracked packages/plugins to pull")
		printPackageStatus(outWriter, status)
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := aipackages.MergeInstalledForScope(aipackages.DefaultPath(source), status, options.scope); err != nil {
		return err
	}
	fmt.Fprintf(outWriter, "pulled installed package/plugin state into %s\n", aipackages.DefaultPath(source))
	return nil
}

func applyPiPackagesFromSource(ctx context.Context, sourceRoot, installRoot, scope string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	desired, err := aipackages.Load(aipackages.DefaultPath(sourceRoot))
	if err != nil {
		return err
	}
	return aipackages.ApplyPi(ctx, installRoot, desired.ForScope(scope).Pi)
}

func isCancellationError(ctx context.Context, err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil
}

func packageRoots(sourceRoot, out string) (source, installRoot string, err error) {
	source, err = config.ResolveSource(sourceRoot)
	if err != nil {
		return "", "", err
	}
	installRoot = out
	if installRoot == "" {
		installRoot, err = os.UserHomeDir()
		if err != nil {
			return "", "", fmt.Errorf("resolving default install root: %w", err)
		}
	}
	installRoot, err = filepath.Abs(installRoot)
	if err != nil {
		return "", "", fmt.Errorf("resolving install root %s: %w", installRoot, err)
	}
	return source, installRoot, nil
}

func printPackageStatus(outWriter io.Writer, status aipackages.Status) {
	fmt.Fprintln(outWriter, "packages:")
	printResourceStatus(outWriter, "pi", status.Pi)
	printResourceStatus(outWriter, "claude", status.Claude)
	printResourceStatus(outWriter, "codex", status.Codex)
	printResourceStatus(outWriter, "antigravity", status.Antigravity)
}

func printResourceStatus(outWriter io.Writer, name string, status aipackages.ResourceStatus) {
	fmt.Fprintf(outWriter, "  %s:\n", name)
	if len(status.OK) == 0 && len(status.Missing) == 0 && len(status.Untracked) == 0 && len(status.Orphaned) == 0 {
		fmt.Fprintln(outWriter, "    ok (none declared or installed)")
		return
	}
	for _, value := range status.OK {
		fmt.Fprintf(outWriter, "    ok %s\n", value)
	}
	for _, value := range status.Missing {
		fmt.Fprintf(outWriter, "    missing %s\n", value)
	}
	for _, value := range status.Untracked {
		fmt.Fprintf(outWriter, "    untracked %s\n", value)
	}
	for _, value := range status.Orphaned {
		fmt.Fprintf(outWriter, "    orphaned %s\n", value)
	}
}

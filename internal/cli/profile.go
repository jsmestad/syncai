package cli

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"sort"

	"github.com/jsmestad/syncai/internal/profiles"
	"github.com/jsmestad/syncai/internal/renderers"
	"github.com/jsmestad/syncai/internal/schema"
	"github.com/spf13/cobra"
)

type useProfileOptions struct {
	source string
	scope  string
	force  bool
}

func setProfileCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "set-profile <profile>",
		Short: "Persist the chosen profile without rendering (prefer use-profile)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cmd.Context().Err(); err != nil {
				return err
			}
			path, err := profiles.SetActiveProfile(args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "active profile set to %q at %s\n", args[0], path)
			return nil
		},
	}
}

func (a *App) useProfileCommand() *cobra.Command {
	var options useProfileOptions
	command := &cobra.Command{
		Use:   "use-profile <profile>",
		Short: "Render and then persist a model profile atomically",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUseProfile(cmd.Context(), a.renderers, cmd.OutOrStdout(), cmd.ErrOrStderr(), options, args[0])
		},
	}
	command.Flags().StringVar(&options.source, "source", "ai-source", "canonical source directory")
	command.Flags().StringVar(&options.scope, "scope", "", "machine environment (home|work), required")
	command.Flags().BoolVar(&options.force, "force", false, "overwrite locally-edited rendered files")
	return command
}

func runUseProfile(ctx context.Context, available []renderers.Renderer, outWriter, errWriter io.Writer, options useProfileOptions, profileName string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if options.scope == "" {
		return fmt.Errorf("--scope is required (home or work)")
	}
	if err := runRender(ctx, available, outWriter, errWriter, renderOptions{source: options.source, scope: options.scope, profile: profileName, force: options.force}); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := profiles.SetActiveProfile(profileName)
	if err != nil {
		return err
	}
	profile, err := profiles.LoadWithEnvironment(filepath.Join(options.source, "model-profiles.json"), profileName, options.scope)
	if err != nil {
		return err
	}
	fmt.Fprintf(outWriter, "active model profile set to %q at %s\n", profileName, path)
	printResolvedModels(outWriter, options.scope, profile)
	return nil
}

func printResolvedModels(outWriter io.Writer, scope string, profile *profiles.File) {
	models := profile.ResolvedTarget(string(schema.TargetPi))
	roles := make([]string, 0, len(models))
	for role := range models {
		roles = append(roles, role)
	}
	sort.Strings(roles)
	fmt.Fprintf(outWriter, "resolved Pi models (%s/%s):\n", scope, profile.ActiveProfile)
	for _, role := range roles {
		fmt.Fprintf(outWriter, "  %-12s %s\n", role, models[role])
	}
}

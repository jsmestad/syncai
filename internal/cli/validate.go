package cli

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/jsmestad/syncai/internal/config"
	"github.com/jsmestad/syncai/internal/guidance"
	"github.com/jsmestad/syncai/internal/load"
	aipackages "github.com/jsmestad/syncai/internal/packages"
	"github.com/jsmestad/syncai/internal/profiles"
	"github.com/jsmestad/syncai/internal/renderers"
	"github.com/spf13/cobra"
)

type validateOptions struct {
	source  string
	profile string
}

func validateCommand() *cobra.Command {
	var options validateOptions
	command := &cobra.Command{
		Use:   "validate",
		Short: "Parse source and report errors without writing",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runValidate(cmd.OutOrStdout(), options)
		},
	}
	command.Flags().StringVar(&options.source, "source", "", "canonical source directory (default: SYNCAI_SOURCE, saved init source, or ./ai-source)")
	command.Flags().StringVar(&options.profile, "profile", "", "active model profile override")
	return command
}

func runValidate(out io.Writer, options validateOptions) error {
	source, err := config.ResolveSource(options.source)
	if err != nil {
		return err
	}
	if _, err := profiles.LoadWithProfile(filepath.Join(source, "model-profiles.json"), options.profile); err != nil {
		return err
	}
	agents, err := load.Agents(filepath.Join(source, "agents"))
	if err != nil {
		return err
	}
	skills, err := load.SkillDirs(source, "")
	if err != nil {
		return err
	}
	if err := renderers.ValidateSkillConflicts(skills, guidance.BuiltInSkills()); err != nil {
		return err
	}
	extensions, err := load.Extensions(source, "")
	if err != nil {
		return err
	}
	if _, err := aipackages.Load(aipackages.DefaultPath(source)); err != nil {
		return err
	}
	scopeCounts := map[string]int{}
	for _, agent := range agents {
		key := agent.ScopeString()
		if key == "" {
			key = "(universal)"
		}
		scopeCounts[key]++
	}
	fmt.Fprintf(out, "ok: parsed %d agents, %d skill dirs, %d extensions (scopes: %v)\n", len(agents), len(skills), len(extensions), scopeCounts)
	return nil
}

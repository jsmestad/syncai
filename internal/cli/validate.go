package cli

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/jsmestad/syncai/internal/load"
	aipackages "github.com/jsmestad/syncai/internal/packages"
	"github.com/jsmestad/syncai/internal/profiles"
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
	command.Flags().StringVar(&options.source, "source", "ai-source", "canonical source directory")
	command.Flags().StringVar(&options.profile, "profile", "", "active model profile override")
	return command
}

func runValidate(out io.Writer, options validateOptions) error {
	if _, err := profiles.LoadWithProfile(filepath.Join(options.source, "model-profiles.json"), options.profile); err != nil {
		return err
	}
	agents, err := load.Agents(filepath.Join(options.source, "agents"))
	if err != nil {
		return err
	}
	skills, err := load.SkillDirs(options.source, "")
	if err != nil {
		return err
	}
	extensions, err := load.Extensions(options.source, "")
	if err != nil {
		return err
	}
	if _, err := aipackages.Load(aipackages.DefaultPath(options.source)); err != nil {
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

package cli

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/jsmestad/syncai/internal/config"
	"github.com/jsmestad/syncai/internal/guidance"
	"github.com/jsmestad/syncai/internal/load"
	"github.com/spf13/cobra"
)

type listOptions struct {
	source string
	scope  string
}

func listCommand() *cobra.Command {
	var options listOptions
	command := &cobra.Command{
		Use:   "list",
		Short: "List agents and skills that would render at a given scope (read-only)",
		Long: `Read-only enumeration of what render would emit at the given scope.
Useful for auditing a profile (e.g. "what does work include?") without
writing any files or touching the manifest.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runList(cmd.OutOrStdout(), cmd.ErrOrStderr(), options)
		},
	}
	command.Flags().StringVar(&options.source, "source", "", "canonical source directory (default: SYNCAI_SOURCE, saved init source, or ./ai-source)")
	command.Flags().StringVar(&options.scope, "scope", "", "install scope filter (home|work). Empty = list everything.")
	return command
}

func runList(outWriter, errWriter io.Writer, options listOptions) error {
	if err := validateScope(options.scope); err != nil {
		return err
	}
	source, err := config.ResolveSource(options.source)
	if err != nil {
		return err
	}

	agents, err := load.Agents(filepath.Join(source, "agents"))
	if err != nil {
		return err
	}
	allSkills, err := load.SkillDirs(source, "")
	if err != nil {
		return err
	}
	matchedSkills, err := load.SkillDirs(source, options.scope)
	if err != nil {
		return err
	}

	matched := filterByScope(agents, options.scope)
	scopeNote := "no scope filter"
	if options.scope != "" {
		scopeNote = fmt.Sprintf("scope=%s", options.scope)
	}
	fmt.Fprintf(outWriter, "source: %s (%s)\n\n", source, scopeNote)
	fmt.Fprintf(outWriter, "agents (%d/%d):\n", len(matched), len(agents))
	for _, agent := range matched {
		marker := ""
		if scope := agent.ScopeString(); scope != "" {
			marker = fmt.Sprintf(" [scope: %s]", scope)
		}
		fmt.Fprintf(outWriter, "  %s%s\n", agent.Name, marker)
	}
	if options.scope != "" && len(matched) < len(agents) {
		fmt.Fprintf(outWriter, "\nexcluded (%d, scoped to other profiles):\n", len(agents)-len(matched))
		for _, agent := range agents {
			if !agent.MatchesScope(options.scope) {
				fmt.Fprintf(outWriter, "  %s [scope: %s]\n", agent.Name, agent.ScopeString())
			}
		}
	}

	builtInSkills := guidance.BuiltInSkills()
	fmt.Fprintf(outWriter, "\nskills (%d/%d):\n", len(matchedSkills)+len(builtInSkills), len(allSkills)+len(builtInSkills))
	for _, skill := range matchedSkills {
		fmt.Fprintf(outWriter, "  %s\n", filepath.Base(skill))
	}
	for _, skill := range builtInSkills {
		fmt.Fprintf(outWriter, "  %s [built-in]\n", skill.Name)
	}
	if options.scope != "" && len(matchedSkills) < len(allSkills) {
		matchedSet := map[string]bool{}
		for _, skill := range matchedSkills {
			matchedSet[skill] = true
		}
		fmt.Fprintf(outWriter, "\nexcluded skills (%d):\n", len(allSkills)-len(matchedSkills))
		for _, skill := range allSkills {
			if !matchedSet[skill] {
				fmt.Fprintf(outWriter, "  %s\n", filepath.Base(skill))
			}
		}
	}

	allExtensions, err := load.Extensions(source, "")
	if err != nil {
		return err
	}
	matchedExtensions, err := load.Extensions(source, options.scope)
	if err != nil {
		return err
	}
	fmt.Fprintf(outWriter, "\nextensions (%d/%d):\n", len(matchedExtensions), len(allExtensions))
	for _, extension := range matchedExtensions {
		marker := ""
		if scope := extension.ScopeString(); scope != "" {
			marker = fmt.Sprintf(" [scope: %s]", scope)
		}
		kind := "file"
		if extension.IsDirectory {
			kind = "dir"
		}
		fmt.Fprintf(outWriter, "  %s (%s)%s\n", extension.Name, kind, marker)
	}
	if options.scope != "" && len(matchedExtensions) < len(allExtensions) {
		matchedSet := map[string]bool{}
		for _, extension := range matchedExtensions {
			matchedSet[extension.Name] = true
		}
		fmt.Fprintf(outWriter, "\nexcluded extensions (%d):\n", len(allExtensions)-len(matchedExtensions))
		for _, extension := range allExtensions {
			if !matchedSet[extension.Name] {
				fmt.Fprintf(outWriter, "  %s [scope: %s]\n", extension.Name, extension.ScopeString())
			}
		}
	}

	warnIfEmptyRender(errWriter, options.scope, len(agents), len(matched))
	return nil
}

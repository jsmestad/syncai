package cli

import (
	"fmt"

	"github.com/jsmestad/syncai/internal/guidance"
	"github.com/spf13/cobra"
)

func guideCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "guide",
		Short: "Show workflows and safety guidance for people and coding agents",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprint(cmd.OutOrStdout(), guidance.Guide)
		},
	}
}

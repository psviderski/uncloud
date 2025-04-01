package context

import (
	"github.com/spf13/cobra"
)

func NewRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "context",
		Aliases: []string{"ctx"},
		Short:   "Switch between different cluster contexts. Contains subcommands to manage contexts.",
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO
			return nil
		},
	}

	cmd.AddCommand(
		NewListCommand(),
	)

	return cmd
}

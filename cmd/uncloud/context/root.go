package context

import (
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/spf13/cobra"
)

func NewRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "ctx",
		Aliases: []string{"context"},
		Short:   "Switch between different cluster contexts. Contains subcommands to manage contexts.",
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return selectContext(uncli)
		},
	}

	cmd.AddCommand(
		NewListCommand(),
		NewUseCommand(),
		NewConnectionCommand(),
	)

	return cmd
}

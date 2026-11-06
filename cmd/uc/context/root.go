package context

import (
	"fmt"

	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/cli/completion"
	"github.com/spf13/cobra"
)

func NewRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "ctx",
		Aliases: []string{"context"},
		Short:   "Switch between different cluster contexts. Contains subcommands to manage contexts.",
		Long: `Switch between different cluster contexts.

A single argument that is not a subcommand, switch to the context with that name, with arguments you
can select a context interactively.`,
		Example: `  # Interactively pick a context.
  uc ctx

  # Switch directly to the context "prod".
  uc ctx prod

  # List contexts.
  uc ctx ls`,
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)

			if len(args) == 1 {
				if err := uncli.SetCurrentContext(args[0]); err != nil {
					return fmt.Errorf("failed to set the current cluster context to '%s': %w", args[0], err)
				}
				fmt.Printf("Current cluster context is now '%s'.\n", args[0])
				return nil
			}
			return selectContext(uncli)
		},
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
			if len(args) > 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return completion.Contexts(cmd.Context(), uncli, args, toComplete)
		},
	}

	cmd.AddCommand(
		NewListCommand(),
		NewUseCommand(),
		NewConnectionCommand(),
		NewShowCommand(),
	)

	return cmd
}

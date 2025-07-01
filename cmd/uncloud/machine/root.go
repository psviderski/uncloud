package machine

import (
	"github.com/spf13/cobra"
)

func NewRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "machine",
		Aliases: []string{"m"},
		Short:   "Manage machines in an Uncloud cluster.",
	}
	cmd.AddCommand(
		NewAddCommand(),
		NewInitCommand(),
		NewListCommand(),
		NewRmCommand(),
		NewTokenCommand(),
	)
	return cmd
}

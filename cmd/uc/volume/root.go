package volume

import (
	"github.com/spf13/cobra"
)

func NewRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "volume",
		Short: "Manage volumes in the cluster.",
	}
	cmd.AddCommand(
		NewCreateCommand(),
		NewInspectCommand(),
		NewListCommand(),
		NewRemoveCommand(),
	)
	return cmd
}

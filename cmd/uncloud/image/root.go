package image

import (
	"github.com/spf13/cobra"
)

func NewRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "image",
		Short: "Manage images on machines in the cluster.",
	}

	cmd.AddCommand(
		NewListCommand(),
		NewPushCommand(),
		NewPullCommand(),
		NewRemoveCommand(),
		NewInspectCommand(),
		NewPruneCommand(),
	)

	return cmd
}

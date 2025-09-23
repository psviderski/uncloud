package image

import (
	"github.com/spf13/cobra"
)

func NewRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "image",
		Short: "Manage Docker images in a cluster.",
	}

	cmd.AddCommand(
		NewPushCommand(),
	)

	return cmd
}

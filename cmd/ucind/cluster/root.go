package cluster

import (
	"github.com/spf13/cobra"
)

const DefaultClusterName = "ucind-default"

func NewRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cluster",
		Short: "Manage local Docker-based clusters.",
	}
	cmd.AddCommand(
		NewCreateCommand(),
		// NewListCommand(),
		NewRemoveCommand(),
	)
	return cmd
}

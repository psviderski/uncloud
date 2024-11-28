package cluster

import (
	"fmt"
	"github.com/spf13/cobra"
	"uncloud/internal/ucind"
)

func NewRemoveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rm [NAME]",
		Short: "Remove a cluster.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := cmd.Context().Value("provisioner").(*ucind.Provisioner)

			name := DefaultClusterName
			if len(args) > 0 {
				name = args[0]
			}

			if err := p.RemoveCluster(cmd.Context(), name); err != nil {
				return fmt.Errorf("remove cluster '%s': %w", name, err)
			}
			fmt.Printf("Cluster '%s' removed.\n", name)
			return nil
		},
	}
	return cmd
}

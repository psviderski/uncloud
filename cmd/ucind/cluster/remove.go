package cluster

import (
	"fmt"

	"github.com/psviderski/uncloud/internal/ucind"
	"github.com/psviderski/uncloud/internal/ucind/completion"
	"github.com/spf13/cobra"
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
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
			if len(args) > 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return completion.Clusters(cmd, args, toComplete)
		},
	}
	return cmd
}

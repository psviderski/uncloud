package cluster

import (
	"fmt"
	"github.com/spf13/cobra"
	"uncloud/internal/ucind"
)

type createOptions struct {
}

func NewCreateCommand() *cobra.Command {
	//opts := createOptions{}
	cmd := &cobra.Command{
		Use:   "create [NAME]",
		Short: "Create a new cluster.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := cmd.Context().Value("provisioner").(*ucind.Provisioner)

			name := DefaultClusterName
			if len(args) > 0 {
				name = args[0]
			}

			if err := p.CreateCluster(cmd.Context(), name, ucind.CreateClusterOptions{}); err != nil {
				return fmt.Errorf("create cluster '%s': %w", name, err)
			}
			fmt.Printf("Cluster '%s' created.\n", name)
			return nil
		},
	}
	return cmd
}

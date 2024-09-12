package machine

import (
	"github.com/spf13/cobra"
	"uncloud/internal/cli"
)

func NewListCommand() *cobra.Command {
	opts := addOptions{}
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List machines in a cluster",
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return uncli.ListMachines(cmd.Context(), opts.cluster)
		},
	}
	cmd.Flags().StringVarP(
		&opts.cluster, "cluster", "c", "",
		"Name of the cluster (default is the current cluster)",
	)
	return cmd
}

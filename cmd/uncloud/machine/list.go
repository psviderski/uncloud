package machine

import (
	"github.com/spf13/cobra"
	"uncloud/internal/cli"
)

func NewListCommand() *cobra.Command {
	var cluster string
	cmd := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "List machines in a cluster.",
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return uncli.ListMachines(cmd.Context(), cluster)
		},
	}
	cmd.Flags().StringVarP(
		&cluster, "cluster", "c", "",
		"Name of the cluster. (default is the current cluster)",
	)
	return cmd
}

package main

import (
	"github.com/spf13/cobra"
	"uncloud/internal/cli"
)

func NewRunCommand() *cobra.Command {
	var (
		cluster string
		opts    cli.ServiceOptions
	)

	cmd := &cobra.Command{
		Use:   "run IMAGE",
		Short: "Run a service in a cluster.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)

			opts.Image = args[0]
			return uncli.RunService(cmd.Context(), cluster, &opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Name, "name", "n", "",
		"Assign a name to the service. A random name is generated if not specified.")
	cmd.Flags().StringSliceVarP(&opts.Publish, "publish", "p", nil,
		"Publish a service port to make it accessible outside the cluster. Can be specified multiple times. "+
			"Format: [load_balancer_port:]container_port[/protocol]")
	cmd.Flags().StringVarP(
		&cluster, "cluster", "c", "",
		"Name of the cluster to run the service in. (default is the current cluster)",
	)

	return cmd
}

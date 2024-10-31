package main

import (
	"fmt"
	"github.com/spf13/cobra"
	"uncloud/internal/cli"
)

type runOptions struct {
	name    string
	publish []string
	cluster string
}

func NewRunCommand() *cobra.Command {
	opts := runOptions{}
	cmd := &cobra.Command{
		Use:   "run IMAGE",
		Short: "Run a service in a cluster.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)

			image := args[0]

			//return uncli.RunService(cmd.Context(), ...)
			return fmt.Errorf("not implemented: run image %q %s", image, uncli)
		},
	}
	cmd.Flags().StringVarP(&opts.name, "name", "n", "",
		"Assign a name to the service. A random name is generated if not specified.")
	cmd.Flags().StringVarP(
		&opts.cluster, "cluster", "c", "",
		"Name of the cluster to run the service in. (default is the current cluster)",
	)
	cmd.Flags().StringSliceVarP(&opts.publish, "publish", "p", nil,
		"Publish a service port to make it accessible outside the cluster. Can be specified multiple times. "+
			"Format: [load_balancer_port:]container_port[/protocol]")

	return cmd
}

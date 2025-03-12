package main

import (
	"context"
	"fmt"
	"github.com/spf13/cobra"
	"uncloud/internal/cli"
)

type deployOptions struct {
	//configPath string
	services []string
	//machines   []string
	//env        []string
	//envFile    string
	//projectDir string

	cluster string
}

// NewDeployCommand creates a new command to deploy services from a Compose file.
func NewDeployCommand() *cobra.Command {
	opts := deployOptions{}
	cmd := &cobra.Command{
		Use: "deploy [FLAGS] [SERVICE...]",
		// TODO: remove WIP when the command is fully implemented.
		Short: "WIP: Deploy services from a Compose file.",
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)

			if len(args) > 0 {
				opts.services = args
			}

			return deploy(cmd.Context(), uncli, opts)
		},
	}

	cmd.Flags().StringVarP(&opts.cluster, "cluster", "c", "", "Name of the cluster to deploy to (default is the current cluster)")

	return cmd
}

// deploy parses the compose file and deploys the services to the Uncloud cluster.
func deploy(ctx context.Context, uncli *cli.CLI, opts deployOptions) error {
	return fmt.Errorf("not implemented")
}

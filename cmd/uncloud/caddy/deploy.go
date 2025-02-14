package caddy

import (
	"context"
	"fmt"
	"github.com/spf13/cobra"
	"uncloud/internal/cli"
)

type deployOptions struct {
	image   string
	cluster string
}

func NewDeployCommand() *cobra.Command {
	opts := deployOptions{}

	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy or upgrade Caddy reverse proxy across all machines in the cluster.",
		Long: "Deploy or upgrade Caddy reverse proxy across all machines in the cluster.\n" +
			"It performs a rolling update if Caddy is already running.",
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return deploy(cmd.Context(), uncli, opts)
		},
	}

	cmd.Flags().StringVar(&opts.image, "image", "",
		"Caddy Docker image to deploy. (default caddy:LATEST_VERSION)")
	cmd.Flags().StringVarP(
		&opts.cluster, "cluster", "c", "",
		"Name of the cluster to deploy to. (default is the current cluster)",
	)

	return cmd
}

func deploy(ctx context.Context, uncli *cli.CLI, opts deployOptions) error {
	client, err := uncli.ConnectCluster(ctx, opts.cluster)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer client.Close()

	if err = client.DeployCaddy(ctx, opts.image); err != nil {
		return fmt.Errorf("deploy caddy: %w", err)
	}

	return nil
}

package caddy

import (
	"context"
	"fmt"
	"github.com/docker/compose/v2/pkg/progress"
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

	d, err := client.NewCaddyDeployment(opts.image)
	if err != nil {
		return fmt.Errorf("create caddy deployment: %w", err)
	}

	plan, err := d.Plan(ctx)
	if err != nil {
		return fmt.Errorf("plan caddy deployment: %w", err)
	}

	if len(plan.SequenceOperation.Operations) == 0 {
		fmt.Println("caddy service is up to date.")
		return nil
	}

	return progress.RunWithTitle(ctx, func(ctx context.Context) error {
		if _, err = d.Run(ctx); err != nil {
			return fmt.Errorf("deploy caddy: %w", err)
		}
		return nil
	}, uncli.ProgressOut(), "Deploying service "+d.Spec.Name)
}

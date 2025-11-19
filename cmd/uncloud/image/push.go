package image

import (
	"context"
	"fmt"

	"github.com/containerd/platforms"
	"github.com/docker/compose/v2/pkg/progress"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/pkg/client"
	"github.com/spf13/cobra"
)

type pushOptions struct {
	image    string
	machines []string
	platform string
}

func NewPushCommand() *cobra.Command {
	opts := pushOptions{}

	cmd := &cobra.Command{
		Use:   "push IMAGE",
		Short: "Upload a local Docker image to the cluster.",
		Long: `Upload a local Docker image to the cluster transferring only the missing layers.
The image is uploaded to all cluster machines (default) or the specified machine(s).`,
		Example: `  # Push image to all machines in the cluster.
  uc image push myapp:latest

  # Push image to specific machine.
  uc image push myapp:latest -m machine1

  # Push image to multiple machines.
  uc image push myapp:latest -m machine1,machine2,machine3

  # Push a specific platform of a multi-platform image.
  uc image push myapp:latest --platform linux/amd64`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)

			opts.image = args[0]
			return push(cmd.Context(), uncli, opts)
		},
	}

	cmd.Flags().StringSliceVarP(&opts.machines, "machine", "m", nil,
		"Machine names or IDs to push the image to. Can be specified multiple times or as a comma-separated list. "+
			"(default is all machines)")
	cmd.Flags().StringVar(
		&opts.platform, "platform", "",
		"Push a specific platform of a multi-platform image (e.g., linux/amd64, linux/arm64).\n"+
			"Local Docker must be configured to use containerd image store to support multi-platform images.",
	)

	return cmd
}

func push(ctx context.Context, uncli *cli.CLI, opts pushOptions) error {
	clusterClient, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer clusterClient.Close()

	machines := cli.ExpandCommaSeparatedValues(opts.machines)
	pushOpts := client.PushImageOptions{}

	// Special handling for an explicit "all" keyword to push to all machines.
	if len(machines) == 1 && machines[0] == "all" {
		pushOpts.AllMachines = true
	} else if len(machines) > 0 {
		pushOpts.Machines = machines
	} else {
		// Default is to push to all machines in the cluster.
		pushOpts.AllMachines = true
	}

	if opts.platform != "" {
		p, err := platforms.Parse(opts.platform)
		if err != nil {
			return fmt.Errorf("invalid platform '%s': %w", opts.platform, err)
		}
		pushOpts.Platform = &p
	}

	return progress.RunWithTitle(ctx, func(ctx context.Context) error {
		if err = clusterClient.PushImage(ctx, opts.image, pushOpts); err != nil {
			return fmt.Errorf("push image to cluster: %w", err)
		}
		return nil
	}, uncli.ProgressOut(), fmt.Sprintf("Pushing image %s to cluster", opts.image))
}

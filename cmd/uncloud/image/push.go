package image

import (
	"context"
	"fmt"

	"github.com/docker/compose/v2/pkg/progress"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/spf13/cobra"
)

type pushOptions struct {
	image    string
	machines []string
	context  string
}

func NewPushCommand() *cobra.Command {
	opts := pushOptions{}

	cmd := &cobra.Command{
		Use:   "push IMAGE",
		Short: "Upload a local Docker image to the cluster.",
		Long: `Upload a local Docker image to the cluster transferring only the missing layers.
The image is uploaded to the machine which CLI is connected to (default) or the specified machine(s).`,
		Example: `  # Push image to the currently connected machine.
  uc image push myapp:latest

  # Push image to specific machine.
  uc image push myapp:latest -m machine1

  # Push image to multiple machines.
  uc image push myapp:latest -m machine1,machine2,machine3`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)

			opts.image = args[0]
			return push(cmd.Context(), uncli, opts)
		},
	}

	cmd.Flags().StringSliceVarP(&opts.machines, "machine", "m", nil,
		"Machine names to push the image to. Can be specified multiple times or as a comma-separated "+
			"list of machine names. (default is connected machine)")
	cmd.Flags().StringVarP(
		&opts.context, "context", "c", "",
		"Name of the cluster context. (default is the current context)",
	)

	return cmd
}

func push(ctx context.Context, uncli *cli.CLI, opts pushOptions) error {
	client, err := uncli.ConnectCluster(ctx, opts.context)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer client.Close()

	machines := cli.ExpandCommaSeparatedValues(opts.machines)

	return progress.RunWithTitle(ctx, func(ctx context.Context) error {
		if err = client.PushImage(ctx, opts.image, machines); err != nil {
			return fmt.Errorf("push image to cluster: %w", err)
		}
		return nil
	}, uncli.ProgressOut(), fmt.Sprintf("Pushing image %s to cluster", opts.image))
}

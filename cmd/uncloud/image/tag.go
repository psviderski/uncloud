package image

import (
	"context"
	"fmt"

	"github.com/psviderski/uncloud/internal/cli"
	"github.com/spf13/cobra"
)

type tagOptions struct {
	machines []string
}

func NewTagCommand() *cobra.Command {
	opts := tagOptions{}

	cmd := &cobra.Command{
		Use:   "tag SOURCE_IMAGE TARGET_IMAGE",
		Short: "Create a tag for an image on the cluster.",
		Long:  "Create a tag TARGET_IMAGE that refers to SOURCE_IMAGE. By default, on all machines.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return tag(cmd.Context(), uncli, args[0], args[1], opts)
		},
	}

	cmd.Flags().StringSliceVarP(&opts.machines, "machine", "m", nil,
		"Filter machines to tag image on. Can be specified multiple times or as a comma-separated list. "+
			"(default is all machines)")

	return cmd
}

func tag(ctx context.Context, uncli *cli.CLI, source, target string, opts tagOptions) error {
	clusterClient, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer clusterClient.Close()

	machines := cli.ExpandCommaSeparatedValues(opts.machines)

	if err := clusterClient.TagImage(ctx, source, target, machines); err != nil {
		return fmt.Errorf("tag image: %w", err)
	}

	fmt.Printf("Tagged %s as %s\n", source, target)
	return nil
}

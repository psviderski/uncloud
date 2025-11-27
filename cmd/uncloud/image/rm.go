package image

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types/image"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/spf13/cobra"
)

type removeOptions struct {
	machines []string
	force    bool
	noPrune  bool
}

func NewRemoveCommand() *cobra.Command {
	opts := removeOptions{}

	cmd := &cobra.Command{
		Use:     "rm [IMAGE...]",
		Aliases: []string{"remove", "rmi"},
		Short:   "Remove one or more images from the cluster.",
		Long:    "Remove one or more images from the cluster. By default, from all machines.",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return remove(cmd.Context(), uncli, args, opts)
		},
	}

	cmd.Flags().StringSliceVarP(&opts.machines, "machine", "m", nil,
		"Filter machines to remove images from. Can be specified multiple times or as a comma-separated list. "+
			"(default is all machines)")
	cmd.Flags().BoolVarP(&opts.force, "force", "f", false, "Force removal of the image")
	cmd.Flags().BoolVar(&opts.noPrune, "no-prune", false, "Do not delete untagged parents")

	return cmd
}

func remove(ctx context.Context, uncli *cli.CLI, images []string, opts removeOptions) error {
	clusterClient, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer clusterClient.Close()

	// Get all machines to create ID to name mapping.
	allMachines, err := clusterClient.ListMachines(ctx, nil)
	if err != nil {
		return fmt.Errorf("list machines: %w", err)
	}

	machines := cli.ExpandCommaSeparatedValues(opts.machines)

	removeOpts := image.RemoveOptions{
		Force:         opts.force,
		PruneChildren: !opts.noPrune,
	}

	for _, img := range images {
		responses, err := clusterClient.RemoveImage(ctx, img, removeOpts, machines)
		if err != nil {
			fmt.Printf("Error removing image '%s': %v\n", img, err)
			continue
		}

		for _, resp := range responses {
			machineName := resp.Metadata.Machine
			if m := allMachines.FindByNameOrID(machineName); m != nil {
				machineName = m.Machine.Name
			}

			if resp.Metadata.Error != "" {
				fmt.Printf("[%s] Error: %s\n", machineName, resp.Metadata.Error)
				continue
			}

			// If no specific response items (e.g. image not found but no error returned?), just print success?
			// Docker usually returns items.
			if len(resp.Response) == 0 {
				fmt.Printf("[%s] Image '%s' not found or not removed.\n", machineName, img)
				continue
			}

			for _, item := range resp.Response {
				if item.Untagged != "" {
					fmt.Printf("[%s] Untagged: %s\n", machineName, item.Untagged)
				}
				if item.Deleted != "" {
					fmt.Printf("[%s] Deleted: %s\n", machineName, item.Deleted)
				}
			}
		}
	}

	return nil
}

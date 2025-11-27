package image

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/filters"
	"github.com/docker/go-units"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/spf13/cobra"
)

type pruneOptions struct {
	machines []string
	force    bool
	all      bool
	filter   []string
}

func NewPruneCommand() *cobra.Command {
	opts := pruneOptions{}

	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Remove unused images from the cluster.",
		Long:  "Remove unused images from the cluster. By default, from all machines.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !opts.force {
				fmt.Println("Are you sure you want to remove all dangling images?")
				confirmed, err := cli.Confirm()
				if err != nil {
					return err
				}
				if !confirmed {
					return nil
				}
			}
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return prune(cmd.Context(), uncli, opts)
		},
	}

	cmd.Flags().StringSliceVarP(&opts.machines, "machine", "m", nil,
		"Filter machines to prune images on. Can be specified multiple times or as a comma-separated list. "+
			"(default is all machines)")
	cmd.Flags().BoolVarP(&opts.force, "force", "f", false, "Do not prompt for confirmation")
	cmd.Flags().BoolVarP(&opts.all, "all", "a", false, "Remove all unused images, not just dangling ones")
	cmd.Flags().StringSliceVar(&opts.filter, "filter", nil, "Provide filter values (e.g. 'until=<timestamp>')")

	return cmd
}

func prune(ctx context.Context, uncli *cli.CLI, opts pruneOptions) error {
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

	pruneFilters := filters.NewArgs()
	if opts.all {
		pruneFilters.Add("dangling", "false")
	}
	for _, f := range opts.filter {
		name, value, ok := strings.Cut(f, "=")
		if !ok {
			return fmt.Errorf("invalid filter '%s'", f)
		}
		pruneFilters.Add(name, value)
	}

	responses, err := clusterClient.PruneImages(ctx, pruneFilters, machines)
	if err != nil {
		return fmt.Errorf("prune images: %w", err)
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

		report := resp.Report
		if len(report.ImagesDeleted) > 0 {
			fmt.Printf("[%s] Deleted Images:\n", machineName)
			for _, item := range report.ImagesDeleted {
				if item.Untagged != "" {
					fmt.Printf("untagged: %s\n", item.Untagged)
				}
				if item.Deleted != "" {
					fmt.Printf("deleted: %s\n", item.Deleted)
				}
			}
			fmt.Printf("\n")
		}

		fmt.Printf("[%s] Total reclaimed space: %s\n", machineName, units.HumanSize(float64(report.SpaceReclaimed)))
	}

	return nil
}

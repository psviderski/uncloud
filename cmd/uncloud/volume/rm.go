package volume

import (
	"context"
	"errors"
	"fmt"

	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/spf13/cobra"
)

type removeOptions struct {
	force    bool
	machines []string
	yes      bool
}

func NewRemoveCommand() *cobra.Command {
	opts := removeOptions{}

	cmd := &cobra.Command{
		Use:     "rm VOLUME_NAME [VOLUME_NAME...]",
		Aliases: []string{"remove", "delete"},
		Short:   "Remove one or more volumes.",
		Long:    "Remove one or more volumes. You cannot remove a volume that is in use by a container.",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return remove(cmd.Context(), uncli, args, opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.force, "force", "f", false,
		"Force the removal of one or more volumes.")
	cmd.Flags().StringSliceVarP(&opts.machines, "machine", "m", nil,
		"Name or ID of the machine to remove one or more volumes from. "+
			"Can be specified multiple times or as a comma-separated list.\n"+
			"If not specified, the found volume(s) will be removed from all machines.")
	cmd.Flags().BoolVarP(&opts.yes, "yes", "y", false,
		"Do not prompt for confirmation before removing the volume(s).")

	return cmd
}

func remove(ctx context.Context, uncli *cli.CLI, names []string, opts removeOptions) error {
	client, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer client.Close()

	filter := &api.VolumeFilter{
		Names: names,
	}

	if len(opts.machines) > 0 {
		machines := cli.ExpandCommaSeparatedValues(opts.machines)
		filter.Machines = machines
	}

	volumes, err := client.ListVolumes(ctx, filter)
	if err != nil {
		return fmt.Errorf("list volumes: %w", err)
	}

	if len(volumes) == 0 {
		if len(names) == 1 {
			return fmt.Errorf("volume '%s' not found", names[0])
		}
		return fmt.Errorf("no volumes found matching the specified names")
	}

	// Confirm removal if not using --yes flag.
	if !opts.yes {
		fmt.Println("The following volumes will be removed:")
		for _, v := range volumes {
			fmt.Printf(" • '%s' on machine '%s'\n", v.Volume.Name, v.MachineName)
		}

		fmt.Println()
		confirmed, err := cli.Confirm()
		if err != nil {
			return fmt.Errorf("confirm removal: %w", err)
		}
		if !confirmed {
			fmt.Println("Cancelled. No volumes were removed.")
			return nil
		}
	}

	// Remove the volumes one by one collecting errors.
	var removeErr error
	for _, v := range volumes {
		if err = client.RemoveVolume(ctx, v.MachineID, v.Volume.Name, opts.force); err != nil {
			if !errors.Is(err, api.ErrNotFound) {
				removeErr = errors.Join(removeErr, fmt.Errorf("failed to remove volume '%s' on machine '%s': %w",
					v.Volume.Name, v.MachineName, err))
			}
			continue
		}

		fmt.Printf("Volume '%s' removed from machine '%s'.\n", v.Volume.Name, v.MachineName)
	}

	return removeErr
}

package image

import (
	"context"
	"errors"
	"fmt"

	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/cli/completion"
	"github.com/psviderski/uncloud/internal/cli/tui"
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
		Use:     "rm IMAGE [IMAGE...]",
		Aliases: []string{"remove", "delete"},
		Short:   "Remove one or more images.",
		Long:    "Remove one or more images. You cannot remove an image that is in use by a container.",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return remove(cmd.Context(), uncli, args, opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.force, "force", "f", false,
		"Force the removal of one or more images.")
	cmd.Flags().StringSliceVarP(&opts.machines, "machine", "m", nil,
		"Name or ID of the machine to remove one or more images from. "+
			"Can be specified multiple times or as a comma-separated list.\n"+
			"If not specified, the found images(s) will be removed from all machines.")
	cmd.Flags().BoolVarP(&opts.yes, "yes", "y", false,
		"Do not prompt for confirmation before removing the image(s).")

	completion.MachinesFlag(cmd)

	return cmd
}

func remove(ctx context.Context, uncli *cli.CLI, names []string, opts removeOptions) error {
	client, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer client.Close()

	filter := api.ImageFilter{}

	if len(opts.machines) > 0 {
		machines := cli.ExpandCommaSeparatedValues(opts.machines)
		filter.Machines = machines
	}

	images := []api.MachineImages{}
	for _, name := range names {
		filter.Name = name
		clusterImages, err := client.ListImages(ctx, filter)
		if err != nil {
			return fmt.Errorf("list images: %w", err)
		}
		for i := range clusterImages {
			if len(clusterImages[i].Images) > 0 {
				images = append(images, clusterImages[i])
			}
		}
	}

	if len(images) == 0 {
		return fmt.Errorf("no images found matching the specified names")
	}

	allMachines, err := client.ListMachines(ctx, nil)
	if err != nil {
		return fmt.Errorf("list machines: %w", err)
	}

	// Confirm removal if not using --yes flag.
	if !opts.yes {
		fmt.Println("The following images will be removed:")
		for _, machineImage := range images {
			// Get machine name for better readability.
			machineName := machineImage.Metadata.Machine
			if m := allMachines.FindByNameOrID(machineName); m != nil {
				machineName = m.Machine.Name
			}

			for _, img := range machineImage.Images {
				id := normalizeID(img.ID)
				name := normalizeName(img)

				fmt.Printf(" • '%s' ('%s') on machine '%s'\n", name, id, machineName)
			}
		}

		fmt.Println()
		confirmed, err := tui.Confirm("")
		if err != nil {
			return fmt.Errorf("confirm removal: %w", err)
		}
		if !confirmed {
			return cli.Cancelled("Cancelled. No images were removed.")
		}
	}

	var removeErr error
	for _, machineImage := range images {
		machineName := machineImage.Metadata.Machine
		if m := allMachines.FindByNameOrID(machineName); m != nil {
			machineName = m.Machine.Name
		}

		for _, img := range machineImage.Images {
			id := normalizeID(img.ID)
			name := normalizeName(img)

			if err := client.RemoveImage(ctx, machineImage.Metadata.Machine, img.ID, opts.force); err != nil {
				if !errors.Is(err, api.ErrNotFound) {
					removeErr = errors.Join(removeErr, fmt.Errorf("failed to remove image '%s' ('%s') on machine '%s': %w",
						name, id, machineName, err))
				}
				continue
			}

			fmt.Printf("Image '%s' ('%s') removed from machine '%s'.\n", name, id, machineName)
		}
	}

	return removeErr
}

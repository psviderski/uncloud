package image

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/psviderski/uncloud/internal/cli"
	"github.com/spf13/cobra"
)

func NewInspectCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inspect [IMAGE...]",
		Short: "Display detailed information on one or more images.",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return inspect(cmd.Context(), uncli, args)
		},
	}

	return cmd
}

func inspect(ctx context.Context, uncli *cli.CLI, images []string) error {
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

	for _, img := range images {
		// Try to inspect locally first (on cluster machines)
		machineImages, err := clusterClient.InspectImage(ctx, img)
		if err == nil {
			for _, mi := range machineImages {
				machineName := mi.Metadata.Machine
				if m := allMachines.FindByNameOrID(machineName); m != nil {
					machineName = m.Machine.Name
				}

				fmt.Printf("Machine: %s\n", machineName)
				if mi.Metadata.Error != "" {
					fmt.Printf("Error: %s\n", mi.Metadata.Error)
					continue
				}

				// Pretty print the image info
				b, err := json.MarshalIndent(mi.Image, "", "    ")
				if err != nil {
					fmt.Printf("Error marshaling image info: %v\n", err)
					continue
				}
				fmt.Println(string(b))
			}
			continue
		}

		// If not found on cluster, try remote inspect
		// TODO: differentiate between "not found" and other errors?
		// InspectRemoteImage checks remote registry.
		remoteImages, err := clusterClient.InspectRemoteImage(ctx, img)
		if err != nil {
			fmt.Printf("Error inspecting image '%s': %v\n", img, err)
			continue
		}

		for _, ri := range remoteImages {
			machineName := ri.Metadata.Machine
			if m := allMachines.FindByNameOrID(machineName); m != nil {
				machineName = m.Machine.Name
			}

			fmt.Printf("Machine (Remote Lookup): %s\n", machineName)
			if ri.Metadata.Error != "" {
				fmt.Printf("Error: %s\n", ri.Metadata.Error)
				continue
			}

			b, err := json.MarshalIndent(ri.Image, "", "    ")
			if err != nil {
				fmt.Printf("Error marshaling image info: %v\n", err)
				continue
			}
			fmt.Println(string(b))
		}
	}

	return nil
}

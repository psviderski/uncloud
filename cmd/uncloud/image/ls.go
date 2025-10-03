package image

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/docker/go-units"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/spf13/cobra"
)

type listOptions struct {
	machines []string
	context  string
}

func NewListCommand() *cobra.Command {
	opts := listOptions{}

	cmd := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "List images on machines in the cluster.",
		Long:    "List images on machines in the cluster. By default, on all machines.",
		Example: `  # List images on all machines.
  uc image ls

  # List images on specific machine.
  uc image ls -m machine1

  # List images on multiple machines.
  uc image ls -m machine1,machine2`,
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return list(cmd.Context(), uncli, opts)
		},
	}

	cmd.Flags().StringSliceVarP(&opts.machines, "machine", "m", nil,
		"Filter images by machine name or ID. Can be specified multiple times or as a comma-separated list. "+
			"(default is include all machines)")
	cmd.Flags().StringVarP(
		&opts.context, "context", "c", "",
		"Name of the cluster context. (default is the current context)",
	)

	return cmd
}

func list(ctx context.Context, uncli *cli.CLI, opts listOptions) error {
	clusterClient, err := uncli.ConnectCluster(ctx, opts.context)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer clusterClient.Close()

	// Get all machines to create ID to name mapping.
	allMachines, err := clusterClient.ListMachines(ctx, nil)
	if err != nil {
		return fmt.Errorf("list machines: %w", err)
	}

	machineIDToName := make(map[string]string)
	for _, machineMember := range allMachines {
		if machineMember.Machine != nil && machineMember.Machine.Id != "" && machineMember.Machine.Name != "" {
			machineIDToName[machineMember.Machine.Id] = machineMember.Machine.Name
		}
	}

	machines := cli.ExpandCommaSeparatedValues(opts.machines)

	clusterImages, err := clusterClient.ListImages(ctx, api.ImageFilter{Machines: machines})
	if err != nil {
		return fmt.Errorf("list images: %w", err)
	}

	// Check if there are any images across all machines.
	hasImages := false
	for _, machineImages := range clusterImages {
		if len(machineImages.Images) > 0 {
			hasImages = true
			break
		}
	}

	if !hasImages {
		fmt.Println("No images found.")
		return nil
	}

	// Replace machine IDs with names in metadata for better readability.
	for _, machineImages := range clusterImages {
		if m := allMachines.FindByNameOrID(machineImages.Metadata.Machine); m != nil {
			machineImages.Metadata.Machine = m.Machine.Name
		}
	}
	// Sort machines alphabetically by name.
	sort.Slice(clusterImages, func(i, j int) bool {
		return clusterImages[i].Metadata.Machine < clusterImages[j].Metadata.Machine
	})

	// Print the images in a table format.
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	if _, err = fmt.Fprintln(tw, "MACHINE\tIMAGE\tIMAGE ID\tCREATED\tSIZE\tSTORE"); err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	// Print rows for each machine's images.
	for _, machineImages := range clusterImages {
		store := "docker"
		if machineImages.ContainerdStore {
			store = "containerd"
		}

		// Print each image for this machine.
		for _, img := range machineImages.Images {
			imageName := "<none>"
			if len(img.RepoTags) > 0 && img.RepoTags[0] != "<none>:<none>" {
				imageName = img.RepoTags[0]
			}

			// Show the first 12 chars without 'sha256:' as the image ID like Docker does.
			imageID := strings.TrimPrefix(img.ID, "sha256:")[:12]

			created := ""
			createdAt := time.Unix(img.Created, 0)
			if !createdAt.IsZero() {
				created = units.HumanDuration(time.Now().UTC().Sub(createdAt)) + " ago"
			}

			size := units.HumanSizeWithPrecision(float64(img.Size), 3)

			if _, err = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
				machineImages.Metadata.Machine, imageName, imageID, created, size, store); err != nil {
				return fmt.Errorf("write row: %w", err)
			}
		}
	}

	return tw.Flush()
}

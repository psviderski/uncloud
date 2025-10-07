package image

import (
	"context"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/containerd/platforms"
	"github.com/docker/docker/api/types/image"
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

// imageRow represents a single image with its metadata for display.
type imageRow struct {
	id           string
	name         string
	platforms    string
	createdHuman string
	createdUnix  int64
	size         string
	store        string
	machine      string
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

	// Collect all images from all machines.
	var rows []imageRow

	for _, machineImages := range clusterImages {
		// Get machine name for better readability.
		machineName := machineImages.Metadata.Machine
		if m := allMachines.FindByNameOrID(machineName); m != nil {
			machineName = m.Machine.Name
		}

		store := "docker"
		if machineImages.ContainerdStore {
			store = "containerd"
		}

		// Process each image for this machine.
		for _, img := range machineImages.Images {
			// Show the first 12 chars without 'sha256:' as the image ID like Docker does.
			id := strings.TrimPrefix(img.ID, "sha256:")[:12]

			name := "<none>"
			if len(img.RepoTags) > 0 && img.RepoTags[0] != "<none>:<none>" {
				name = img.RepoTags[0]
			}

			imgPlatforms := imagePlatforms(img)
			formattedPlatforms := strings.Join(imgPlatforms, ",")

			created := ""
			createdAt := time.Unix(img.Created, 0)
			if !createdAt.IsZero() {
				created = units.HumanDuration(time.Now().UTC().Sub(createdAt)) + " ago"
			}

			size := units.HumanSizeWithPrecision(float64(img.Size), 3)

			rows = append(rows, imageRow{
				id:           id,
				name:         name,
				platforms:    formattedPlatforms,
				createdHuman: created,
				createdUnix:  img.Created,
				size:         size,
				store:        store,
				machine:      machineName,
			})
		}
	}

	if len(rows) == 0 {
		fmt.Println("No images found.")
		return nil
	}

	// Sort images by created time (newest first), then by machine name.
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].createdUnix != rows[j].createdUnix {
			return rows[i].createdUnix > rows[j].createdUnix
		}
		return rows[i].machine < rows[j].machine
	})

	// Print the images in a table format.
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	if _, err = fmt.Fprintln(tw, "IMAGE ID\tNAME\tPLATFORMS\tCREATED\tSIZE\tSTORE\tMACHINE"); err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	for _, img := range rows {
		if _, err = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			img.id, img.name, img.platforms, img.createdHuman, img.size, img.store, img.machine); err != nil {
			return fmt.Errorf("write row: %w", err)
		}
	}

	return tw.Flush()
}

func imagePlatforms(img image.Summary) []string {
	var formattedPlatforms []string

	for _, m := range img.Manifests {
		if m.Kind != image.ManifestKindImage || !m.Available {
			continue
		}
		formattedPlatforms = append(formattedPlatforms, platforms.Format(m.ImageData.Platform))
	}

	slices.Sort(formattedPlatforms)
	return formattedPlatforms
}

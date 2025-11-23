package image

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/containerd/platforms"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/go-units"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/cli/output"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/spf13/cobra"
)

type listOptions struct {
	machines   []string
	nameFilter string
	format     string
}

func NewListCommand() *cobra.Command {
	opts := listOptions{}

	cmd := &cobra.Command{
		Use:     "ls [REPO:[TAG]]",
		Aliases: []string{"list"},
		Short:   "List images on machines in the cluster.",
		Long:    "List images on machines in the cluster. By default, on all machines. Optionally filter by image name.",
		Example: `  # List all images on all machines.
  uc image ls

  # List images on specific machine.
  uc image ls -m machine1

  # List images on multiple machines.
  uc image ls -m machine1,machine2

  # List images filtered by name (with any tag) on all machines.
  uc image ls myapp

  # List images filtered by name pattern on specific machine.
  uc image ls "myapp:1.*" -m machine1

  # Output in JSON format
  uc image ls --format json`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.nameFilter = args[0]
			}
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return list(cmd.Context(), uncli, opts)
		},
	}

	cmd.Flags().StringSliceVarP(&opts.machines, "machine", "m", nil,
		"Filter images by machine name or ID. Can be specified multiple times or as a comma-separated list. "+
			"(default is include all machines)")
	cmd.Flags().StringVar(&opts.format, "format", "table", "Output format (table, json)")

	return cmd
}

// imageItem represents a single image with its metadata.
type imageItem struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Platforms []string `json:"platforms"`
	Created   int64    `json:"created"`
	Size      int64    `json:"size"`
	// InUse indicates if the image is used by a container. -1 means unknown.
	InUse   int64  `json:"inUse"`
	Store   string `json:"store"`
	Machine string `json:"machine"`
}

func list(ctx context.Context, uncli *cli.CLI, opts listOptions) error {
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

	machineIDToName := make(map[string]string)
	for _, machineMember := range allMachines {
		if machineMember.Machine != nil && machineMember.Machine.Id != "" && machineMember.Machine.Name != "" {
			machineIDToName[machineMember.Machine.Id] = machineMember.Machine.Name
		}
	}

	machines := cli.ExpandCommaSeparatedValues(opts.machines)

	clusterImages, err := clusterClient.ListImages(ctx, api.ImageFilter{
		Machines: machines,
		Name:     opts.nameFilter,
	})
	if err != nil {
		return fmt.Errorf("list images: %w", err)
	}

	// Collect all images from all machines.
	var items []imageItem

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

			imgPlatforms, _ := imagePlatforms(img)

			items = append(items, imageItem{
				ID:        id,
				Name:      name,
				Platforms: imgPlatforms,
				Created:   img.Created,
				Size:      img.Size,
				InUse:     img.Containers,
				Store:     store,
				Machine:   machineName,
			})
		}
	}

	if len(items) == 0 {
		if opts.format == "json" {
			fmt.Println("[]")
			return nil
		}
		if opts.nameFilter != "" {
			fmt.Printf("No images matching '%s' found.\n", opts.nameFilter)
		} else {
			fmt.Println("No images found.")
		}
		return nil
	}

	// Sort images by name, then by machine name.
	sort.Slice(items, func(i, j int) bool {
		if items[i].Name != items[j].Name {
			return items[i].Name < items[j].Name
		}
		return items[i].Machine < items[j].Machine
	})

	// Define columns.
	columns := []output.Column[imageItem]{
		{
			Header: "IMAGE ID",
			Field:  "ID",
		},
		{
			Header: "NAME",
			Field:  "Name",
		},
		{
			Header: "PLATFORMS",
			Accessor: func(item imageItem) string {
				if len(item.Platforms) == 0 {
					return "-"
				}
				style := output.PillStyle()
				styled := make([]string, len(item.Platforms))
				for i, p := range item.Platforms {
					styled[i] = style.Render(p)
				}
				return strings.Join(styled, " ")
			},
		},
		{
			Header: "CREATED",
			Accessor: func(item imageItem) string {
				createdAt := time.Unix(item.Created, 0)
				if createdAt.IsZero() {
					return ""
				}
				return units.HumanDuration(time.Now().UTC().Sub(createdAt)) + " ago"
			},
		},
		{
			Header: "SIZE",
			Accessor: func(item imageItem) string {
				return units.HumanSizeWithPrecision(float64(item.Size), 3)
			},
		},
	}

	// Add IN USE column if available.
	inUseAvailable := slices.ContainsFunc(items, func(i imageItem) bool {
		return i.InUse != -1
	})

	if inUseAvailable {
		columns = append(columns, output.Column[imageItem]{
			Header: "IN USE",
			Accessor: func(item imageItem) string {
				if item.InUse == -1 {
					return "-"
				}
				if item.InUse > 0 {
					return lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render("●")
				}
				return lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("○")
			},
		})
	}

	columns = append(columns,
		output.Column[imageItem]{
			Header: "STORE",
			Field:  "Store",
		},
		output.Column[imageItem]{
			Header: "MACHINE",
			Field:  "Machine",
		},
	)

	return output.Print(items, columns, opts.format)
}

// imagePlatforms returns a list of platforms supported by the image and a boolean indicating if it's multi-platform.
func imagePlatforms(img image.Summary) ([]string, bool) {
	var formattedPlatforms []string
	multiPlatform := false

	for _, m := range img.Manifests {
		if m.Kind != image.ManifestKindImage || !m.Available {
			continue
		}

		if m.ID != img.ID {
			// There is an image manifest that has digest different from the main image digest.
			// This means the image manifest is an index or a manifest list (multi-platform image).
			multiPlatform = true
		}
		formattedPlatforms = append(formattedPlatforms, platforms.Format(m.ImageData.Platform))
	}

	slices.Sort(formattedPlatforms)

	return formattedPlatforms, multiPlatform
}

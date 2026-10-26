package image

import (
	"context"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/containerd/platforms"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/pkg/stringid"
	"github.com/docker/go-units"

	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/cli/completion"
	"github.com/psviderski/uncloud/internal/cli/tui"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/spf13/cobra"
)

type listOptions struct {
	machines   []string
	nameFilter string
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
  uc image ls "myapp:1.*" -m machine1`,
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

	completion.MachinesFlag(cmd)

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
	inUse        string
	store        string
	machine      string
}

func list(ctx context.Context, uncli *cli.CLI, opts listOptions) error {
	clusterClient, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer clusterClient.Close()

	machines := cli.ExpandCommaSeparatedValues(opts.machines)

	clusterImages, err := clusterClient.ListImages(ctx, api.ImageFilter{
		Machines: machines,
		Name:     opts.nameFilter,
	})
	if err != nil {
		return fmt.Errorf("list images: %w", err)
	}

	// Collect all images from all machines.
	var rows []imageRow

	for _, machineImages := range clusterImages {
		if err := machineImages.Error(); err != nil {
			tui.PrintWarning(fmt.Sprintf("failed to list images on machine '%s': %s",
				machineImages.Metadata.MachineName, err))
			continue
		}

		// Get machine name for better readability.
		machineName := machineImages.Metadata.MachineName

		store := "docker"
		if machineImages.ContainerdStore {
			store = "containerd"
		}

		rows = append(rows, imageRows(machineImages.Images, machineName, store)...)
	}

	if len(rows) == 0 {
		if opts.nameFilter != "" {
			fmt.Printf("No images matching '%s' found.\n", opts.nameFilter)
		} else {
			fmt.Println("No images found.")
		}
		return nil
	}

	// Sort images by name, then by machine name.
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].name != rows[j].name {
			return rows[i].name < rows[j].name
		}
		return rows[i].machine < rows[j].machine
	})

	// Print the images in a table format.
	lipgloss.Println(formatImageTable(rows))

	return nil
}

// imageRows keeps every tag visible and retains digest-only and unnamed images.
func imageRows(images []image.Summary, machineName, store string) []imageRow {
	var rows []imageRow
	for _, img := range images {
		imageRefs := api.ImageReferences(img)
		var imageNames []string
		for _, ref := range imageRefs {
			if !strings.Contains(ref, "@") {
				imageNames = append(imageNames, ref)
			}
		}
		if len(imageNames) == 0 {
			imageNames = imageRefs
		}
		if len(imageNames) == 0 {
			imageNames = []string{"<none>"}
		}

		imgPlatforms, _ := imagePlatforms(img)
		created := ""
		if img.Created > 0 {
			created = units.HumanDuration(time.Since(time.Unix(img.Created, 0))) + " ago"
		}
		// Check if the image is in use by any containers. Only supported by Docker API >=1.51
		inUse := "-"
		if img.Containers >= 0 { // -1 means the info is not available.
			inUse = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("○")
			if img.Containers > 0 {
				inUse = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render("●")
			}
		}

		for _, name := range imageNames {
			rows = append(rows, imageRow{
				id:           stringid.TruncateID(img.ID),
				name:         name,
				platforms:    formatPlatforms(imgPlatforms),
				createdHuman: created,
				createdUnix:  img.Created,
				size:         units.HumanSizeWithPrecision(float64(img.Size), 3),
				inUse:        inUse,
				store:        store,
				machine:      machineName,
			})
		}
	}

	return rows
}

// imagePlatforms returns a list of platforms supported by the image and a boolean indicating if it's multi-platform.
func imagePlatforms(img image.Summary) ([]string, bool) {
	var formattedPlatforms []string
	multiPlatform := false

	for _, m := range img.Manifests {
		if m.Kind != image.ManifestKindImage || !m.Available || m.ImageData == nil {
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

func formatPlatforms(platforms []string) string {
	if len(platforms) == 0 {
		return "-"
	}

	platformStyle := lipgloss.NewStyle().
		BorderForeground(lipgloss.Color("152")).
		Foreground(lipgloss.Color("0")).
		Background(lipgloss.Color("152"))
	// Use fancy pill borders only if the output is a terminal with color support.
	if colorprofile.Detect(os.Stdout, os.Environ()) > colorprofile.ASCII {
		platformStyle = platformStyle.Border(lipgloss.Border{Left: "", Right: ""}, false, true, false, true)
	}

	styledPlatforms := make([]string, len(platforms))
	for i, p := range platforms {
		styledPlatforms[i] = platformStyle.Render(p)
	}

	return strings.Join(styledPlatforms, " ")
}

func formatImageTable(rows []imageRow) string {
	columns := []struct {
		name string
		hide bool
	}{
		{name: "IMAGE ID"},
		{name: "NAME"},
		{name: "PLATFORMS"},
		{name: "CREATED"},
		{name: "SIZE"},
		{name: "IN USE"},
		{name: "STORE"},
		{name: "MACHINE"},
	}

	// Hide the "IN USE" column if none of the images have that info available.
	inUseInfoAvailable := slices.ContainsFunc(rows, func(r imageRow) bool {
		return r.inUse != "-"
	})
	if !inUseInfoAvailable {
		// Hide "IN USE" column.
		columns[5].hide = true
	}

	// Hide the "STORE" column if all machines use the containerd store.
	hasNonContainerd := slices.ContainsFunc(rows, func(r imageRow) bool {
		return r.store != "containerd"
	})
	if !hasNonContainerd {
		columns[6].hide = true
	}

	t := tui.NewTable()

	var headers []string
	for _, col := range columns {
		if !col.hide {
			headers = append(headers, col.name)
		}
	}
	t.Headers(headers...)

	for _, row := range rows {
		values := []string{
			row.id,
			tui.FormatImage(row.name, tui.NoStyle),
			row.platforms,
			row.createdHuman,
			row.size,
			row.inUse,
			row.store,
			row.machine,
		}
		var filteredValues []string
		for i, v := range values {
			if !columns[i].hide {
				filteredValues = append(filteredValues, v)
			}
		}
		t.Row(filteredValues...)
	}

	return t.String()
}

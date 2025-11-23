package volume

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/cli/output"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/spf13/cobra"
)

type listOptions struct {
	machines []string
	quiet    bool
	format   string
}

func NewListCommand() *cobra.Command {
	opts := listOptions{}

	cmd := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "List volumes across all machines in the cluster.",
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return list(cmd.Context(), uncli, opts)
		},
	}

	cmd.Flags().StringSliceVarP(&opts.machines, "machine", "m", nil,
		"Filter volumes by machine name or ID. Can be specified multiple times or as a comma-separated list. "+
			"(default is include all machines)")
	cmd.Flags().BoolVarP(&opts.quiet, "quiet", "q", false,
		"Only display volume names.")
	cmd.Flags().StringVar(&opts.format, "format", "table", "Output format (table, json)")

	return cmd
}

type volumeItem struct {
	Name    string `json:"name"`
	Driver  string `json:"driver"`
	Machine string `json:"machine"`
}

func list(ctx context.Context, uncli *cli.CLI, opts listOptions) error {
	client, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer client.Close()

	// Apply machine filter if specified.
	var filter *api.VolumeFilter
	if len(opts.machines) > 0 {
		machines := cli.ExpandCommaSeparatedValues(opts.machines)
		filter = &api.VolumeFilter{
			Machines: machines,
		}
	}

	volumes, err := client.ListVolumes(ctx, filter)
	if err != nil {
		return fmt.Errorf("list volumes: %w", err)
	}

	if len(volumes) == 0 {
		if opts.format == "json" {
			fmt.Println("[]")
			return nil
		}
		if !opts.quiet {
			fmt.Println("No volumes found.")
		}
		return nil
	}

	// Sort the volumes by name first, then by machine name.
	slices.SortFunc(volumes, func(a, b api.MachineVolume) int {
		cmp := strings.Compare(a.Volume.Name, b.Volume.Name)
		if cmp != 0 {
			return cmp
		}
		return strings.Compare(a.MachineName, b.MachineName)
	})

	// If quiet mode, just print volume names.
	if opts.quiet {
		// Quiet mode usually implies text output of just names, regardless of json format flag unless explicitly documented otherwise.
		// If both are present, standard convention varies. Docker ignores format if quiet is set?
		// Docker: `docker volume ls -q --format json` -> prints raw names.
		// So quiet takes precedence or they are mutually exclusive.
		for _, v := range volumes {
			fmt.Println(v.Volume.Name)
		}
		return nil
	}

	var items []volumeItem
	for _, v := range volumes {
		items = append(items, volumeItem{
			Name:    v.Volume.Name,
			Driver:  v.Volume.Driver,
			Machine: v.MachineName,
		})
	}

	columns := []output.Column[volumeItem]{
		{Header: "NAME", Field: "Name"},
		{Header: "DRIVER", Field: "Driver"},
		{Header: "MACHINE", Field: "Machine"},
	}

	return output.Print(items, columns, opts.format)
}

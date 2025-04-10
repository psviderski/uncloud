package volume

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/spf13/cobra"
)

type listOptions struct {
	machines []string
	quiet    bool
	context  string
}

func NewListCommand() *cobra.Command {
	opts := listOptions{}

	cmd := &cobra.Command{
		Use:     "ls [FLAGS]",
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

	cmd.Flags().StringVarP(&opts.context, "context", "c", "",
		"Name of the cluster context. (default is the current context)")

	return cmd
}

func list(ctx context.Context, uncli *cli.CLI, opts listOptions) error {
	client, err := uncli.ConnectCluster(ctx, opts.context)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer client.Close()

	// Apply machine filter if specified.
	var filter *api.VolumeFilter
	if len(opts.machines) > 0 {
		// Expand comma-separated machine names.
		var machines []string
		for _, m := range opts.machines {
			for _, nameOrID := range strings.Split(m, ",") {
				if nameOrID = strings.TrimSpace(nameOrID); nameOrID != "" {
					machines = append(machines, nameOrID)
				}
			}
		}

		filter = &api.VolumeFilter{
			Machines: machines,
		}
	}

	volumes, err := client.ListVolumes(ctx, filter)
	if err != nil {
		return fmt.Errorf("list volumes: %w", err)
	}

	if len(volumes) == 0 {
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
		for _, v := range volumes {
			fmt.Println(v.Volume.Name)
		}
		return nil
	}

	// Print the volumes in a table format.
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(tw, "NAME\tDRIVER\tMACHINE")

	for _, v := range volumes {
		fmt.Fprintf(tw, "%s\t%s\t%s\n",
			v.Volume.Name,
			v.Volume.Driver,
			v.MachineName,
		)
	}

	return tw.Flush()
}

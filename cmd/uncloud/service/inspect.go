package service

import (
	"context"
	"fmt"
	"os"
	"slices"
	"text/tabwriter"
	"time"

	"github.com/docker/docker/pkg/stringid"
	"github.com/docker/go-units"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/spf13/cobra"
)

type inspectOptions struct {
	service string
}

func NewInspectCommand(groupID string) *cobra.Command {
	opts := inspectOptions{}
	cmd := &cobra.Command{
		Use:   "inspect SERVICE",
		Short: "Display detailed information on a service.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			opts.service = args[0]
			return inspect(cmd.Context(), uncli, opts)
		},
		GroupID: groupID,
	}
	return cmd
}

func inspect(ctx context.Context, uncli *cli.CLI, opts inspectOptions) error {
	client, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer client.Close()

	svc, err := client.InspectService(ctx, opts.service)
	if err != nil {
		return fmt.Errorf("inspect service: %w", err)
	}

	machines, err := client.ListMachines(ctx, nil)
	if err != nil {
		return fmt.Errorf("list machines: %w", err)
	}
	machinesNamesByID := make(map[string]string)
	for _, m := range machines {
		machinesNamesByID[m.Machine.Id] = m.Machine.Name
	}

	fmt.Printf("Service ID: %s\n", svc.ID)
	fmt.Printf("Name:       %s\n", svc.Name)
	fmt.Printf("Mode:       %s\n", svc.Mode)
	fmt.Println()

	// Parse created times for sorting and display.
	createdTimes := make(map[string]time.Time, len(svc.Containers))
	for _, ctr := range svc.Containers {
		createdTimes[ctr.Container.ID], _ = time.Parse(time.RFC3339Nano, ctr.Container.Created)
	}

	// Sort containers by created time (newest first).
	slices.SortFunc(svc.Containers, func(a, b api.MachineServiceContainer) int {
		return createdTimes[b.Container.ID].Compare(createdTimes[a.Container.ID])
	})

	// Print the list of containers in a table format.
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	if _, err = fmt.Fprintln(tw, "CONTAINER ID\tIMAGE\tCREATED\tSTATUS\tIP ADDRESS\tMACHINE"); err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	now := time.Now().UTC()
	for _, ctr := range svc.Containers {
		created := units.HumanDuration(now.Sub(createdTimes[ctr.Container.ID])) + " ago"

		machine := machinesNamesByID[ctr.MachineID]
		if machine == "" {
			machine = ctr.MachineID
		}
		state, err := ctr.Container.HumanState()
		if err != nil {
			return fmt.Errorf("get human state: %w", err)
		}

		ip := ctr.Container.UncloudNetworkIP()
		ipStr := ""
		// The container might not have an IP if it's not running or uses the host network.
		if ip.IsValid() {
			ipStr = ip.String()
		}

		_, err = fmt.Fprintf(
			tw,
			"%s\t%s\t%s\t%s\t%s\t%s\n",
			stringid.TruncateID(ctr.Container.ID),
			ctr.Container.Config.Image,
			created,
			state,
			ipStr,
			machine,
		)
		if err != nil {
			return fmt.Errorf("write row: %w", err)
		}
	}
	return tw.Flush()
}

package machine

import (
	"context"
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/psviderski/uncloud/internal/cli"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/emptypb"
)

func NewRTTCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rtt",
		Short: "Show round-trip times between machines.",
		Long: `Show round-trip times between machines.

Round-trip time statistics are collected from the Corrosion gossip protocol
and represent the average of recent RTT samples between each pair of machines
in the cluster. The values shown include the average RTT and standard deviation
for each machine-to-machine connection.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return rtt(cmd.Context(), uncli)
		},
	}
	return cmd
}

func rtt(ctx context.Context, uncli *cli.CLI) error {
	client, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer client.Close()

	// Setup context to proxy request to all machines.
	ctx, _, err = client.ProxyMachinesContext(ctx, nil)
	if err != nil {
		return fmt.Errorf("setup proxy context: %w", err)
	}

	resp, err := client.MachineClient.InspectMachine(ctx, &emptypb.Empty{})
	if err != nil {
		return fmt.Errorf("inspect machines: %w", err)
	}

	// Map machine IDs to names for display from the response.
	machineNames := make(map[string]string)
	for _, m := range resp.Machines {
		machineNames[m.Machine.Id] = m.Machine.Name
	}

	type row struct {
		machine string
		peer    string
		avg     float64
		stdDev  float64
	}
	var rows []row

	for _, m := range resp.Machines {
		// Unlikely to occur, but might be a possible edge case when
		// a machine is still initializing. So just to be safe...
		if m.Rtts == nil {
			continue
		}
		for peerID, stats := range m.Rtts {
			peerName := peerID
			if name, ok := machineNames[peerID]; ok {
				peerName = name
			}
			rows = append(rows, row{
				machine: m.Machine.Name,
				peer:    peerName,
				avg:     stats.Average,
				stdDev:  stats.StdDev,
			})
		}
	}

	// Sort by machine name then peer name
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].machine == rows[j].machine {
			return rows[i].peer < rows[j].peer
		}
		return rows[i].machine < rows[j].machine
	})

	// Print table
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(tw, "MACHINE\tPEER\tRTT")

	for _, r := range rows {
		fmt.Fprintf(tw, "%s\t%s\t%.1f ±%.1f\n", r.machine, r.peer, r.avg, r.stdDev)
	}
	return tw.Flush()
}

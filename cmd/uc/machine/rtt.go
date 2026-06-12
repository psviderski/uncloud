package machine

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/cli/tui"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/emptypb"
)

func NewRTTCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rtt",
		Short: "Show round-trip times between machines.",
		Long: `Show round-trip times between machines.

Round-trip time statistics are collected from the Corrosion gossip protocol
and represent the median of recent RTT samples between each pair of machines
in the cluster. The values shown include the median RTT and standard deviation
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
	ctx = client.ProxyMachinesContext(ctx, nil)

	resp, err := client.MachineClient.InspectMachine(ctx, &emptypb.Empty{})
	if err != nil {
		return fmt.Errorf("inspect machines: %w", err)
	}

	// Map machine IDs to names for display from the response.
	machineNames := make(map[string]string)
	for _, m := range resp.Machines {
		// NOTE: Metadata should never be nil in practice. This is legacy fallback that will be removed.
		if m.Metadata == nil {
			tui.PrintWarning("metadata is missing in response from unknown server")
			continue
		}
		if m.Metadata.Error != "" {
			tui.PrintWarning(fmt.Sprintf("failed to inspect machine '%s': %s", m.Metadata.MachineName, m.Metadata.Error))
			continue
		}
		if m.Machine == nil {
			continue
		}
		machineNames[m.Machine.Id] = m.Machine.Name
	}

	type row struct {
		machine string
		peer    string
		median  time.Duration
		stdDev  time.Duration
	}
	var rows []row

	for _, m := range resp.Machines {
		// Unlikely to occur, but might be a possible edge case when
		// a machine is still initializing. So just to be safe.
		if m.Machine == nil || m.Rtts == nil {
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
				median:  stats.Median.AsDuration(),
				stdDev:  stats.StdDev.AsDuration(),
			})
		}
	}

	// Sort by machine name then peer name.
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].machine == rows[j].machine {
			return rows[i].peer < rows[j].peer
		}
		return rows[i].machine < rows[j].machine
	})

	// Print table.
	t := tui.NewTable()
	t.Headers("MACHINE", "PEER", "MEDIAN", "STDDEV")

	for _, r := range rows {
		t.Row(r.machine, r.peer, tui.FormatRTT(r.median), formatRTTStdDev(r.stdDev))
	}

	fmt.Println(t)
	return nil
}

// formatRTTStdDev formats a round-trip time standard deviation with one decimal place, e.g. "±19.4ms".
func formatRTTStdDev(d time.Duration) string {
	return fmt.Sprintf("±%.1fms", float64(d)/float64(time.Millisecond))
}

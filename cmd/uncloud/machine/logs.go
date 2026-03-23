package machine

import (
	"context"
	"fmt"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/psviderski/uncloud/cmd/uncloud/internal/logs"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client"
	"github.com/spf13/cobra"
)

func NewLogsCommand(groupID string) *cobra.Command {
	var options logs.Options

	cmd := &cobra.Command{
		Use:     "logs [UNIT...]",
		Aliases: []string{"log"},
		Short:   "View systemd service logs.",
		Long: `View logs from all replicas of the specified units(s) (uncloud, docker or corrosion) across all machines in the cluster.

If no units are specified, streams logs from the uncloud unit.`,
		Example: `  # View recent logs for a system service.
  uc logs uncloud

  # Stream logs in real-time (follow mode).
  uc logs -f uncloud

  # View logs from multiple services.
  uc logs web uncloud docker

  # View logs from uncloud
  uc logs

  # Show last 20 lines per replica (default is 100).
  uc logs -n 20 docker

  # Show all logs without line limit.
  uc logs -n all docker

  # View logs from a specific time range.
  uc logs --since 3h --until 1h30m docker

  # View logs only from replicas running on specific machines.
  uc logs -m machine1,machine2 docker corrosion`,
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return runLogs(cmd.Context(), uncli, args, options)
		},
		GroupID: groupID,
	}

	cmd.Flags().AddFlagSet(logs.Flags(&options))
	return cmd
}

var validunits = map[string]struct{}{
	"uncloud":   {},
	"docker":    {},
	"corrosion": {},
}

func runLogs(ctx context.Context, uncli *cli.CLI, units []string, opts logs.Options) error {
	for _, unit := range units {
		if _, ok := validunits[unit]; !ok {
			return fmt.Errorf("invalid unit file '%s'", unit)
		}
	}

	tail, err := logs.Tail(opts.Tail)
	if err != nil {
		return err
	}

	c, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer c.Close()

	logsOpts := api.ServiceLogsOptions{
		Follow:   opts.Follow,
		Tail:     tail,
		Since:    opts.Since,
		Until:    opts.Until,
		Machines: cli.ExpandCommaSeparatedValues(opts.Machines),
	}

	// Collect log streams from all services.
	machineIDsSet := mapset.NewSet[string]()
	unitStreams := make([]<-chan api.ServiceLogEntry, 0, len(units))

	for _, unit := range units {
		// This is wrong because we're calling the wrong endpoints. Must be c.MachineLogs.
		svc, ch, err := c.ServiceLogs(ctx, unit, logsOpts)
		if err != nil {
			return fmt.Errorf("stream logs for systemd service '%s': %w", unit, err)
		}
		unitStreams = append(unitStreams, ch)

		machineIDs := svc.MachineIDs()
		machineIDsSet.Append(machineIDs...)
	}

	var stream <-chan api.ServiceLogEntry
	if len(units) == 1 {
		stream = unitStreams[0]
	} else {
		// Merge all service streams into a single sorted stream without stall detection as its handled per-service.
		merger := client.NewLogMerger(unitStreams, client.LogMergerOptions{})
		stream = merger.Stream()
	}

	// Fetch machine names for all machines (machineIDsSet) service containers are running on.
	machines, err := c.ListMachines(ctx, &api.MachineFilter{NamesOrIDs: machineIDsSet.ToSlice()})
	if err != nil {
		return fmt.Errorf("list machines: %w", err)
	}
	machineNames := make([]string, 0, len(machines))
	for _, m := range machines {
		machineNames = append(machineNames, m.Machine.Name)
	}

	formatter := logs.NewFormatter(machineNames, units, opts.UTC)

	// Print merged logs.
	for entry := range stream {
		if entry.Err != nil {
			formatter.PrintError(entry)
			continue
		}
		formatter.PrintEntry(entry)
	}

	return nil
}

package machine

import (
	"context"
	"fmt"

	"github.com/psviderski/uncloud/cmd/uncloud/internal/logs"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/journal"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client"
	"github.com/spf13/cobra"
)

func NewLogsCommand() *cobra.Command {
	var options logs.Options

	cmd := &cobra.Command{
		Use:     "logs [UNIT...]",
		Aliases: []string{"log"},
		Short:   "View systemd service logs.",
		Long: `View logs from all replicas of the specified units(s) (uncloud, docker or uncloud-corrosion) across all machines in the cluster.

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
	}

	cmd.Flags().AddFlagSet(logs.Flags(&options))
	return cmd
}

func runLogs(ctx context.Context, uncli *cli.CLI, units []string, opts logs.Options) error {
	if len(units) == 0 {
		units = []string{journal.UnitUncloud}
	}

	for _, unit := range units {
		if !journal.ValidUnit(unit) {
			return fmt.Errorf("invalid unit '%s'", unit)
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

	// Fetch machine names for all machines we want the unit logs from.
	machines, err := c.ListMachines(ctx, &api.MachineFilter{NamesOrIDs: opts.Machines})
	if err != nil {
		return fmt.Errorf("list machines: %w", err)
	}
	machineNames := make([]string, 0, len(machines))
	for _, m := range machines {
		machineNames = append(machineNames, m.Machine.Name)
	}

	// Collect log streams from the units on the machine machines.
	unitStreams := make([]<-chan api.ServiceLogEntry, 0, len(units)+len(machines))

	for _, machine := range machineNames {
		for _, unit := range units {
			ch, err := c.MachineLogs(ctx, machine, unit, logsOpts)
			if err != nil {
				return fmt.Errorf("stream logs for systemd service '%s': %w", unit, err)
			}
			unitStreams = append(unitStreams, ch)
		}
	}

	var stream <-chan api.ServiceLogEntry
	if len(units) == 1 {
		stream = unitStreams[0]
	} else {
		merger := client.NewLogMerger(unitStreams, client.LogMergerOptions{})
		stream = merger.Stream()
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

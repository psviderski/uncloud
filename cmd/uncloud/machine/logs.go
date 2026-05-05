package machine

import (
	"context"
	"fmt"

	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/cli/logs"
	"github.com/psviderski/uncloud/internal/journal"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client"
	"github.com/spf13/cobra"
)

func NewLogsCommand() *cobra.Command {
	var options logs.Options

	cmd := &cobra.Command{
		Use:     "logs [SERVICE...]",
		Aliases: []string{"log"},
		Short:   "View systemd service logs.",
		Long: `View logs from the specified systemd service(s) across all machines in the cluster.
Use -m to restrict to specific machines.

Supported services:
  uncloud            the Uncloud daemon
  docker             the Docker daemon
  uncloud-corrosion  the Corrosion distributed state store

If no services are specified, streams logs from the uncloud service.`,
		Example: `  # View recent logs for the uncloud service.
  uc machine logs
  uc machine logs uncloud

  # Stream logs in real-time (follow mode).
  uc machine logs -f uncloud

  # View logs from multiple services.
  uc machine logs uncloud docker uncloud-corrosion

  # Show last 20 lines per machine (default is 100).
  uc machine logs -n 20 docker

  # Show all logs without line limit.
  uc machine logs -n all docker

  # View logs from a specific time range.
  uc machine logs --since 3h --until 1h30m docker

  # View logs only from specific machines.
  uc machine logs -m machine1,machine2 uncloud uncloud-corrosion`,
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
			return fmt.Errorf("invalid systemd service '%s'", unit)
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

	snapshot, err := c.NewClusterSnapshot(ctx, client.ClusterSnapshotOptions{
		Machines: true,
	})
	if err != nil {
		return fmt.Errorf("load cluster snapshot: %w", err)
	}

	machineSource := snapshot.Machines
	if len(logsOpts.Machines) > 0 {
		machineSource = make(api.MachineMembersList, 0, len(logsOpts.Machines))
		for _, nameOrID := range logsOpts.Machines {
			m := snapshot.FindMachineByNameOrID(nameOrID)
			if m == nil {
				return fmt.Errorf("machine not found: %s", nameOrID)
			}
			machineSource = append(machineSource, m)
		}
	}
	machineNames := make([]string, 0, len(machineSource))
	for _, m := range machineSource {
		machineNames = append(machineNames, m.Machine.Name)
	}

	// Collect one log stream per unit. MachineLogs merges across machines internally.
	unitStreams := make([]<-chan api.ServiceLogEntry, 0, len(units))
	for _, unit := range units {
		ch, err := c.MachineLogsWithSnapshot(ctx, snapshot, unit, logsOpts)
		if err != nil {
			return fmt.Errorf("stream logs for systemd service '%s': %w", unit, err)
		}
		unitStreams = append(unitStreams, ch)
	}

	var stream <-chan api.ServiceLogEntry
	if len(unitStreams) == 1 {
		stream = unitStreams[0]
	} else {
		// Each MachineLogs stream already runs its own inner merger with stall detection,
		// so the outer merger across units skips it to avoid duplicate warnings.
		merger := client.NewLogMerger(unitStreams, client.LogMergerOptions{})
		stream = merger.Stream()
	}

	formatter := logs.NewFormatter(machineNames, units, opts.UTC)

	// Print merged logs.
	for entry := range stream {
		formatter.PrintEntry(entry)
	}

	return nil
}

package machine

import (
	"context"
	"fmt"
	"strconv"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/psviderski/uncloud/cmd/uncloud/internal/logs"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/spf13/cobra"
)

func NewLogsCommand(groupID string) *cobra.Command {
	var options logs.Options

	cmd := &cobra.Command{
		Use:     "logs [UNIT]",
		Aliases: []string{"log"},
		Short:   "View deamon logs.",
		Long: `View logs from all replicas of the specified unit across all machines in the cluster.

The allowed units are 'uncloud', 'docker' or 'corrosion'. If none are specified 'uncloud' is assumed.`,
		Example: `  # View recent logs for the uncloud daemon.
  uc machine logs uncloud

  # Stream logs in real-time (follow mode), using the default unit (=uncloud).
  uc machine logs -f

  # Show last 20 lines of docker logs (default is 100).
  uc machine logs -n 20 docker

  # Show all logs without line limit of corrosion
  uc machine logs -n all corrosion

  # View logs only from replicas running on specific machines.
  uc machine logs -m machine1,machine2 uncloud`,
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return runLogs(cmd.Context(), uncli, args, options)
		},
		GroupID: groupID,
	}

	cmd.Flags().AddFlagSet(logs.Flags(&options))

	return cmd
}

func runLogs(ctx context.Context, uncli *cli.CLI, units []string, opts logs.Options) error {
	// units... we only allow 1...
	unit := int32(pb.MachineLogsRequest_UNCLOUD)
	if len(units) > 0 {
		unit, ok := pb.MachineLogsRequest_Unit_value[units[0]]
		if !ok {
			return fmt.Errorf("invalid unit: '%s'", unit)
		}
	}

	tail := -1
	if opts.Tail != "all" {
		tailInt, err := strconv.Atoi(opts.Tail)
		if err != nil {
			return fmt.Errorf("invalid --tail value '%s': %w", opts.Tail, err)
		}
		tail = tailInt
	}

	c, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer c.Close()

	logsOpts := api.MachineLogsOptions{
		Follow:   opts.Follow,
		Tail:     tail,
		Since:    opts.Since,
		Until:    opts.Until,
		Machines: cli.ExpandCommaSeparatedValues(opts.Machines),
	}

	// Collect log streams from the unit.
	machineIDsSet := mapset.NewSet[string]()
	stream, err := c.MachineLogs(ctx, unit, logsOpts)
	if err != nil {
		return fmt.Errorf("stream logs for unit '%s': %w", unit, err)
	}
	//		machineIDs := svc.MachineIDs()
	//		machineIDsSet.Append(machineIDs...)

	// Fetch machine names for all machines (machineIDsSet) units are running on.
	machines, err := c.ListMachines(ctx, &api.MachineFilter{NamesOrIDs: machineIDsSet.ToSlice()})
	if err != nil {
		return fmt.Errorf("list machines: %w", err)
	}
	machineNames := make([]string, 0, len(machines))
	for _, m := range machines {
		machineNames = append(machineNames, m.Machine.Name)
	}

	formatter := logs.NewFormatter(machineNames, pb.MachineLogsRequest_Unit_name[unit], opts.UTC)

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

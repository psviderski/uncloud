package machine

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/cli/completion"
	"github.com/psviderski/uncloud/internal/cli/logs"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client"
	"github.com/spf13/cobra"
)

func NewLogsCommand() *cobra.Command {
	var options logs.Options

	cmd := &cobra.Command{
		Use:     "logs [SERVICE...]",
		Aliases: []string{"log"},
		Short:   "View system service logs.",
		Long: `View logs from the specified system service(s) across all machines in the cluster.
Use -m to restrict to specific machines.

Supported services:
  corrosion  the Corrosion distributed state store
  docker     the Docker daemon
  uncloud    the Uncloud daemon

If no services are specified, streams logs from the uncloud service.`,
		Example: `  # View recent logs for the uncloud service.
  uc machine logs
  uc machine logs uncloud

  # Stream logs in real-time (follow mode).
  uc machine logs -f uncloud

  # View logs from multiple services.
  uc machine logs uncloud docker corrosion

  # Show last 20 lines per machine (default is 100).
  uc machine logs -n 20 docker

  # Show all logs without line limit.
  uc machine logs -n all docker

  # View logs from a specific time range.
  uc machine logs --since 3h --until 1h30m docker

  # View logs only from specific machines.
  uc machine logs -m machine1,machine2 uncloud corrosion`,
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return runLogs(cmd.Context(), uncli, args, options)
		},
	}

	cmd.Flags().AddFlagSet(logs.Flags(&options))
	completion.MachinesFlag(cmd)

	return cmd
}

func runLogs(ctx context.Context, uncli *cli.CLI, services []string, opts logs.Options) error {
	if len(services) == 0 {
		services = []string{api.SystemServiceUncloud}
	}
	for _, service := range services {
		if !slices.Contains(api.SystemServices, service) {
			return fmt.Errorf("invalid system service '%s'; valid services: %s",
				service, strings.Join(api.SystemServices, ", "))
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

	// Resolve machine records for the formatter's column width computation.
	machines, err := c.ListMachines(ctx, &api.MachineFilter{
		NamesOrIDs: logsOpts.Machines,
	})
	if err != nil {
		return fmt.Errorf("list machines: %w", err)
	}
	machineNames := make([]string, 0, len(machines))
	for _, m := range machines {
		machineNames = append(machineNames, m.Machine.Name)
	}

	// Collect one log stream per service. MachineLogs merges across machines internally.
	serviceStreams := make([]<-chan api.ServiceLogEntry, 0, len(services))
	for _, service := range services {
		ch, err := c.MachineLogs(ctx, service, logsOpts)
		if err != nil {
			return fmt.Errorf("stream logs for system service '%s': %w", service, err)
		}
		serviceStreams = append(serviceStreams, ch)
	}

	var stream <-chan api.ServiceLogEntry
	if len(serviceStreams) == 1 {
		stream = serviceStreams[0]
	} else {
		// Each MachineLogs stream already runs its own inner merger with stall detection,
		// so the outer merger across services skips it to avoid duplicate warnings.
		merger := client.NewLogMerger(serviceStreams, client.LogMergerOptions{})
		stream = merger.Stream()
	}

	formatter := logs.NewFormatter(machineNames, services, opts.UTC)

	// Print merged logs.
	for entry := range stream {
		formatter.PrintEntry(entry)
	}

	return nil
}

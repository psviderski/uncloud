package machine

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/psviderski/uncloud/cmd/uncloud/internal/logs"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client"
	"github.com/psviderski/uncloud/pkg/client/compose"
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

func runLogs(ctx context.Context, uncli *cli.CLI, serviceNames []string, opts logs.Options) error {
	// If no services specified, try to load them from the Compose file(s).
	fromCompose := false
	if len(serviceNames) == 0 {
		fromCompose = true
		project, err := compose.LoadProject(ctx, opts.Files)
		if err != nil {
			return fmt.Errorf("load Compose file(s): %w", err)
		}
		// View logs for all services, including disabled by inactive profiles.
		serviceNames = append(project.ServiceNames(), project.DisabledServiceNames()...)
		if len(serviceNames) == 0 {
			return errors.New("no services found in Compose file(s)")
		}
	}

	// Parse tail option.
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

	logsOpts := api.ServiceLogsOptions{
		Follow:   opts.Follow,
		Tail:     tail,
		Since:    opts.Since,
		Until:    opts.Until,
		Machines: cli.ExpandCommaSeparatedValues(opts.Machines),
	}

	// Collect log streams from all services. When service names come from a Compose file,
	// skip the ones that are not found in the cluster (they may have been removed or not deployed yet).
	machineIDsSet := mapset.NewSet[string]()
	svcStreams := make([]<-chan api.ServiceLogEntry, 0, len(serviceNames))
	var foundServices, notFoundServices []string

	for _, serviceName := range serviceNames {
		svc, ch, err := c.ServiceLogs(ctx, serviceName, logsOpts)
		if err != nil {
			if errors.Is(err, api.ErrNotFound) && fromCompose {
				notFoundServices = append(notFoundServices, serviceName)
				continue
			}
			return fmt.Errorf("stream logs for service '%s': %w", serviceName, err)
		}
		svcStreams = append(svcStreams, ch)
		foundServices = append(foundServices, serviceName)

		machineIDs := svc.MachineIDs()
		machineIDsSet.Append(machineIDs...)
	}

	if fromCompose {
		if len(foundServices) == 0 {
			return fmt.Errorf("stream logs for services defined in %s: no services found in the cluster",
				strings.Join(opts.Files, ", "))
		}
		serviceNames = foundServices

		for _, name := range notFoundServices {
			client.PrintWarning(fmt.Sprintf("service '%s' not found in the cluster, skipping", name))
		}
	}

	var stream <-chan api.ServiceLogEntry
	if len(serviceNames) == 1 {
		stream = svcStreams[0]
	} else {
		// Merge all service streams into a single sorted stream without stall detection as its handled per-service.
		merger := client.NewLogMerger(svcStreams, client.LogMergerOptions{})
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

	formatter := logs.NewFormatter(machineNames, serviceNames, opts.UTC)

	// Print merged logs.
	for entry := range stream {
		if entry.Err != nil {
			formatter.printError(entry)
			continue
		}
		formatter.printEntry(entry)
	}

	return nil
}

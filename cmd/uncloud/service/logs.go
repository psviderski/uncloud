package service

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	mapset "github.com/deckarep/golang-set/v2"
	"github.com/docker/docker/pkg/stringid"
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
		Use:     "logs [SERVICE...]",
		Aliases: []string{"log"},
		Short:   "View service logs.",
		Long: `View logs from all replicas of the specified service(s) across all machines in the cluster.

If no services are specified, streams logs from all services defined in the Compose file
(compose.yaml by default or the file(s) specified with --file).`,
		Example: `  # View recent logs for a service.
  uc logs web

  # Stream logs in real-time (follow mode).
  uc logs -f web

  # View logs from multiple services.
  uc logs web api db

  # View logs from all services in compose.yaml.
  uc logs

  # Show last 20 lines per replica (default is 100).
  uc logs -n 20 web

  # Show all logs without line limit.
  uc logs -n all web

  # View logs from a specific time range.
  uc logs --since 3h --until 1h30m web

  # View logs only from replicas running on specific machines.
  uc logs -m machine1,machine2 web api`,
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

	formatter := newLogFormatter(machineNames, serviceNames, opts.UTC)

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

// Available colors for machine/service differentiation.
var colorPalette = []color.Color{
	lipgloss.BrightGreen,
	lipgloss.BrightYellow,
	lipgloss.BrightBlue,
	lipgloss.BrightMagenta,
	lipgloss.BrightCyan,
	lipgloss.Green,
	lipgloss.Yellow,
	lipgloss.Blue,
	lipgloss.Magenta,
	lipgloss.Cyan,
}

// logFormatter handles formatting and printing of log entries with dynamic column alignment.
type logFormatter struct {
	machineNames []string
	serviceNames []string

	maxMachineWidth int
	maxServiceWidth int

	utc bool
}

func newLogFormatter(machineNames, serviceNames []string, utc bool) *logFormatter {
	slices.Sort(machineNames)
	slices.Sort(serviceNames)

	maxMachineWidth := 0
	for _, name := range machineNames {
		if len(name) > maxMachineWidth {
			maxMachineWidth = len(name)
		}
	}

	maxServiceWidth := 0
	for _, name := range serviceNames {
		if len(name) > maxServiceWidth {
			maxServiceWidth = len(name)
		}
	}

	return &logFormatter{
		machineNames:    machineNames,
		serviceNames:    serviceNames,
		maxMachineWidth: maxMachineWidth,
		maxServiceWidth: maxServiceWidth,
		utc:             utc,
	}
}

// formatTimestamp formats timestamp using local timezone or UTC if configured.
func (f *logFormatter) formatTimestamp(t time.Time) string {
	if f.utc {
		t = t.UTC()
	} else {
		t = t.In(time.Local)
	}
	dimStyle := lipgloss.NewStyle().Faint(true)

	return dimStyle.Render(t.Format(time.StampMilli))
}

func (f *logFormatter) formatMachine(name string) string {
	style := lipgloss.NewStyle().Bold(true).PaddingRight(f.maxMachineWidth - len(name))

	if len(f.serviceNames) == 1 {
		// Machine name is coloured for single-service logs.
		i := slices.Index(f.machineNames, name)
		if i == -1 {
			f.machineNames = append(f.machineNames, name)
			i = len(f.machineNames) - 1
		}

		style = style.Foreground(colorPalette[i%len(colorPalette)])
	}

	return style.Render(name)
}

func (f *logFormatter) formatServiceContainer(serviceName, containerID string) string {
	styleService := lipgloss.NewStyle().Bold(true).PaddingRight(f.maxServiceWidth - len(serviceName))
	styleContainer := lipgloss.NewStyle().Faint(true)

	if len(f.serviceNames) > 1 {
		// Service name is coloured for multi-service logs.
		i := slices.Index(f.serviceNames, serviceName)
		if i == -1 {
			f.serviceNames = append(f.serviceNames, serviceName)
			i = len(f.serviceNames) - 1
		}

		styleService = styleService.Foreground(colorPalette[i%len(colorPalette)])
	}

	return styleService.Render(serviceName) + styleContainer.Render("["+containerID[:5]+"]")
}

// printEntry prints a single log entry with proper formatting.
func (f *logFormatter) printEntry(entry api.ServiceLogEntry) {
	if entry.Stream != api.LogStreamStdout && entry.Stream != api.LogStreamStderr {
		return
	}

	var output strings.Builder

	// Timestamp
	output.WriteString(f.formatTimestamp(entry.Timestamp))
	output.WriteString(" ")

	// Machine name
	output.WriteString(f.formatMachine(entry.Metadata.MachineName))
	output.WriteString(" ")

	// Service[container_id]
	output.WriteString(f.formatServiceContainer(entry.Metadata.ServiceName, entry.Metadata.ContainerID))
	output.WriteString(" ")

	// Message
	output.Write(entry.Message)

	// Print to appropriate stream.
	if entry.Stream == api.LogStreamStderr {
		fmt.Fprint(os.Stderr, output.String())
	} else {
		fmt.Print(output.String())
	}
}

// printError prints an error entry (e.g., stalled stream warning).
func (f *logFormatter) printError(entry api.ServiceLogEntry) {
	if entry.Metadata.ContainerID != "" {
		msg := fmt.Sprintf("WARNING: log stream from %s[%s] on machine '%s'",
			entry.Metadata.ServiceName,
			stringid.TruncateID(entry.Metadata.ContainerID),
			entry.Metadata.MachineName)

		if errors.Is(entry.Err, api.ErrLogStreamStalled) {
			msg += " stopped responding"
		} else {
			msg += fmt.Sprintf(": %v", entry.Err)
		}

		style := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11")) // Bold bright yellow
		fmt.Fprintln(os.Stderr, style.Render(msg))
	} else {
		msg := fmt.Sprintf("ERROR: %v", entry.Err)
		style := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("9")) // Bold bright red
		fmt.Fprintln(os.Stderr, style.Render(msg))
	}
}

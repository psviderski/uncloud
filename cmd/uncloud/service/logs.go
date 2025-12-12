package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	mapset "github.com/deckarep/golang-set/v2"
	"github.com/docker/docker/pkg/stringid"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client"
	"github.com/psviderski/uncloud/pkg/client/compose"
	"github.com/spf13/cobra"
)

type logsOptions struct {
	files    []string
	follow   bool
	tail     string
	since    string
	until    string
	utc      bool
	machines []string
}

func NewLogsCommand(groupID string) *cobra.Command {
	var options logsOptions

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

	cmd.Flags().StringSliceVar(&options.files, "file", nil,
		"One or more Compose files to load service names from when no services are specified. (default compose.yaml)")
	cmd.Flags().BoolVarP(&options.follow, "follow", "f", false,
		"Continually stream new logs.")
	cmd.Flags().StringSliceVarP(&options.machines, "machine", "m", nil,
		"Filter logs by machine name or ID. Can be specified multiple times or as a comma-separated list.")
	cmd.Flags().StringVar(&options.since, "since", "",
		"Show logs generated on or after the given timestamp. Accepts relative duration, RFC 3339 date, or Unix timestamp.\n"+
			"Examples:\n"+
			"  --since 2m30s                      Relative duration (2 minutes 30 seconds ago)\n"+
			"  --since 1h                         Relative duration (1 hour ago)\n"+
			"  --since 2025-11-24                 RFC 3339 date only (midnight using local timezone)\n"+
			"  --since 2024-05-14T22:50:00        RFC 3339 date/time using local timezone\n"+
			"  --since 2024-01-31T10:30:00Z       RFC 3339 date/time in UTC\n"+
			"  --since 1763953966                 Unix timestamp (seconds since January 1, 1970)")
	cmd.Flags().StringVarP(&options.tail, "tail", "n", "100",
		"Show the most recent logs and limit the number of lines shown per replica. Use 'all' to show all logs.")
	cmd.Flags().StringVar(&options.until, "until", "",
		"Show logs generated before the given timestamp. Accepts relative duration, RFC 3339 date, or Unix timestamp.\n"+
			"See --since for examples.")
	cmd.Flags().BoolVar(&options.utc, "utc", false,
		"Print timestamps in UTC instead of local timezone.")

	return cmd
}

func runLogs(ctx context.Context, uncli *cli.CLI, serviceNames []string, opts logsOptions) error {
	// If no services specified, try to load them from the Compose file(s).
	if len(serviceNames) == 0 {
		project, err := compose.LoadProject(ctx, opts.files)
		if err != nil {
			return fmt.Errorf("load compose file(s): %w", err)
		}
		// View logs for all services, including disabled by inactive profiles.
		serviceNames = append(project.ServiceNames(), project.DisabledServiceNames()...)
		if len(serviceNames) == 0 {
			return errors.New("no services found in compose file(s)")
		}
	}

	// Parse tail option.
	tail := -1
	if opts.tail != "all" {
		tailInt, err := strconv.Atoi(opts.tail)
		if err != nil {
			return fmt.Errorf("invalid --tail value '%s': %w", opts.tail, err)
		}
		tail = tailInt
	}

	c, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer c.Close()

	logsOpts := api.ServiceLogsOptions{
		Follow:   opts.follow,
		Tail:     tail,
		Since:    opts.since,
		Until:    opts.until,
		Machines: cli.ExpandCommaSeparatedValues(opts.machines),
	}

	// Collect log streams from all services.
	machineIDsSet := mapset.NewSet[string]()
	svcStreams := make([]<-chan api.ServiceLogEntry, 0, len(serviceNames))
	for _, serviceName := range serviceNames {
		svc, ch, err := c.ServiceLogs(ctx, serviceName, logsOpts)
		if err != nil {
			return fmt.Errorf("stream logs for service '%s': %w", serviceName, err)
		}
		svcStreams = append(svcStreams, ch)

		machineIDs := svc.MachineIDs()
		machineIDsSet.Append(machineIDs...)
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

	formatter := newLogFormatter(machineNames, serviceNames, opts.utc)

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
var colorPalette = []lipgloss.Color{
	lipgloss.Color("10"), // Bright green
	lipgloss.Color("11"), // Bright yellow
	lipgloss.Color("12"), // Bright blue
	lipgloss.Color("13"), // Bright magenta
	lipgloss.Color("14"), // Bright cyan
	lipgloss.Color("2"),  // Green
	lipgloss.Color("3"),  // Yellow
	lipgloss.Color("4"),  // Blue
	lipgloss.Color("5"),  // Magenta
	lipgloss.Color("6"),  // Cyan
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

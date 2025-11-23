package logs

import (
	"container/heap"
	"context"
	"fmt"
	"hash/fnv"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/internal/machine/docker"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client"
	"github.com/spf13/cobra"
	"google.golang.org/grpc/metadata"
)

func NewRootCommand() *cobra.Command {
	var options logsOptions

	cmd := &cobra.Command{
		Use:     "logs SERVICE",
		Aliases: []string{"log"},
		Short:   "View service logs",
		Long: `View logs from all replicas of a service.

The logs command retrieves and displays logs from all running replicas
of the specified service across all machines in the cluster.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return streamLogs(cmd.Context(), uncli, args[0], options)
		},
	}

	cmd.Flags().StringVarP(&options.context, "context", "c", "",
		"Name of the cluster context. (default is the current context)")
	cmd.Flags().BoolVarP(&options.follow, "follow", "f", false,
		"Follow log output")
	cmd.Flags().Int64Var(&options.tail, "tail", -1,
		"Number of lines to show from the end of the logs")
	cmd.Flags().BoolVarP(&options.timestamps, "timestamps", "t", false,
		"Show timestamps")
	cmd.Flags().StringVar(&options.since, "since", "",
		"Show logs since timestamp or duration (42m for 42 minutes)")
	cmd.Flags().StringVar(&options.until, "until", "",
		"Show logs before a timestamp or duration (42m for 42 minutes)")
	cmd.Flags().BoolVar(&options.strictOrder, "strict-order", false,
		"Merge logs in strict chronological order (slower but accurate)")

	return cmd
}

type logsOptions struct {
	context     string
	follow      bool
	tail        int64
	timestamps  bool
	since       string
	until       string
	strictOrder bool
}

func streamLogs(ctx context.Context, uncli *cli.CLI, serviceName string, opts logsOptions) error {
	c, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer c.Close()

	// Get service info to find all replicas
	service, err := c.InspectService(ctx, serviceName)
	if err != nil {
		return fmt.Errorf("inspect service: %w", err)
	}

	if len(service.Containers) == 0 {
		return fmt.Errorf("no containers found for service %s", serviceName)
	}

	// Group containers by machine
	containersByMachine := make(map[string][]api.MachineServiceContainer)
	for _, container := range service.Containers {
		containersByMachine[container.MachineID] = append(containersByMachine[container.MachineID], container)
	}

	// Choose between fast mode and strict ordering mode
	if opts.strictOrder {
		return streamLogsOrdered(ctx, c, containersByMachine, serviceName, opts)
	}

	// use by default: logs are printed as they arrive
	return streamLogsFast(ctx, c, containersByMachine, serviceName, opts)
}

// streamLogsFast is the default fast mode that prints logs as they arrive
func streamLogsFast(ctx context.Context, client *client.Client, containersByMachine map[string][]api.MachineServiceContainer, serviceName string, opts logsOptions) error {
	logChan := make(chan logEntry, 100)
	errChan := make(chan error, len(containersByMachine))
	var wg sync.WaitGroup

	// Start streaming logs from each machine
	for machine, containers := range containersByMachine {
		wg.Add(1)
		go func(machine string, containers []api.MachineServiceContainer) {
			defer wg.Done()
			if err := streamMachineLogs(ctx, client, machine, containers, serviceName, opts, logChan); err != nil {
				errChan <- fmt.Errorf("stream logs from machine %s: %w", machine, err)
			}
		}(machine, containers)
	}

	go func() {
		wg.Wait()
		close(logChan)
		close(errChan)
	}()

	// Print logs as they arrive
	for {
		select {
		case log, ok := <-logChan:
			if !ok {
				// All streams completed
				return nil
			}
			printLogEntry(log, opts.timestamps)
		case err := <-errChan:
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

type logEntry struct {
	timestamp   time.Time
	machineID   string
	machineName string
	serviceName string
	replica     string
	stream      string // stdout or stderr
	message     string
}

// logEntryWithChannel wraps a log entry with its source channel for heap ordering
type logEntryWithChannel struct {
	entry   logEntry
	channel <-chan logEntry
	index   int // index in the heap
}

// logEntryHeap implements heap.Interface for ordering log entries by timestamp
type logEntryHeap []*logEntryWithChannel

func (h logEntryHeap) Len() int { return len(h) }

func (h logEntryHeap) Less(i, j int) bool {
	// Earlier timestamps come first
	return h[i].entry.timestamp.Before(h[j].entry.timestamp)
}

func (h logEntryHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *logEntryHeap) Push(x interface{}) {
	n := len(*h)
	item := x.(*logEntryWithChannel)
	item.index = n
	*h = append(*h, item)
}

func (h *logEntryHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil  // avoid memory leak
	item.index = -1 // for safety
	*h = old[0 : n-1]
	return item
}

// streamLogsOrdered implements strict chronological ordering using a min-heap
func streamLogsOrdered(ctx context.Context, client *client.Client, containersByMachine map[string][]api.MachineServiceContainer, serviceName string, opts logsOptions) error {
	// Create separate channels for each machine
	machineChannels := make(map[string]chan logEntry)
	errChan := make(chan error, len(containersByMachine))
	var wg sync.WaitGroup

	// Start streaming logs from each machine into separate channels
	for machine, containers := range containersByMachine {
		ch := make(chan logEntry, 100)
		machineChannels[machine] = ch

		wg.Add(1)
		go func(machine string, containers []api.MachineServiceContainer, ch chan<- logEntry) {
			defer wg.Done()
			defer close(ch)
			if err := streamMachineLogs(ctx, client, machine, containers, serviceName, opts, ch); err != nil {
				errChan <- fmt.Errorf("stream logs from machine %s: %w", machine, err)
			}
		}(machine, containers, ch)
	}

	go func() {
		wg.Wait()
		close(errChan)
	}()

	h := &logEntryHeap{}
	heap.Init(h)

	// Read initial entries from each channel
	for _, ch := range machineChannels {
		select {
		case entry, ok := <-ch:
			if ok {
				heap.Push(h, &logEntryWithChannel{
					entry:   entry,
					channel: ch,
				})
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// Process log entries in chronological order
	for h.Len() > 0 {
		select {
		case err := <-errChan:
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
			}
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		item := heap.Pop(h).(*logEntryWithChannel)
		printLogEntry(item.entry, opts.timestamps)

		// Try to read the next entry from the same channel
		select {
		case entry, ok := <-item.channel:
			if ok {
				heap.Push(h, &logEntryWithChannel{
					entry:   entry,
					channel: item.channel,
				})
			}
		case <-ctx.Done():
			return ctx.Err()
		default:
			select {
			case entry, ok := <-item.channel:
				if ok {
					heap.Push(h, &logEntryWithChannel{
						entry:   entry,
						channel: item.channel,
					})
				}
			default:
				// Channel is truly empty, don't re-add
			}
		}
	}

	// Drain any remaining errors
	for err := range errChan {
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		}
	}

	return nil
}

func streamMachineLogs(ctx context.Context, client *client.Client, machine string, containers []api.MachineServiceContainer, serviceName string, opts logsOptions, logChan chan<- logEntry) error {
	// Get machine info to proxy requests
	machineInfo, err := client.InspectMachine(ctx, machine)
	if err != nil {
		return fmt.Errorf("inspect machine %s: %w", machine, err)
	}

	// Create context that proxies to this specific machine
	machineCtx := proxyToMachine(ctx, machineInfo.Machine)

	// Stream logs from each container
	var wg sync.WaitGroup
	for _, container := range containers {
		wg.Add(1)
		go func(container api.MachineServiceContainer) {
			defer wg.Done()
			if err := streamContainerLogs(machineCtx, client, machineInfo.Machine.Id, machineInfo.Machine.Name,
				container, serviceName, opts, logChan); err != nil {
				// Log error but don't fail the whole operation
				fmt.Fprintf(os.Stderr, "Warning: failed to stream logs for container %s: %v\n", container.Container.ID,
					err)
			}
		}(container)
	}
	wg.Wait()
	return nil
}

func streamContainerLogs(ctx context.Context, client *client.Client, machineID string, machineName string, container api.MachineServiceContainer, serviceName string, opts logsOptions, logChan chan<- logEntry) error {
	dockerOpts := docker.ContainerLogsOptions{
		Follow:     opts.follow,
		Tail:       opts.tail,
		Timestamps: opts.timestamps,
		Since:      opts.since,
		Until:      opts.until,
	}

	// Get container logs channel from Docker
	logsChan, err := client.Docker.ContainerLogs(ctx, container.Container.ID, dockerOpts)
	if err != nil {
		return fmt.Errorf("get container logs: %w", err)
	}

	// Read from Docker logs channel and forward to our channel
	for entry := range logsChan {
		stream := "stdout"
		if entry.StreamType == 2 {
			stream = "stderr"
		}

		// Parse timestamp if provided
		var timestamp time.Time
		if entry.Timestamp != "" {
			timestamp, _ = time.Parse(time.RFC3339Nano, entry.Timestamp)
		} else {
			timestamp = time.Now()
		}

		logChan <- logEntry{
			timestamp:   timestamp,
			machineID:   machineID,
			machineName: machineName,
			serviceName: serviceName,
			replica:     container.Container.Name,
			stream:      stream,
			message:     entry.Message,
		}
	}

	return nil
}

// proxyToMachine returns a new context that proxies gRPC requests to the specified machine.
func proxyToMachine(ctx context.Context, machine *pb.MachineInfo) context.Context {
	machineIP, _ := machine.Network.ManagementIp.ToAddr()
	md := metadata.Pairs("machines", machineIP.String())
	return metadata.NewOutgoingContext(ctx, md)
}

// getMachineColor returns a consistent color for a given machine ID
func getMachineColor(machineID string) *color.Color {
	h := fnv.New32a()
	h.Write([]byte(machineID))
	hash := h.Sum32()

	colors := []*color.Color{
		color.New(color.FgCyan),
		color.New(color.FgGreen),
		color.New(color.FgYellow),
		color.New(color.FgBlue),
		color.New(color.FgMagenta),
		color.New(color.FgRed),
		color.New(color.FgHiCyan),
		color.New(color.FgHiGreen),
		color.New(color.FgHiYellow),
		color.New(color.FgHiBlue),
		color.New(color.FgHiMagenta),
		color.New(color.FgHiRed),
	}
	colorIndex := hash % uint32(len(colors))
	return colors[colorIndex]
}

func printLogEntry(entry logEntry, showTimestamp bool) {
	var output strings.Builder

	if showTimestamp {
		output.WriteString(entry.timestamp.Format(time.DateTime))
		output.WriteString(" ")
	}

	// Get color for machine ID
	machineColor := getMachineColor(entry.machineID)

	// Format colored prefix: [machineName (machineID)/serviceName]
	prefix := fmt.Sprintf("[%s (%s)/%s]", entry.machineName, entry.machineID, entry.serviceName)
	coloredPrefix := machineColor.Sprint(prefix)

	// Build final output
	output.WriteString(coloredPrefix)
	output.WriteString(" ")
	output.WriteString(entry.message)

	// Print to appropriate stream
	if entry.stream == "stderr" {
		fmt.Fprintln(os.Stderr, output.String())
	} else {
		fmt.Println(output.String())
	}
}

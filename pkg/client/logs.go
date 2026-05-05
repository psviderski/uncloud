package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/docker/docker/pkg/stringid"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/pkg/api"
)

// ServiceLogs streams log entries from all service containers in chronological order based on timestamps.
// Keep in mind that perfect ordering of log events across multiple machines can't be guaranteed due to the
// imperfection of physical clocks or potential clock skew between machines.
// It uses a low watermark algorithm to ensure proper ordering across multiple machines.
// Heartbeat entries from the server advance the watermark to enable timely emission of buffered logs.
func (cli *Client) ServiceLogs(
	ctx context.Context, serviceNameOrID string, opts api.ServiceLogsOptions,
) (api.Service, <-chan api.ServiceLogEntry, error) {
	snapshot, err := cli.NewClusterSnapshot(ctx, ClusterSnapshotOptions{Machines: true, Services: true})
	if err != nil {
		return api.Service{}, nil, fmt.Errorf("load cluster snapshot: %w", err)
	}
	return cli.ServiceLogsWithSnapshot(ctx, snapshot, serviceNameOrID, opts)
}

// ServiceLogsWithSnapshot streams log entries using a request-scoped snapshot for service and machine lookup.
func (cli *Client) ServiceLogsWithSnapshot(
	ctx context.Context, snapshot *ClusterSnapshot, serviceNameOrID string, opts api.ServiceLogsOptions,
) (api.Service, <-chan api.ServiceLogEntry, error) {
	svc, ok := snapshot.FindServiceByID(serviceNameOrID)
	var err error
	if !ok {
		svc, ok, err = snapshot.FindServiceByName(serviceNameOrID)
		if err != nil {
			return svc, nil, err
		}
	}
	if !ok {
		return svc, nil, api.ErrNotFound
	}

	containers := append(svc.Containers, svc.HookContainers...)
	if len(containers) == 0 {
		return svc, nil, fmt.Errorf("no containers found for service: %s", serviceNameOrID)
	}

	if len(opts.Containers) > 0 {
		selected := make(map[string]api.MachineServiceContainer, len(opts.Containers))
		for _, nameOrID := range opts.Containers {
			ctr, err := svc.FindContainer(nameOrID)
			if err != nil {
				return svc, nil, fmt.Errorf("find container '%s' in service '%s': %w",
					nameOrID, serviceNameOrID, err)
			}
			selected[ctr.Container.ID] = ctr
		}

		containers = slices.Collect(maps.Values(selected))
	}

	machines, err := filterSnapshotMachines(snapshot, opts.Machines)
	if err != nil {
		return svc, nil, err
	}

	ctrStreams := make([]<-chan api.ServiceLogEntry, 0, len(containers))
	for _, ctr := range containers {
		// Skip containers not running on the specified machines.
		m := machines.FindByNameOrID(ctr.MachineID)
		if len(opts.Machines) > 0 && m == nil {
			continue
		}
		if m == nil {
			return svc, nil, fmt.Errorf("machine '%s' not found in snapshot", ctr.MachineID)
		}

		machineName := m.Machine.Name

		stream, err := cli.containerLogsFromMachine(ctx, m.Machine, ctr.Container.ID, opts)
		if err != nil {
			// TODO: cancel already-opened streams. Currently they leak until ctx is cancelled which could
			//  be critical when used as SDK.
			return svc, nil, fmt.Errorf("stream logs from service container '%s' on machine '%s': %w",
				stringid.TruncateID(ctr.Container.ID), machineName, err)
		}

		// Enrich log entries from the container with service metadata.
		metadata := api.ServiceLogEntryMetadata{
			ServiceID:   svc.ID,
			ServiceName: svc.Name,
			ContainerID: ctr.Container.ID,
			MachineID:   ctr.MachineID,
			MachineName: machineName,
			Hook:        ctr.Container.Config.Labels[api.LabelHook],
		}
		enrichedStream := logsStreamWithServiceMetadata(stream, metadata)
		ctrStreams = append(ctrStreams, enrichedStream)
	}

	if len(ctrStreams) == 0 {
		return svc, nil, errors.New("no service containers found on the specified machine(s)")
	}

	// Use the log merger to combine streams from all containers in chronological order.
	merger := NewLogMerger(ctrStreams, DefaultLogMergerOptions)
	mergedStream := merger.Stream()

	return svc, mergedStream, nil
}

// ContainerLogs streams log entries from a single container on a specified machine.
func (cli *Client) ContainerLogs(
	ctx context.Context, machineNameOrID string, containerID string, opts api.ServiceLogsOptions,
) (<-chan api.LogEntry, error) {
	proxyCtx, _, err := cli.ProxyMachinesContext(ctx, []string{machineNameOrID})
	if err != nil {
		return nil, fmt.Errorf("create request context to proxy to machine '%s': %w", machineNameOrID, err)
	}

	return cli.containerLogs(ctx, proxyCtx, containerID, opts)
}

func (cli *Client) containerLogsFromMachine(
	ctx context.Context, machine *pb.MachineInfo, containerID string, opts api.ServiceLogsOptions,
) (<-chan api.LogEntry, error) {
	return cli.containerLogs(ctx, proxyToMachine(ctx, machine), containerID, opts)
}

func (cli *Client) containerLogs(
	ctx context.Context, proxyCtx context.Context, containerID string, opts api.ServiceLogsOptions,
) (<-chan api.LogEntry, error) {
	req := &pb.LogsRequest{
		Id:     containerID,
		Follow: opts.Follow,
		Tail:   int32(opts.Tail),
		Since:  opts.Since,
		Until:  opts.Until,
	}
	if !opts.Follow && opts.Tail == 0 {
		// If not following and tail is 0, set tail to -1 to return all logs.
		// Otherwise, no logs will be returned at all.
		req.Tail = -1
	}

	stream, err := cli.Docker.GRPCClient.ContainerLogs(proxyCtx, req)
	if err != nil {
		return nil, err
	}

	ch := make(chan api.LogEntry)

	go func() {
		defer close(ch)

		for {
			pbEntry, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				ch <- api.LogEntry{
					Err: err,
				}
				return
			}

			entry := api.LogEntry{
				Stream:    api.LogStreamTypeFromProto(pbEntry.Stream),
				Message:   pbEntry.Message,
				Timestamp: pbEntry.Timestamp.AsTime(),
			}

			select {
			case ch <- entry:
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch, nil
}

// MachineLogs streams journal logs for the given systemd service across one or more machines in
// chronological order based on timestamps. If opts.Machines is empty, logs are streamed from all
// machines in the cluster.
func (cli *Client) MachineLogs(
	ctx context.Context, unit string, opts api.ServiceLogsOptions,
) (<-chan api.ServiceLogEntry, error) {
	snapshot, err := cli.NewClusterSnapshot(ctx, ClusterSnapshotOptions{Machines: true})
	if err != nil {
		return nil, fmt.Errorf("load cluster snapshot: %w", err)
	}
	return cli.MachineLogsWithSnapshot(ctx, snapshot, unit, opts)
}

// MachineLogsWithSnapshot streams journal logs using a request-scoped snapshot for machine lookup.
func (cli *Client) MachineLogsWithSnapshot(
	ctx context.Context, snapshot *ClusterSnapshot, unit string, opts api.ServiceLogsOptions,
) (<-chan api.ServiceLogEntry, error) {
	machines, err := filterSnapshotMachines(snapshot, opts.Machines)
	if err != nil {
		return nil, err
	}
	if len(machines) == 0 {
		return nil, errors.New("no machines found")
	}

	streams := make([]<-chan api.ServiceLogEntry, 0, len(machines))
	for _, m := range machines {
		ch, err := cli.systemdServiceLogsFromMachine(ctx, m.Machine, unit, opts)
		if err != nil {
			// TODO: cancel already-opened streams. Currently they leak until ctx is cancelled which could
			//  be critical when used as SDK.
			return nil, fmt.Errorf("stream logs from systemd service '%s' on machine '%s': %w",
				unit, m.Machine.Name, err)
		}

		// Enrich journal log entries with the systemd service name and machine metadata.
		metadata := api.ServiceLogEntryMetadata{
			ServiceID:   unit,
			ServiceName: unit,
			MachineID:   m.Machine.Id,
			MachineName: m.Machine.Name,
		}
		streams = append(streams, logsStreamWithServiceMetadata(ch, metadata))
	}

	merger := NewLogMerger(streams, DefaultLogMergerOptions)
	return merger.Stream(), nil
}

// systemdServiceLogs streams log entries from a single systemd service on the specified machine.
func (cli *Client) systemdServiceLogs(
	ctx context.Context, machineID, unit string, opts api.ServiceLogsOptions,
) (<-chan api.LogEntry, error) {
	proxyCtx, _, err := cli.ProxyMachinesContext(ctx, []string{machineID})
	if err != nil {
		return nil, fmt.Errorf("create request context to proxy to machine '%s': %w", machineID, err)
	}

	return cli.systemdServiceLogsWithContext(ctx, proxyCtx, unit, opts)
}

func (cli *Client) systemdServiceLogsFromMachine(
	ctx context.Context, machine *pb.MachineInfo, unit string, opts api.ServiceLogsOptions,
) (<-chan api.LogEntry, error) {
	return cli.systemdServiceLogsWithContext(ctx, proxyToMachine(ctx, machine), unit, opts)
}

func (cli *Client) systemdServiceLogsWithContext(
	ctx context.Context, proxyCtx context.Context, unit string, opts api.ServiceLogsOptions,
) (<-chan api.LogEntry, error) {
	req := &pb.LogsRequest{
		Id:     unit,
		Follow: opts.Follow,
		Tail:   int32(opts.Tail),
		Since:  opts.Since,
		Until:  opts.Until,
	}
	if !opts.Follow && opts.Tail == 0 {
		// If not following and tail is 0, set tail to -1 to return all logs.
		// Otherwise, no logs will be returned at all.
		req.Tail = -1
	}

	stream, err := cli.MachineClient.MachineLogs(proxyCtx, req)
	if err != nil {
		return nil, err
	}

	ch := make(chan api.LogEntry)
	go func() {
		defer close(ch)

		for {
			pbEntry, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				ch <- api.LogEntry{Err: err}
				return
			}

			entry := api.LogEntry{
				Stream:    api.LogStreamTypeFromProto(pbEntry.Stream),
				Message:   pbEntry.Message,
				Timestamp: pbEntry.Timestamp.AsTime(),
			}

			select {
			case ch <- entry:
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch, nil
}

func filterSnapshotMachines(snapshot *ClusterSnapshot, namesOrIDs []string) (api.MachineMembersList, error) {
	if len(namesOrIDs) == 0 {
		return snapshot.Machines, nil
	}

	var machines api.MachineMembersList
	var notFound []string
	for _, nameOrID := range namesOrIDs {
		if m := snapshot.Machines.FindByNameOrID(nameOrID); m != nil {
			machines = append(machines, m)
		} else {
			notFound = append(notFound, nameOrID)
		}
	}
	if len(notFound) > 0 {
		return nil, fmt.Errorf("machines not found: %s", strings.Join(notFound, ", "))
	}
	return machines, nil
}

// logsStreamWithServiceMetadata wraps a container logs stream and enriches each log entry with service metadata.
func logsStreamWithServiceMetadata(
	stream <-chan api.LogEntry, metadata api.ServiceLogEntryMetadata,
) <-chan api.ServiceLogEntry {
	out := make(chan api.ServiceLogEntry)

	go func() {
		for entry := range stream {
			out <- api.ServiceLogEntry{
				Metadata: metadata,
				LogEntry: entry,
			}
		}
		close(out)
	}()

	return out
}

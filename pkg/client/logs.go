package client

import (
	"context"
	"fmt"
	"io"

	"github.com/docker/docker/pkg/stringid"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/pkg/api"
)

const defaultLogBufferSize = 100

// ServiceLogs streams log entries from all service containers in chronological order based on timestamps.
// Keep in mind that perfect ordering of log events across multiple machines can't be guaranteed due to the
// imperfection of physical clocks or potential clock skew between machines.
// It uses a low watermark algorithm to ensure proper ordering across multiple machines.
// Heartbeat entries from the server advance the watermark to enable timely emission of buffered logs.
func (cli *Client) ServiceLogs(
	ctx context.Context, serviceNameOrID string, opts api.ServiceLogsOptions,
) (api.Service, <-chan api.ServiceLogEntry, error) {
	svc, err := cli.InspectService(ctx, serviceNameOrID)
	if err != nil {
		return svc, nil, fmt.Errorf("inspect service: %w", err)
	}

	if len(svc.Containers) == 0 {
		return svc, nil, fmt.Errorf("no containers found for service: %s", serviceNameOrID)
	}

	svcStreams := make([]<-chan api.ServiceLogEntry, 0, len(svc.Containers))
	for _, ctr := range svc.Containers {
		stream, err := cli.ContainerLogs(ctx, ctr.MachineID, ctr.Container.ID, opts)
		if err != nil {
			// Try to get machine name for friendlier error message.
			machineName := ctr.MachineID
			machines, mErr := cli.ListMachines(ctx, &api.MachineFilter{NamesOrIDs: []string{ctr.MachineID}})
			if mErr == nil && len(machines) > 0 {
				machineName = machines[0].Machine.Name
			}

			return svc, nil, fmt.Errorf("stream logs from service container '%s' on machine '%s': %w",
				stringid.TruncateID(ctr.Container.ID), machineName, err)
		}

		// Enrich log entries from the container with service metadata.
		enrichedStream := withServiceMetadata(stream, svc.ID, svc.Name)
		svcStreams = append(svcStreams, enrichedStream)
	}

	// Use the log merger to combine streams from all containers in chronological order.
	merger := NewLogMerger(svcStreams, DefaultMergerOptions())
	mergedStream := merger.Merge(ctx)

	return svc, mergedStream, nil
}

// ContainerLogs streams log entries from a single container on a specified machine.
func (cli *Client) ContainerLogs(
	ctx context.Context, machineNameOrID string, containerID string, opts api.ServiceLogsOptions,
) (<-chan api.ServiceLogEntry, error) {
	proxyCtx, machines, err := cli.ProxyMachinesContext(ctx, []string{machineNameOrID})
	if err != nil {
		return nil, fmt.Errorf("create request context to proxy to machine '%s': %w", machineNameOrID, err)
	}

	req := &pb.ContainerLogsRequest{
		ContainerId: containerID,
		Follow:      opts.Follow,
		Tail:        int32(opts.Tail),
		Since:       opts.Since,
		Until:       opts.Until,
	}
	stream, err := cli.Docker.GRPCClient.ContainerLogs(proxyCtx, req)
	if err != nil {
		return nil, err
	}

	ch := make(chan api.ServiceLogEntry, defaultLogBufferSize)

	go func() {
		defer close(ch)

		metadata := api.ServiceLogEntryMetadata{
			ContainerID: containerID,
			MachineID:   machines[0].Machine.Id,
			MachineName: machines[0].Machine.Name,
		}

		for {
			pbEntry, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				ch <- api.ServiceLogEntry{
					Metadata: metadata,
					Err:      err,
				}
				return
			}

			entry := api.ServiceLogEntry{
				Metadata:  metadata,
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

// withServiceMetadata wraps a logs stream and enriches each log entry with service metadata.
func withServiceMetadata(
	stream <-chan api.ServiceLogEntry,
	serviceID string,
	serviceName string,
) <-chan api.ServiceLogEntry {
	out := make(chan api.ServiceLogEntry, defaultLogBufferSize)

	go func() {
		defer close(out)

		for entry := range stream {
			entry.Metadata.ServiceID = serviceID
			entry.Metadata.ServiceName = serviceName

			out <- entry
		}
	}()

	return out
}

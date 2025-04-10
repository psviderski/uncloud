package client

import (
	"context"
	"fmt"

	"github.com/docker/compose/v2/pkg/progress"
	"github.com/docker/docker/api/types/volume"
	dockerclient "github.com/docker/docker/client"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/pkg/api"
)

// CreateVolume creates a new volume on the specified machine.
func (cli *Client) CreateVolume(
	ctx context.Context, machineNameOrID string, opts volume.CreateOptions,
) (api.MachineVolume, error) {
	var resp api.MachineVolume

	if opts.Name == "" {
		return resp, fmt.Errorf("volume name is required (anonymous volumes are not supported)")
	}

	machine, err := cli.InspectMachine(ctx, machineNameOrID)
	if err != nil {
		return resp, fmt.Errorf("inspect machine '%s': %w", machineNameOrID, err)
	}
	// Proxy Docker gRPC requests to the selected machine.
	ctx = proxyToMachine(ctx, machine.Machine)

	pw := progress.ContextWriter(ctx)
	eventID := fmt.Sprintf("Volume %s on %s", opts.Name, machine.Machine.Name)
	pw.Event(progress.CreatingEvent(eventID))

	vol, err := cli.Docker.CreateVolume(ctx, opts)
	if err != nil {
		return resp, fmt.Errorf("create volume on machine '%s': %w", machine.Machine.Name, err)
	}

	resp = api.MachineVolume{
		MachineID:   machine.Machine.Id,
		MachineName: machine.Machine.Name,
		Volume:      vol,
	}
	pw.Event(progress.CreatedEvent(eventID))

	return resp, nil
}

// ListVolumes returns a list of all volumes on the cluster machines.
func (cli *Client) ListVolumes(ctx context.Context) ([]api.MachineVolume, error) {
	machines, err := cli.ListMachines(ctx)
	if err != nil {
		return nil, fmt.Errorf("list machines: %w", err)
	}

	// Broadcast the volume list request to all machines.
	listCtx, err := api.ProxyMachinesContext(ctx, cli, nil)
	if err != nil {
		return nil, fmt.Errorf("create request context to broadcast to all machines: %w", err)
	}

	machineVolumes, err := cli.Docker.ListVolumes(listCtx, volume.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list volumes: %w", err)
	}

	var volumes []api.MachineVolume
	// Process responses from all machines.
	for _, mv := range machineVolumes {
		if mv.Metadata != nil && mv.Metadata.Error != "" {
			// TODO: return failed machines in the response.
			PrintWarning(fmt.Sprintf("failed to list volumes on machine '%s': %s",
				mv.Metadata.Machine, mv.Metadata.Error))
			continue
		}

		var m *pb.MachineMember
		if mv.Metadata == nil {
			// ListVolumes was proxied to only one machine.
			m = machines[0]
		} else {
			m = machines.FindByManagementIP(mv.Metadata.Machine)
			if m == nil {
				return nil, fmt.Errorf("machine not found by management IP: %s", mv.Metadata.Machine)
			}
		}

		for _, vol := range mv.Response.Volumes {
			volumes = append(volumes, api.MachineVolume{
				MachineID:   m.Machine.Id,
				MachineName: m.Machine.Name,
				Volume:      *vol,
			})
		}
	}

	return volumes, nil
}

// RemoveVolume removes a volume from the specified machine.
func (cli *Client) RemoveVolume(ctx context.Context, machineNameOrID, volumeName string, force bool) error {
	machine, err := cli.InspectMachine(ctx, machineNameOrID)
	if err != nil {
		return fmt.Errorf("inspect machine '%s': %w", machineNameOrID, err)
	}
	// Proxy Docker gRPC requests to the selected machine.
	ctx = proxyToMachine(ctx, machine.Machine)

	pw := progress.ContextWriter(ctx)
	eventID := fmt.Sprintf("Volume %s on %s", volumeName, machine.Machine.Name)
	pw.Event(progress.RemovingEvent(eventID))

	if err = cli.Docker.RemoveVolume(ctx, volumeName, force); err != nil {
		if dockerclient.IsErrNotFound(err) {
			return api.ErrNotFound
		}
		return fmt.Errorf("remove volume '%s' from machine '%s': %w",
			volumeName, machine.Machine.Name, err)
	}
	pw.Event(progress.RemovedEvent(eventID))

	return nil
}

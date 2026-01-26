package client

import (
	"context"
	"fmt"

	"github.com/containerd/errdefs"
	"github.com/docker/compose/v2/pkg/progress"
	"github.com/docker/docker/api/types/volume"
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
		return resp, err
	}

	resp = api.MachineVolume{
		MachineID:   machine.Machine.Id,
		MachineName: machine.Machine.Name,
		Volume:      vol,
	}
	pw.Event(progress.CreatedEvent(eventID))

	return resp, nil
}

// ListVolumes returns a list of all volumes on the cluster machines that match the filter.
func (cli *Client) ListVolumes(ctx context.Context, filter *api.VolumeFilter) ([]api.MachineVolume, error) {
	// Broadcast the volume list request to the specified machines in the filter or all machines if filter is nil.
	var proxyMachines []string
	if filter != nil {
		proxyMachines = filter.Machines
	}

	mctx, err := cli.ProxyMachinesContext(ctx, proxyMachines)
	if err != nil {
		return nil, fmt.Errorf("create request context to broadcast to all machines: %w", err)
	}

	machineVolumes, err := cli.Docker.ListVolumes(mctx, volume.ListOptions{})
	if err != nil {
		return nil, err
	}

	var volumes []api.MachineVolume
	// Process responses from all machines.
	for res := range ResolveMachines(mctx, machineVolumes) {
		mv := res.Item
		m := res.Machine
		// Note: ResolveMachines iterator handles error checking and resolution.
		// It yields only valid results or results where machine might be unknown but no error in metadata.
		// However, for volumes we really need the machine ID.

		if m == nil {
			// We need the machine ID to construct the MachineVolume response.
			// If we couldn't resolve the machine from the metadata (IP), we skip it.
			PrintWarning(fmt.Sprintf("machine not found by management IP: %s", res.MachineAddr))
			continue
		}

		for _, vol := range mv.Response.Volumes {

			volumes = append(volumes, api.MachineVolume{
				MachineID:   m.Machine.Id,
				MachineName: m.Machine.Name,
				Volume:      *vol,
			})
		}
	}

	// Filter volumes based on the provided filter criteria.
	if filter != nil {
		var filteredVolumes []api.MachineVolume
		for _, vol := range volumes {
			if vol.MatchesFilter(filter) {
				filteredVolumes = append(filteredVolumes, vol)
			}
		}
		volumes = filteredVolumes
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
		if errdefs.IsNotFound(err) {
			return api.ErrNotFound
		}
		return err
	}
	pw.Event(progress.RemovedEvent(eventID))

	return nil
}

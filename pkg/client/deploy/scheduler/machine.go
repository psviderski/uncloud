package scheduler

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types/volume"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/pkg/api"
)

type Client interface {
	api.MachineClient
	api.VolumeClient
}

type Machine struct {
	Info    *pb.MachineInfo
	Volumes []volume.Volume
}

// InspectMachines retrieves the list of available machines and their details required for scheduling purposes.
// TODO: refactor to get all the details in one broadcast call to machine API.
func InspectMachines(ctx context.Context, cli Client) ([]*Machine, error) {
	machineMembers, err := cli.ListMachines(ctx, &api.MachineFilter{Available: true})
	if err != nil {
		return nil, fmt.Errorf("list machines: %w", err)
	}
	volumes, err := cli.ListVolumes(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("list volumes: %w", err)
	}

	var machines []*Machine
	for _, m := range machineMembers {
		machine := &Machine{
			Info: m.Machine,
		}

		for _, v := range volumes {
			if v.MachineID == m.Machine.Id {
				machine.Volumes = append(machine.Volumes, v.Volume)
			}
		}

		machines = append(machines, machine)
	}

	return machines, nil
}

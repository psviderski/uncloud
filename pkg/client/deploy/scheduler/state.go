package scheduler

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types/volume"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/pkg/api"
)

// ClusterState represents the current and planned state of machines and their resources in the cluster.
type ClusterState struct {
	Machines []*Machine
}

type Machine struct {
	Info             *pb.MachineInfo
	Volumes          []volume.Volume
	ScheduledVolumes []api.VolumeSpec
}

type Client interface {
	api.MachineClient
	api.VolumeClient
}

// InspectClusterState creates a new cluster state by inspecting the machines using the cluster client.
func InspectClusterState(ctx context.Context, cli Client) (*ClusterState, error) {
	// TODO: refactor to get all the details in one broadcast call to machine API,
	//  e.g. InspectMachine with include options.
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

	return &ClusterState{
		Machines: machines,
	}, nil
}

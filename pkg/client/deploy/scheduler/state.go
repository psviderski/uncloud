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
	// ExistingContainers is the number of containers already running on this machine (for ranking).
	ExistingContainers int
	// ScheduledCPU tracks CPU nanocores reserved by containers scheduled during this planning session.
	ScheduledCPU int64
	// ScheduledMemory tracks memory bytes reserved by containers scheduled during this planning session.
	ScheduledMemory int64
	// ScheduledContainers tracks the number of containers scheduled on this machine during this planning session.
	ScheduledContainers int
}

// AvailableCPU returns the available CPU nanocores on the machine after accounting for
// both running containers and containers scheduled during this planning session.
func (m *Machine) AvailableCPU() int64 {
	return m.Info.TotalCpuNanos - m.Info.ReservedCpuNanos - m.ScheduledCPU
}

// AvailableMemory returns the available memory bytes on the machine after accounting for
// both running containers and containers scheduled during this planning session.
func (m *Machine) AvailableMemory() int64 {
	return m.Info.TotalMemoryBytes - m.Info.ReservedMemoryBytes - m.ScheduledMemory
}

// ReserveResources reserves the given CPU and memory for a container scheduled on this machine.
func (m *Machine) ReserveResources(cpu, memory int64) {
	m.ScheduledCPU += cpu
	m.ScheduledMemory += memory
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

// Machine returns the machine with the given name or ID from the cluster state.
func (s *ClusterState) Machine(nameOrID string) (*Machine, bool) {
	for _, m := range s.Machines {
		if m.Info.Id == nameOrID || m.Info.Name == nameOrID {
			return m, true
		}
	}
	return nil, false
}

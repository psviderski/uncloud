package client

import (
	"context"
	"fmt"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/internal/secret"
	"github.com/psviderski/uncloud/pkg/api"
	"math/rand/v2"
	"slices"
)

// Strategy defines how a service should be deployed or updated. Different implementations can provide various
// deployment patterns such as rolling updates, blue/green deployments, etc.
type Strategy interface {
	// Type returns the type of the deployment strategy, e.g. "rolling", "blue-green".
	Type() string
	// Plan returns the operation to reconcile the service to the desired state.
	// If the service does not exist (new deployment), svc will be nil.
	Plan(ctx context.Context, cli *Client, svc *api.Service, spec api.ServiceSpec) (Plan, error)
}

// RollingStrategy implements a rolling update deployment pattern where containers are updated one at a time
// to minimize service disruption.
type RollingStrategy struct {
	// MachineFilter optionally restricts which machines can be used for deployment.
	MachineFilter MachineFilter
}

func (s *RollingStrategy) Type() string {
	return "rolling"
}

func (s *RollingStrategy) Plan(
	ctx context.Context, cli *Client, svc *api.Service, spec api.ServiceSpec,
) (Plan, error) {
	// We can assume that the spec is valid at this point because it has been validated by the deployment.
	switch spec.Mode {
	case api.ServiceModeReplicated:
		return s.planReplicated(ctx, cli, svc, spec)
	case api.ServiceModeGlobal:
		return s.planGlobal(ctx, cli, svc, spec)
	default:
		return Plan{}, fmt.Errorf("unsupported service mode: '%s'", spec.Mode)
	}
}

// planReplicated creates a plan for a replicated service deployment.
// For replicated services, we want to maintain a specific number of containers (replicas) across the available machines
// in the cluster.
func (s *RollingStrategy) planReplicated(
	ctx context.Context, cli *Client, svc *api.Service, spec api.ServiceSpec,
) (Plan, error) {
	plan, err := newEmptyPlan(svc, spec)
	if err != nil {
		return plan, err
	}

	machines, err := cli.ListMachines(ctx)
	if err != nil {
		return plan, fmt.Errorf("list machines: %w", err)
	}
	// Filter machines that are not DOWN and match the machine filter if provided.
	var availableMachines []*pb.MachineInfo
	var unmatchedMachines []*pb.MachineInfo
	var downMachines []*pb.MachineInfo
	for _, m := range machines {
		if m.State == pb.MachineMember_DOWN {
			downMachines = append(downMachines, m.Machine)
		} else {
			if s.MachineFilter == nil || s.MachineFilter(m.Machine) {
				availableMachines = append(availableMachines, m.Machine)
			} else {
				unmatchedMachines = append(unmatchedMachines, m.Machine)
			}
		}
	}

	if len(availableMachines) == 0 {
		if s.MachineFilter != nil {
			return plan, ErrNoMatchingMachines
		}
		return plan, fmt.Errorf("no available machines to deploy service")
	}
	// Randomise the order of machines to avoid always deploying to the same machines first.
	rand.Shuffle(len(availableMachines), func(i, j int) {
		availableMachines[i], availableMachines[j] = availableMachines[j], availableMachines[i]
	})

	// Organise existing containers by machine.
	containersOnMachine := make(map[string][]api.Container)
	upToDateContainersOnMachine := make(map[string]int)
	containerSpecStatuses := make(map[string]ContainerSpecStatus)
	if svc != nil {
		for _, c := range svc.Containers {
			if !c.Container.State.Running || c.Container.State.Paused {
				// Skip containers that are not running.
				continue
			}

			status, err := CompareContainerToSpec(c.Container, spec)
			if err != nil {
				return plan, fmt.Errorf("compare container to spec: %w", err)
			}
			containerSpecStatuses[c.Container.ID] = status

			if status == ContainerUpToDate {
				upToDateContainersOnMachine[c.MachineID] += 1
			}
		}

		// Sort containers such that running containers with the desired spec are first.
		slices.SortFunc(svc.Containers, func(c1, c2 api.MachineContainer) int {
			if status, ok := containerSpecStatuses[c1.Container.ID]; ok && status == ContainerUpToDate {
				return -1
			}
			if status, ok := containerSpecStatuses[c2.Container.ID]; ok && status == ContainerUpToDate {
				return 1
			}
			return 0
		})

		for _, c := range svc.Containers {
			containersOnMachine[c.MachineID] = append(containersOnMachine[c.MachineID], c.Container)
		}

		// Sort machines such that machines with the most up-to-date containers are first, followed by machines with
		// existing containers, and finally machines without containers.
		slices.SortFunc(availableMachines, func(m1, m2 *pb.MachineInfo) int {
			if upToDateContainersOnMachine[m1.Id] > 0 && upToDateContainersOnMachine[m2.Id] > 0 {
				return upToDateContainersOnMachine[m2.Id] - upToDateContainersOnMachine[m1.Id]
			}
			if upToDateContainersOnMachine[m1.Id] > 0 {
				return -1
			}
			if upToDateContainersOnMachine[m2.Id] > 0 {
				return 1
			}
			return len(containersOnMachine[m2.Id]) - len(containersOnMachine[m1.Id])
		})
	}

	// Spread the containers across the available machines evenly using a simple round-robin approach, starting with
	// machines that already have containers and prioritising machines with containers that match the desired spec.
	for i := 0; i < int(spec.Replicas); i++ {
		m := availableMachines[i%len(availableMachines)]
		containers := containersOnMachine[m.Id]

		if len(containers) == 0 {
			// No more existing containers on this machine, create a new one.
			plan.Operations = append(plan.Operations, &RunContainerOperation{
				ServiceID: plan.ServiceID,
				Spec:      spec,
				MachineID: m.Id,
			})
			continue
		}

		ctr := containers[0]
		containersOnMachine[m.Id] = containers[1:]

		if status, ok := containerSpecStatuses[ctr.ID]; ok { // Contains statuses for only running containers.
			if status == ContainerUpToDate {
				continue
			}
			// TODO: handle ContainerNeedsUpdate when update of mutable fields on a container is supported.

			conflictingPorts, portsErr := ctr.ConflictingServicePorts(spec.Ports)
			if portsErr != nil || len(conflictingPorts) > 0 {
				// Stop the malformed container or the container with conflicting ports.
				plan.Operations = append(plan.Operations, &StopContainerOperation{
					ServiceID:   plan.ServiceID,
					ContainerID: ctr.ID,
					MachineID:   m.Id,
				})
			}
		}

		// Run a new container.
		plan.Operations = append(plan.Operations, &RunContainerOperation{
			ServiceID: plan.ServiceID,
			Spec:      spec,
			MachineID: m.Id,
		})

		// Remove the old container.
		plan.Operations = append(plan.Operations, &RemoveContainerOperation{
			ServiceID:   plan.ServiceID,
			ContainerID: ctr.ID,
			MachineID:   m.Id,
		})
	}

	// Remove any remaining containers that are not needed.
	for mid, containers := range containersOnMachine {
		for _, c := range containers {
			plan.Operations = append(plan.Operations, &RemoveContainerOperation{
				ServiceID:   plan.ServiceID,
				ContainerID: c.ID,
				MachineID:   mid,
			})
		}
	}

	return plan, nil
}

// planGlobal creates a plan for a global service deployment, ensuring one container runs on each available machine.
// For machines with an existing container, it attempts to start a new container before removing the old one if
// possible. If the new container would have port conflicts with the existing one, the old container is removed first.
// It handles multiple containers per machine (though this should not occur in normal operation) and skips machines
// that are down.
func (s *RollingStrategy) planGlobal(
	ctx context.Context, cli *Client, svc *api.Service, spec api.ServiceSpec,
) (Plan, error) {
	plan, err := newEmptyPlan(svc, spec)
	if err != nil {
		return plan, err
	}

	// Map machineID to service containers on that machine. For the global mode, there should be at most one
	// container per machine but we use a slice to handle multiple containers that may exist due to a bug
	// or interruption in the previous deployment.
	containersOnMachine := make(map[string][]api.MachineContainer)
	if svc != nil {
		for _, c := range svc.Containers {
			containersOnMachine[c.MachineID] = append(containersOnMachine[c.MachineID], c)
		}
	}

	machines, err := cli.ListMachines(ctx)
	if err != nil {
		return plan, fmt.Errorf("list machines: %w", err)
	}
	// Filter machines if a machine filter is provided.
	// TODO: not sure this is the right behaviour to ignore other machines that might run service containers.
	//  Maybe there should be another filter to specify which machines to deploy to but keep the rest running.
	//  Could be useful to test a new version on a subset of machines before rolling out to all.
	if s.MachineFilter != nil {
		machines = slices.DeleteFunc(machines, func(m *pb.MachineMember) bool {
			return !s.MachineFilter(m.Machine)
		})
		if len(machines) == 0 {
			return plan, ErrNoMatchingMachines
		}
	}

	// TODO: figure out how to return a warning if there are machines down. Embed the machinesDown in the plan?
	var machinesDown []*pb.MachineInfo
	for _, m := range machines {
		// Skip machines that are down but collect them to report a warning later.
		if m.State == pb.MachineMember_DOWN {
			machinesDown = append(machinesDown, m.Machine)
			fmt.Printf("WARNING: failed to run a service container on machine '%s' which is Down.\n", m.Machine.Id)
			continue
		}

		containers := containersOnMachine[m.Machine.Id]
		ops, err := reconcileGlobalContainer(containers, spec, plan.ServiceID, m.Machine.Id)
		if err != nil {
			return plan, err
		}
		plan.Operations = append(plan.Operations, ops...)
	}

	return plan, nil
}

// reconcileGlobalContainer returns a sequence of operations to reconcile containers on a machine for a global service.
// It ensures exactly one container with the desired spec is running on the machine by creating a new container and
// removing old ones. If there is a host port conflict, it stops the old container before starting a new one.
func reconcileGlobalContainer(
	containers []api.MachineContainer, spec api.ServiceSpec, serviceID, machineID string,
) ([]Operation, error) {
	var ops []Operation

	if len(containers) == 0 {
		// No containers on this machine, create a new one.
		ops = append(ops, &RunContainerOperation{
			ServiceID: serviceID,
			Spec:      spec,
			MachineID: machineID,
		})
		return ops, nil
	}

	// Check if there is a container with the same spec already running. If so, remove the rest.
	upToDate := false
	for i, c := range containers {
		if !c.Container.State.Running || c.Container.State.Paused {
			// Skip containers that are not running.
			continue
		}

		status, err := CompareContainerToSpec(c.Container, spec)
		if err != nil {
			return nil, fmt.Errorf("compare container to spec: %w", err)
		}
		if status == ContainerUpToDate {
			// The container is already running with the same spec.
			upToDate = true
			for j, old := range containers {
				if i == j {
					continue
				}
				ops = append(ops, &RemoveContainerOperation{
					ServiceID:   serviceID,
					ContainerID: old.Container.ID,
					MachineID:   old.MachineID,
				})
			}
			break
		}
		// TODO: handle ContainerNeedsUpdate when update of mutable fields on a container is supported.
	}
	if upToDate {
		return ops, nil
	}

	// The machine has containers but none of them match the new spec.
	// Stop the old running containers that have conflicting ports with the new spec before running a new one.
	for _, c := range containers {
		if c.Container.State.Running {
			conflictingPorts, err := c.Container.ConflictingServicePorts(spec.Ports)
			if err != nil {
				return nil, fmt.Errorf("check conflicting ports: %w", err)
			}

			if len(conflictingPorts) > 0 {
				// Stop the running container with conflicting ports.
				ops = append(ops, &StopContainerOperation{
					ServiceID:   serviceID,
					ContainerID: c.Container.ID,
					MachineID:   c.MachineID,
				})
			}
		}
	}

	// Run a new container.
	ops = append(ops, &RunContainerOperation{
		ServiceID: serviceID,
		Spec:      spec,
		MachineID: machineID,
	})

	// Remove the old containers.
	for _, c := range containers {
		ops = append(ops, &RemoveContainerOperation{
			ServiceID:   serviceID,
			ContainerID: c.Container.ID,
			MachineID:   c.MachineID,
		})
	}

	return ops, nil
}

// newEmptyPlan creates a new empty plan for a service deployment with initialised service ID and name.
func newEmptyPlan(svc *api.Service, spec api.ServiceSpec) (Plan, error) {
	var plan Plan

	// Generate a new service ID for the initial service deployment if it doesn't exist yet.
	if svc != nil {
		plan.ServiceID = svc.ID
		plan.ServiceName = svc.Name
	} else {
		var err error
		plan.ServiceID, err = secret.NewID()
		if err != nil {
			return plan, fmt.Errorf("generate service ID: %w", err)
		}
		plan.ServiceName = spec.Name
	}

	return plan, nil
}

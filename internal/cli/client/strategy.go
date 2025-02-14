package client

import (
	"context"
	"errors"
	"fmt"
	"uncloud/internal/api"
	"uncloud/internal/machine/api/pb"
	"uncloud/internal/secret"
)

// Strategy defines how a service should be deployed or updated. Different implementations can provide various
// deployment patterns such as rolling updates, blue/green deployments, etc.
type Strategy interface {
	// Plan returns the operation to reconcile the service to the desired state.
	// If the service does not exist (new deployment), svc will be nil.
	Plan(ctx context.Context, cli *Client, svc *api.Service, spec api.ServiceSpec) (Operation, error)
}

// RollingStrategy implements a rolling update deployment pattern where containers are updated one at a time
// to minimize service disruption.
type RollingStrategy struct{}

func (s *RollingStrategy) Plan(
	ctx context.Context, cli *Client, svc *api.Service, spec api.ServiceSpec,
) (Operation, error) {
	switch spec.Mode {
	case "", api.ServiceModeReplicated:
		return s.planReplicated(ctx, cli, svc, spec)
	case api.ServiceModeGlobal:
		return s.planGlobal(ctx, cli, svc, spec)
	default:
		return nil, fmt.Errorf("unsupported service mode: %s", spec.Mode)
	}
}

// planReplicated creates a plan for a replicated service deployment.
func (s *RollingStrategy) planReplicated(
	ctx context.Context, cli *Client, svc *api.Service, spec api.ServiceSpec,
) (Operation, error) {
	return nil, errors.New("not implemented")
}

// planGlobal creates a plan for a global service deployment, ensuring one container runs on each available machine.
// For machines with an existing container, it attempts to start a new container before removing the old one if
// possible. If the new container would have port conflicts with the existing one, the old container is removed first.
// It handles multiple containers per machine (though this should not occur in normal operation) and skips machines
// that are down.
func (s *RollingStrategy) planGlobal(
	ctx context.Context, cli *Client, svc *api.Service, spec api.ServiceSpec,
) (Operation, error) {
	serviceID := ""
	// Map machineID to service containers on that machine. For the global mode, there should be at most one
	// container per machine but we use a slice to handle multiple containers that may exist due to a bug
	// or interruption in the previous deployment.
	containersOnMachine := make(map[string][]api.MachineContainer)
	if svc != nil {
		serviceID = svc.ID
		for _, c := range svc.Containers {
			containersOnMachine[c.MachineID] = append(containersOnMachine[c.MachineID], c)
		}
	} else {
		// Generate a new service ID for the first service deployment.
		var err error
		serviceID, err = secret.NewID()
		if err != nil {
			return nil, fmt.Errorf("generate service ID: %w", err)
		}
	}

	machines, err := cli.ListMachines(ctx)
	if err != nil {
		return nil, fmt.Errorf("list machines: %w", err)
	}

	plan := &SequenceOperation{}
	// TODO: figure out how to return a warning if there are machines down.
	var machinesDown []*pb.MachineInfo
	for _, m := range machines {
		// Skip machines that are down but collect them to report a warning later.
		if m.State == pb.MachineMember_DOWN {
			machinesDown = append(machinesDown, m.Machine)
			continue
		}

		containers := containersOnMachine[m.Machine.Id]
		ops, err := reconcileGlobalContainer(containers, spec, serviceID, m.Machine.Id)
		if err != nil {
			return nil, err
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

		svcSpec, err := c.Container.ServiceSpec()
		if err != nil {
			return nil, fmt.Errorf("get service spec: %w", err)
		}
		if svcSpec.Equals(spec) {
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

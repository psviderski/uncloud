package deploy

import (
	"fmt"
	"math/rand/v2"
	"slices"

	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/internal/secret"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client/deploy/operation"
	"github.com/psviderski/uncloud/pkg/client/deploy/scheduler"
)

// Strategy defines how a service should be deployed or updated. Different implementations can provide various
// deployment patterns such as rolling updates, blue/green deployments, etc.
type Strategy interface {
	// Type returns the type of the deployment strategy, e.g. "rolling", "blue-green".
	Type() string
	// Plan returns the operation to reconcile the service to the desired state.
	// If the service does not exist (new deployment), svc will be nil. state provides the current and planned state
	// of the cluster for scheduling decisions.
	Plan(state *scheduler.ClusterState, svc *api.Service, spec api.ServiceSpec) (Plan, error)
}

// RollingStrategy implements a rolling update deployment pattern where containers are updated one at a time
// to minimize service disruption.
type RollingStrategy struct {
	// ForceRecreate indicates whether all containers should be recreated during the deployment,
	// regardless of whether their specifications have changed.
	ForceRecreate bool

	// state is the current and planned state of the cluster used for scheduling decisions.
	state *scheduler.ClusterState
}

func (s *RollingStrategy) Type() string {
	return "rolling"
}

func (s *RollingStrategy) Plan(state *scheduler.ClusterState, svc *api.Service, spec api.ServiceSpec) (Plan, error) {
	if state == nil {
		return Plan{}, fmt.Errorf("cluster state must be provided")
	}
	s.state = state

	// We can assume that the spec is valid at this point because it has been validated by the deployment.
	switch spec.Mode {
	case api.ServiceModeReplicated:
		return s.planReplicated(svc, spec)
	case api.ServiceModeGlobal:
		return s.planGlobal(svc, spec)
	default:
		return Plan{}, fmt.Errorf("unsupported service mode: '%s'", spec.Mode)
	}
}

// planReplicated creates a plan for a replicated service deployment.
// For replicated services, we want to maintain a specific number of containers (replicas) across the available machines
// in the cluster.
// TODO: schedule containers only on machines that contain the image if pull policy is set to 'never'.
func (s *RollingStrategy) planReplicated(svc *api.Service, spec api.ServiceSpec) (Plan, error) {
	plan, err := newEmptyPlan(svc, spec)
	if err != nil {
		return plan, err
	}

	sched := scheduler.NewServiceScheduler(s.state, spec)
	// TODO: return a detailed report on required constraints and which ones are satisfied?
	availableMachines, err := sched.EligibleMachines()
	if err != nil {
		return plan, err
	}

	var matchedMachines []*pb.MachineInfo
	for _, m := range availableMachines {
		matchedMachines = append(matchedMachines, m.Info)
	}

	// Randomise the order of machines to avoid always deploying to the same machines first.
	rand.Shuffle(len(matchedMachines), func(i, j int) {
		matchedMachines[i], matchedMachines[j] = matchedMachines[j], matchedMachines[i]
	})

	// Organise existing containers by machine.
	containersOnMachine := make(map[string][]api.ServiceContainer)
	upToDateContainersOnMachine := make(map[string]int)
	containerSpecStatuses := make(map[string]ContainerSpecStatus)
	if svc != nil {
		for _, c := range svc.Containers {
			if !c.Container.State.Running || c.Container.State.Paused {
				// Skip containers that are not running.
				continue
			}

			var status ContainerSpecStatus
			if s.ForceRecreate {
				status = ContainerNeedsRecreate
			} else {
				status = EvalContainerSpecChange(c.Container.ServiceSpec, spec)
			}
			containerSpecStatuses[c.Container.ID] = status

			if status == ContainerUpToDate {
				upToDateContainersOnMachine[c.MachineID] += 1
			}
		}

		// Sort containers such that running containers with the desired spec are first.
		slices.SortFunc(svc.Containers, func(c1, c2 api.MachineServiceContainer) int {
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
		slices.SortFunc(matchedMachines, func(m1, m2 *pb.MachineInfo) int {
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
		m := matchedMachines[i%len(matchedMachines)]
		containers := containersOnMachine[m.Id]

		if len(containers) == 0 {
			// No more existing containers on this machine, create a new one.
			plan.Operations = append(plan.Operations, &operation.RunContainerOperation{
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
		}

		// Replace the old container with a new one.
		order := determineUpdateOrder(ctr, spec)
		plan.Operations = append(plan.Operations, &operation.ReplaceContainerOperation{
			ServiceID:    plan.ServiceID,
			Spec:         spec,
			MachineID:    m.Id,
			OldContainer: ctr,
			Order:        order,
		})
	}

	// Remove any remaining containers that are not needed.
	for mid, containers := range containersOnMachine {
		for _, c := range containers {
			plan.Operations = append(plan.Operations, &operation.RemoveContainerOperation{
				MachineID: mid,
				Container: c,
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
func (s *RollingStrategy) planGlobal(svc *api.Service, spec api.ServiceSpec) (Plan, error) {
	plan, err := newEmptyPlan(svc, spec)
	if err != nil {
		return plan, err
	}

	// Map machineID to service containers on that machine. For the global mode, there should be at most one
	// container per machine but we use a slice to handle multiple containers that may exist due to a bug
	// or interruption in the previous deployment.
	containersOnMachine := make(map[string][]api.MachineServiceContainer)
	if svc != nil {
		for _, c := range svc.Containers {
			containersOnMachine[c.MachineID] = append(containersOnMachine[c.MachineID], c)
		}
	}

	sched := scheduler.NewServiceScheduler(s.state, spec)
	availableMachines, err := sched.EligibleMachines()
	if err != nil {
		return plan, err
	}

	for _, m := range availableMachines {
		containers := containersOnMachine[m.Info.Id]
		ops, err := reconcileGlobalContainer(containers, spec, plan.ServiceID, m.Info.Id, s.ForceRecreate)
		if err != nil {
			return plan, err
		}
		plan.Operations = append(plan.Operations, ops...)

		delete(containersOnMachine, m.Info.Id)
	}

	// Remove any remaining containers on machines that don't match the new placement constraints.
	for _, containers := range containersOnMachine {
		for _, c := range containers {
			plan.Operations = append(plan.Operations, &operation.RemoveContainerOperation{
				MachineID: c.MachineID,
				Container: c.Container,
			})
		}
	}

	return plan, nil
}

// reconcileGlobalContainer returns a sequence of operations to reconcile containers on a machine for a global service.
// It ensures exactly one container with the desired spec is running on the machine by creating a new container and
// removing old ones. If there is a host port conflict, it stops the old container before starting a new one.
func reconcileGlobalContainer(
	containers []api.MachineServiceContainer, spec api.ServiceSpec, serviceID, machineID string, forceRecreate bool,
) ([]operation.Operation, error) {
	var ops []operation.Operation

	if len(containers) == 0 {
		// No containers on this machine, create a new one.
		ops = append(ops, &operation.RunContainerOperation{
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

		var status ContainerSpecStatus
		if forceRecreate {
			status = ContainerNeedsRecreate
		} else {
			status = EvalContainerSpecChange(c.Container.ServiceSpec, spec)
		}

		if status == ContainerUpToDate {
			// The container is already running with the same spec.
			upToDate = true
			for j, old := range containers {
				if i == j {
					continue
				}
				ops = append(ops, &operation.RemoveContainerOperation{
					MachineID: old.MachineID,
					Container: old.Container,
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
	// Find the first running container to replace (there should typically be only one).
	var containerToReplace *api.MachineServiceContainer
	for i, c := range containers {
		if c.Container.State.Running {
			containerToReplace = &containers[i]
			break
		}
	}

	if containerToReplace != nil {
		// Stop any other running containers that have conflicting ports before replacing the container.
		// This handles the edge case where multiple running containers exist (due to bugs or interrupted deployments)
		// and more than one has ports that conflict with the new spec.
		for _, c := range containers {
			if c.Container.ID == containerToReplace.Container.ID || !c.Container.State.Running {
				continue
			}
			conflictingPorts, err := c.Container.ConflictingServicePorts(spec.Ports)
			if err != nil || len(conflictingPorts) > 0 {
				ops = append(ops, &operation.StopContainerOperation{
					ServiceID:   serviceID,
					ContainerID: c.Container.ID,
					MachineID:   machineID,
				})
			}
		}

		// Replace the running container with a new one.
		order := determineUpdateOrder(containerToReplace.Container, spec)
		ops = append(ops, &operation.ReplaceContainerOperation{
			ServiceID:    serviceID,
			Spec:         spec,
			MachineID:    machineID,
			OldContainer: containerToReplace.Container,
			Order:        order,
		})

		// Remove any other containers (there shouldn't be any in normal operation).
		for _, c := range containers {
			if c.Container.ID == containerToReplace.Container.ID {
				continue
			}
			ops = append(ops, &operation.RemoveContainerOperation{
				MachineID: c.MachineID,
				Container: c.Container,
			})
		}
	} else {
		// No running containers, create a new one and remove all stopped containers.
		ops = append(ops, &operation.RunContainerOperation{
			ServiceID: serviceID,
			Spec:      spec,
			MachineID: machineID,
		})
		for _, c := range containers {
			ops = append(ops, &operation.RemoveContainerOperation{
				MachineID: c.MachineID,
				Container: c.Container,
			})
		}
	}

	return ops, nil
}

// determineUpdateOrder determines the update order for replacing a container based on the service spec
// and current container state. The order can be explicitly set in UpdateConfig, or automatically determined:
// - If the user explicitly set order, respect it
// - Services with port conflicts require stop-first (ports must be freed first)
// - Single-replica services with data volumes default to stop-first (prevents data corruption)
// - Multi-replica services use start-first (concurrent access already happening)
// - All other services default to start-first (minimizes downtime)
func determineUpdateOrder(oldContainer api.ServiceContainer, spec api.ServiceSpec) string {
	// User explicitly set order - respect it
	if spec.UpdateConfig.Order != "" {
		return spec.UpdateConfig.Order
	}

	// Port conflicts require stop-first
	conflictingPorts, err := oldContainer.ConflictingServicePorts(spec.Ports)
	if err != nil || len(conflictingPorts) > 0 {
		return api.UpdateOrderStopFirst
	}

	// Single-replica services with data volumes default to stop-first to prevent data corruption.
	// Multi-replica services already have concurrent access, so start-first is safe.
	if spec.Replicas <= 1 && len(spec.MountedDockerVolumes()) > 0 {
		return api.UpdateOrderStopFirst
	}

	// Default: start-first for minimal downtime
	return api.UpdateOrderStartFirst
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

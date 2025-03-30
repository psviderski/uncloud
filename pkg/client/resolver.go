package client

import (
	"context"
	"fmt"

	"github.com/psviderski/uncloud/pkg/api"
)

// MapNameResolver resolves machine and container IDs to their names using a static map.
type MapNameResolver struct {
	machines   map[string]string
	containers map[string]string
}

func NewNameResolver(machines, containers map[string]string) *MapNameResolver {
	return &MapNameResolver{
		machines:   machines,
		containers: containers,
	}
}

func (r *MapNameResolver) MachineName(machineID string) string {
	if name, ok := r.machines[machineID]; ok {
		return name
	}
	return machineID
}

func (r *MapNameResolver) ContainerName(containerID string) string {
	if name, ok := r.containers[containerID]; ok {
		return name
	}
	return containerID
}

// ServiceOperationNameResolver returns a machine and container name resolver for a service that can be used to format
// deployment operations.
func (cli *Client) ServiceOperationNameResolver(ctx context.Context, svc api.Service) (*MapNameResolver, error) {
	machines, err := cli.ListMachines(ctx)
	if err != nil {
		return nil, fmt.Errorf("list machines: %w", err)
	}
	machineNames := make(map[string]string, len(machines))
	for _, m := range machines {
		machineNames[m.Machine.Id] = m.Machine.Name
	}
	containerNames := make(map[string]string, len(svc.Containers))
	for _, c := range svc.Containers {
		containerNames[c.Container.ID] = c.Container.Name
	}

	return NewNameResolver(machineNames, containerNames), nil
}

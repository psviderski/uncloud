package ucind

import (
	"context"
	"errors"
	"fmt"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
)

const (
	ClusterNameLabel = "ucind.cluster.name"
	ManagedLabel     = "ucind.managed"
)

type Cluster struct {
	Name     string
	Machines []Machine
}

type CreateClusterOptions struct {
	Machines int
}

func (p *Provisioner) CreateCluster(ctx context.Context, name string, opts CreateClusterOptions) (Cluster, error) {
	var c Cluster

	_, err := p.InspectCluster(ctx, name)
	if err == nil {
		return c, fmt.Errorf("cluster with name '%s' already exists", name)
	}
	if !errors.Is(err, ErrNotFound) {
		return c, fmt.Errorf("inspect cluster '%s': %w", name, err)
	}

	netOpts := network.CreateOptions{
		Labels: map[string]string{
			ClusterNameLabel: name,
			ManagedLabel:     "",
		},
	}
	// Create a Docker network with the same as the cluster name.
	if _, err = p.client.NetworkCreate(ctx, name, netOpts); err != nil {
		return c, fmt.Errorf("create Docker network '%s': %w", name, err)
	}
	c.Name = name

	// Create machines (containers) in the created cluster network.
	for i := 1; i < opts.Machines+1; i++ {
		mopts := CreateMachineOptions{
			Name: fmt.Sprintf("machine-%d", i),
		}
		m, err := p.CreateMachine(ctx, name, mopts)
		if err != nil {
			return c, fmt.Errorf("create machine '%s': %w", mopts.Name, err)
		}

		c.Machines = append(c.Machines, m)
	}

	return c, nil
}

func (p *Provisioner) InspectCluster(ctx context.Context, name string) (Cluster, error) {
	var c Cluster

	// Docker network name is the same as the cluster name.
	net, err := p.client.NetworkInspect(ctx, name, network.InspectOptions{})
	if err != nil {
		if client.IsErrNotFound(err) {
			return c, ErrNotFound
		}
		return c, fmt.Errorf("inspect Docker network '%s': %w", name, err)
	}

	if _, ok := net.Labels[ManagedLabel]; !ok {
		// The network with the cluster name exists, but it's not managed by ucind.
		return c, ErrNotFound
	}

	c.Name = name
	// TODO: list containers (machines) with the cluster name label and include them in the cluster struct.

	return c, nil
}

func (p *Provisioner) RemoveCluster(ctx context.Context, name string) error {
	if _, err := p.InspectCluster(ctx, name); err != nil {
		return err
	}

	// Remove all containers (machines) with the cluster name label.
	opts := container.ListOptions{
		All: true,
		Filters: filters.NewArgs(
			filters.Arg("label", ClusterNameLabel+"="+name),
			filters.Arg("label", ManagedLabel),
		),
	}
	containers, err := p.client.ContainerList(ctx, opts)
	if err != nil {
		return fmt.Errorf("list Docker containers with cluster name '%s': %w", name, err)
	}
	for _, c := range containers {
		if err = p.client.ContainerRemove(ctx, c.ID, container.RemoveOptions{Force: true}); err != nil {
			return fmt.Errorf("remove Docker container '%s': %w", c.ID, err)
		}
	}

	if err = p.client.NetworkRemove(ctx, name); err != nil {
		return fmt.Errorf("remove Docker network '%s': %w", name, err)
	}
	return nil
}

package ucind

import (
	"context"
	"errors"
	"fmt"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
)

const (
	ClusterNameLabel = "ucind.cluster.name"
	ManagedLabel     = "ucind.managed"
)

type Cluster struct {
	Name string
}

type CreateClusterOptions struct {
	Machines int
}

func (p *Provisioner) CreateCluster(ctx context.Context, name string, opts CreateClusterOptions) error {
	_, err := p.InspectCluster(ctx, name)
	if err == nil {
		return fmt.Errorf("cluster with name '%s' already exists", name)
	}
	if !errors.Is(err, ErrNotFound) {
		return fmt.Errorf("inspect cluster '%s': %w", name, err)
	}

	netOpts := network.CreateOptions{
		Labels: map[string]string{
			ClusterNameLabel: name,
			ManagedLabel:     "",
		},
	}
	// Docker network name is the same as the cluster name.
	if _, err = p.client.NetworkCreate(ctx, name, netOpts); err != nil {
		return fmt.Errorf("create Docker network '%s': %w", name, err)
	}

	// TODO: create machines (containers) with the cluster name label.

	return nil
}

func (p *Provisioner) InspectCluster(ctx context.Context, name string) (*Cluster, error) {
	// Docker network name is the same as the cluster name.
	net, err := p.client.NetworkInspect(ctx, name, network.InspectOptions{})
	if err != nil {
		if client.IsErrNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("inspect Docker network '%s': %w", name, err)
	}

	if _, ok := net.Labels[ManagedLabel]; !ok {
		// The network with the cluster name exists, but it's not managed by ucind.
		return nil, ErrNotFound
	}

	// TODO: list containers (machines) with the cluster name label and include them in the cluster struct.

	return &Cluster{
		Name: name,
	}, nil
}

func (p *Provisioner) RemoveCluster(ctx context.Context, name string) error {
	if _, err := p.InspectCluster(ctx, name); err != nil {
		return err
	}

	// TODO: remove machines (containers) with the cluster name label.

	if err := p.client.NetworkRemove(ctx, name); err != nil {
		return fmt.Errorf("remove Docker network '%s': %w", name, err)
	}
	return nil
}

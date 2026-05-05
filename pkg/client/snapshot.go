package client

import (
	"context"
	"errors"
	"fmt"

	"github.com/psviderski/uncloud/pkg/api"
)

// ClusterSnapshotOptions specifies which parts of the cluster state to load.
type ClusterSnapshotOptions struct {
	Machines bool
	Services bool
	Domain   bool
}

// ClusterSnapshot is a request-scoped view of cluster state.
type ClusterSnapshot struct {
	Machines api.MachineMembersList
	Services []api.Service
	Domain   string
}

// NewClusterSnapshot loads a request-scoped snapshot from the client.
func (cli *Client) NewClusterSnapshot(ctx context.Context, opts ClusterSnapshotOptions) (*ClusterSnapshot, error) {
	return newClusterSnapshot(ctx, cli, opts)
}

type clusterSnapshotClient interface {
	ListMachines(ctx context.Context, filter *api.MachineFilter) (api.MachineMembersList, error)
	ListServices(ctx context.Context) ([]api.Service, error)
	GetDomain(ctx context.Context) (string, error)
}

func newClusterSnapshot(
	ctx context.Context, cli clusterSnapshotClient, opts ClusterSnapshotOptions,
) (*ClusterSnapshot, error) {
	snapshot := &ClusterSnapshot{}

	if opts.Machines {
		machines, err := cli.ListMachines(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("list machines: %w", err)
		}
		snapshot.Machines = machines
	}

	if opts.Services {
		services, err := cli.ListServices(ctx)
		if err != nil {
			return nil, fmt.Errorf("list services: %w", err)
		}
		snapshot.Services = services
	}

	if opts.Domain {
		domain, err := cli.GetDomain(ctx)
		if err != nil && !errors.Is(err, api.ErrNotFound) {
			return nil, fmt.Errorf("get domain: %w", err)
		}
		snapshot.Domain = domain
	}

	return snapshot, nil
}

// FindServiceByID returns the service matching the ID.
func (s *ClusterSnapshot) FindServiceByID(id string) (api.Service, bool) {
	for _, svc := range s.Services {
		if svc.ID == id {
			return svc, true
		}
	}
	return api.Service{}, false
}

// FindServiceByName returns the service matching the name, or an error if the name is ambiguous.
func (s *ClusterSnapshot) FindServiceByName(name string) (api.Service, bool, error) {
	var matches []api.Service
	for _, svc := range s.Services {
		if svc.Name == name {
			matches = append(matches, svc)
		}
	}

	switch len(matches) {
	case 0:
		return api.Service{}, false, nil
	case 1:
		return matches[0], true, nil
	default:
		return api.Service{}, false, fmt.Errorf("multiple services found with name '%s', use the service ID instead", name)
	}
}

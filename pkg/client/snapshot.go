package client

import (
	"context"
	"errors"
	"fmt"

	"github.com/psviderski/uncloud/pkg/api"
	"golang.org/x/sync/errgroup"
)

// ClusterSnapshotOptions specifies which parts of the cluster state to load.
type ClusterSnapshotOptions struct {
	Services bool
	Domain   bool
}

// ClusterSnapshot is a request-scoped view of cluster state.
type ClusterSnapshot struct {
	Services []api.Service
	Domain   string

	servicesByID   map[string]api.Service
	servicesByName map[string][]api.Service
}

// NewClusterSnapshot loads a request-scoped snapshot from the client.
func (cli *Client) NewClusterSnapshot(ctx context.Context, opts ClusterSnapshotOptions) (*ClusterSnapshot, error) {
	return newClusterSnapshot(ctx, cli, opts)
}

type clusterSnapshotClient interface {
	ListServices(ctx context.Context) ([]api.Service, error)
	GetDomain(ctx context.Context) (string, error)
}

func newClusterSnapshot(
	ctx context.Context, cli clusterSnapshotClient, opts ClusterSnapshotOptions,
) (*ClusterSnapshot, error) {
	snapshot := &ClusterSnapshot{}
	g, gctx := errgroup.WithContext(ctx)

	if opts.Services {
		g.Go(func() error {
			services, err := cli.ListServices(gctx)
			if err != nil {
				return fmt.Errorf("list services: %w", err)
			}
			snapshot.Services = services
			return nil
		})
	}

	if opts.Domain {
		g.Go(func() error {
			domain, err := cli.GetDomain(gctx)
			if err != nil && !errors.Is(err, api.ErrNotFound) {
				return fmt.Errorf("get domain: %w", err)
			}
			snapshot.Domain = domain
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return snapshot, nil
}

func (s *ClusterSnapshot) indexServices() (map[string]api.Service, map[string][]api.Service) {
	if s.servicesByID != nil {
		return s.servicesByID, s.servicesByName
	}
	s.servicesByID = make(map[string]api.Service, len(s.Services))
	s.servicesByName = make(map[string][]api.Service)
	for _, svc := range s.Services {
		s.servicesByID[svc.ID] = svc
		s.servicesByName[svc.Name] = append(s.servicesByName[svc.Name], svc)
	}
	return s.servicesByID, s.servicesByName
}

// FindServiceByID returns the service matching the ID.
// The boolean return value is false if no service matches.
func (s *ClusterSnapshot) FindServiceByID(id string) (api.Service, bool) {
	byID, _ := s.indexServices()
	svc, ok := byID[id]
	return svc, ok
}

// FindServiceByName returns the service matching the name.
// The boolean return value is false if no service matches. An error is returned if the name is ambiguous.
func (s *ClusterSnapshot) FindServiceByName(name string) (api.Service, bool, error) {
	_, byName := s.indexServices()
	matches := byName[name]
	switch len(matches) {
	case 0:
		return api.Service{}, false, nil
	case 1:
		return matches[0], true, nil
	default:
		return api.Service{}, false, fmt.Errorf("multiple services found with name '%s', use the service ID instead", name)
	}
}

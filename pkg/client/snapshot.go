package client

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/pkg/api"
	"golang.org/x/sync/errgroup"
)

// ClusterSnapshotOptions specifies which parts of the cluster state to load.
type ClusterSnapshotOptions struct {
	Machines          bool
	Services          bool
	ServiceNamesOrIDs []string
	Domain            bool
}

// ClusterSnapshot is a request-scoped view of cluster state.
type ClusterSnapshot struct {
	Machines api.MachineMembersList
	Services []api.Service
	Domain   string

	// Indexes built lazily on first lookup. Tests sometimes construct ClusterSnapshot directly,
	// so the lookup methods must tolerate nil index maps.
	machinesByNameOrID map[string]*pb.MachineMember
	servicesByID       map[string]api.Service
	servicesByName     map[string][]api.Service

	machinesLoaded bool
	servicesLoaded bool
	domainLoaded   bool
}

// NewClusterSnapshot loads a request-scoped snapshot from the client.
func (cli *Client) NewClusterSnapshot(ctx context.Context, opts ClusterSnapshotOptions) (*ClusterSnapshot, error) {
	return newClusterSnapshot(ctx, cli, opts)
}

type clusterSnapshotClient interface {
	ListMachines(ctx context.Context, filter *api.MachineFilter) (api.MachineMembersList, error)
	ListServices(ctx context.Context) ([]api.Service, error)
	InspectService(ctx context.Context, id string) (api.Service, error)
	GetDomain(ctx context.Context) (string, error)
}

func newClusterSnapshot(
	ctx context.Context, cli clusterSnapshotClient, opts ClusterSnapshotOptions,
) (*ClusterSnapshot, error) {
	snapshot := &ClusterSnapshot{}
	g, gctx := errgroup.WithContext(ctx)

	if opts.Machines {
		g.Go(func() error {
			machines, err := cli.ListMachines(gctx, nil)
			if err != nil {
				return fmt.Errorf("list machines: %w", err)
			}
			snapshot.Machines = machines
			snapshot.machinesLoaded = true
			return nil
		})
	}

	if opts.Services {
		g.Go(func() error {
			services, err := listSnapshotServices(gctx, cli, opts.ServiceNamesOrIDs)
			if err != nil {
				return fmt.Errorf("list services: %w", err)
			}
			snapshot.Services = services
			snapshot.servicesLoaded = true
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
			snapshot.domainLoaded = true
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return snapshot, nil
}

func listSnapshotServices(
	ctx context.Context, cli clusterSnapshotClient, namesOrIDs []string,
) ([]api.Service, error) {
	if len(namesOrIDs) == 0 {
		return cli.ListServices(ctx)
	}

	if c, ok := cli.(*Client); ok {
		return c.inspectServices(ctx, namesOrIDs)
	}

	servicesByID := make(map[string]api.Service, len(namesOrIDs))
	var mu sync.Mutex
	g, gctx := errgroup.WithContext(ctx)
	for _, nameOrID := range namesOrIDs {
		nameOrID := nameOrID
		g.Go(func() error {
			svc, err := cli.InspectService(gctx, nameOrID)
			if errors.Is(err, api.ErrNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			mu.Lock()
			servicesByID[svc.ID] = svc
			mu.Unlock()
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	services := make([]api.Service, 0, len(servicesByID))
	for _, svc := range servicesByID {
		services = append(services, svc)
	}
	return services, nil
}

func (cli *Client) inspectServices(ctx context.Context, namesOrIDs []string) ([]api.Service, error) {
	machines, err := cli.ListMachines(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("list machines: %w", err)
	}

	servicesByID := make(map[string]api.Service, len(namesOrIDs))
	var mu sync.Mutex
	g, gctx := errgroup.WithContext(ctx)
	for _, nameOrID := range namesOrIDs {
		nameOrID := nameOrID
		g.Go(func() error {
			svc, err := cli.inspectService(gctx, nameOrID, machines)
			if errors.Is(err, api.ErrNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			mu.Lock()
			servicesByID[svc.ID] = svc
			mu.Unlock()
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	services := make([]api.Service, 0, len(servicesByID))
	for _, svc := range servicesByID {
		services = append(services, svc)
	}
	return services, nil
}

func (s *ClusterSnapshot) HasMachines() bool {
	return s.machinesLoaded || s.Machines != nil
}

func (s *ClusterSnapshot) HasServices() bool {
	return s.servicesLoaded || s.Services != nil
}

func (s *ClusterSnapshot) HasDomain() bool {
	return s.domainLoaded || s.Domain != ""
}

func (s *ClusterSnapshot) indexMachines() map[string]*pb.MachineMember {
	if s.machinesByNameOrID != nil {
		return s.machinesByNameOrID
	}
	s.machinesByNameOrID = make(map[string]*pb.MachineMember, len(s.Machines)*2)
	for _, m := range s.Machines {
		if m.Machine == nil {
			continue
		}
		if m.Machine.Id != "" {
			s.machinesByNameOrID[m.Machine.Id] = m
		}
		if m.Machine.Name != "" {
			s.machinesByNameOrID[m.Machine.Name] = m
		}
	}
	return s.machinesByNameOrID
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

// FindMachineByNameOrID returns the machine matching the name or ID, or nil if not found.
func (s *ClusterSnapshot) FindMachineByNameOrID(nameOrID string) *pb.MachineMember {
	return s.indexMachines()[nameOrID]
}

// SelectMachines returns the snapshot machines matching namesOrIDs, or all machines if empty.
func (s *ClusterSnapshot) SelectMachines(namesOrIDs []string) (api.MachineMembersList, error) {
	if len(namesOrIDs) == 0 {
		return s.Machines, nil
	}

	machines := make(api.MachineMembersList, 0, len(namesOrIDs))
	var notFound []string
	for _, nameOrID := range namesOrIDs {
		if m := s.FindMachineByNameOrID(nameOrID); m != nil {
			machines = append(machines, m)
		} else {
			notFound = append(notFound, nameOrID)
		}
	}
	if len(notFound) > 0 {
		return nil, fmt.Errorf("machines not found: %s", strings.Join(notFound, ", "))
	}
	return machines, nil
}

// FindServiceByID returns the service matching the ID.
func (s *ClusterSnapshot) FindServiceByID(id string) (api.Service, bool) {
	byID, _ := s.indexServices()
	svc, ok := byID[id]
	return svc, ok
}

// FindServiceByName returns the service matching the name, or an error if the name is ambiguous.
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

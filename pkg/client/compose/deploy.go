package compose

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/compose-spec/compose-go/v2/graph"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/docker/api/types/volume"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client/deploy"
	"github.com/psviderski/uncloud/pkg/client/deploy/operation"
	"github.com/psviderski/uncloud/pkg/client/deploy/scheduler"
	"golang.org/x/sync/errgroup"
)

type Client interface {
	deploy.Client
	ListServices(ctx context.Context) ([]api.Service, error)
}

type Deployment struct {
	Client       Client
	Project      *types.Project
	SpecResolver *deploy.ServiceSpecResolver
	Strategy     deploy.Strategy
	planning     *planningState
	plan         *Plan
}

func NewDeployment(ctx context.Context, cli Client, project *types.Project) (*Deployment, error) {
	return NewDeploymentWithStrategy(ctx, cli, project, nil)
}

func NewDeploymentWithStrategy(ctx context.Context, cli Client, project *types.Project, strategy deploy.Strategy) (*Deployment, error) {
	planning, err := loadPlanningState(ctx, cli)
	if err != nil {
		return nil, fmt.Errorf("load planning state: %w", err)
	}

	return &Deployment{
		Client:       cli,
		Project:      project,
		SpecResolver: planning.resolver,
		Strategy:     strategy,
		planning:     planning,
	}, nil
}

type planningState struct {
	clusterState *scheduler.ClusterState
	resolver     *deploy.ServiceSpecResolver
	services     map[string][]api.Service
}

func loadPlanningState(ctx context.Context, cli Client) (*planningState, error) {
	state := &planningState{}
	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		clusterState, err := scheduler.InspectClusterState(gctx, cli)
		if err != nil {
			return fmt.Errorf("inspect cluster state: %w", err)
		}
		state.clusterState = clusterState
		return nil
	})

	g.Go(func() error {
		services, err := cli.ListServices(gctx)
		if err != nil {
			return fmt.Errorf("list services: %w", err)
		}

		state.services = make(map[string][]api.Service, len(services))
		for _, svc := range services {
			state.services[svc.Name] = append(state.services[svc.Name], svc)
		}
		return nil
	})

	g.Go(func() error {
		domain, err := cli.GetDomain(gctx)
		if err != nil && !errors.Is(err, api.ErrNotFound) {
			return fmt.Errorf("get domain: %w", err)
		}
		state.resolver = &deploy.ServiceSpecResolver{
			// If the domain is not found (not reserved), an empty domain is used for the resolver.
			ClusterDomain: domain,
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}
	return state, nil
}

func (s *planningState) currentService(name string) (*api.Service, error) {
	matches := s.services[name]
	switch len(matches) {
	case 0:
		return nil, nil
	case 1:
		return &matches[0], nil
	default:
		return nil, fmt.Errorf("multiple services found with name '%s', use the service ID instead", name)
	}
}

func (d *Deployment) Plan(ctx context.Context) (Plan, error) {
	if d.plan != nil {
		return *d.plan, nil
	}
	var plan Plan

	// Generate service specs for all services in the project.
	var serviceSpecs []api.ServiceSpec
	var mu sync.Mutex
	err := graph.InDependencyOrder(ctx, d.Project,
		func(ctx context.Context, name string, _ types.ServiceConfig) error {
			spec, err := d.ServiceSpec(name)
			if err != nil {
				return err
			}
			// The graph is traversed concurrently, so we need to use a mutex to protect the shared slice.
			mu.Lock()
			serviceSpecs = append(serviceSpecs, spec)
			mu.Unlock()
			return nil
		})
	if err != nil {
		return plan, err
	}

	// Check external volumes and plan the creation of missing volumes before deploying services.
	// Updates the planning cluster state with the scheduled volumes.
	volumeOps, err := d.planVolumes(serviceSpecs)
	if err != nil {
		return plan, err
	}
	plan.Volumes = volumeOps

	for _, spec := range serviceSpecs {
		// TODO: properly handle depends_on conditions in the service deployment plan as the first operation.
		currentService, err := d.planning.currentService(spec.Name)
		if err != nil {
			return plan, fmt.Errorf("find current service '%s': %w", spec.Name, err)
		}

		servicePlan, err := deploy.PlanService(d.planning.clusterState, currentService, spec, d.SpecResolver, d.Strategy)
		if err != nil {
			return plan, fmt.Errorf("create deployment plan for service '%s': %w", spec.Name, err)
		}

		// Skip no-op (up-to-date) service plans.
		if len(servicePlan.Operations) > 0 {
			plan.Services = append(plan.Services, &servicePlan)
		}
	}

	d.plan = &plan
	return plan, nil
}

// ServiceSpec returns the service specification for the given compose service that is ready for deployment.
func (d *Deployment) ServiceSpec(name string) (api.ServiceSpec, error) {
	spec, err := ServiceSpecFromCompose(d.Project, name)
	if err != nil {
		return spec, fmt.Errorf("convert compose service '%s' to service spec: %w", name, err)
	}

	return spec, nil
}

// PlanVolumes checks if the external volumes exist and plans the creation of missing volumes.
func (d *Deployment) planVolumes(serviceSpecs []api.ServiceSpec) ([]*operation.CreateVolumeOperation, error) {
	if len(d.Project.Volumes) == 0 {
		// No volumes to check or create.
		return nil, nil
	}

	if err := d.checkExternalVolumesExist(); err != nil {
		return nil, err
	}

	// TODO: The scheduler should ideally work with the resolved service specs to correctly identify eligible machines.
	//  Figure out where the best place to resolve the specs is.
	volumeScheduler, err := scheduler.NewVolumeScheduler(d.planning.clusterState, serviceSpecs)
	if err != nil {
		return nil, fmt.Errorf("init volume scheduler: %w", err)
	}
	scheduledVolumes, err := volumeScheduler.Schedule()
	if err != nil {
		return nil, fmt.Errorf("schedule volumes: %w", err)
	}

	// Generate operations to create scheduled missing volumes.
	var ops []*operation.CreateVolumeOperation
	for machineID, volumes := range scheduledVolumes {
		for _, v := range volumes {
			machineName := machineID
			if m, ok := d.planning.clusterState.Machine(machineID); ok {
				machineName = m.Info.Name
			}

			ops = append(ops, &operation.CreateVolumeOperation{
				MachineID:   machineID,
				MachineName: machineName,
				VolumeSpec:  v,
			})
		}
	}

	return ops, nil
}

// checkExternalVolumesExist checks that all external volumes exist in the cluster.
func (d *Deployment) checkExternalVolumesExist() error {
	var externalNames []string
	for _, v := range d.Project.Volumes {
		if v.External {
			externalNames = append(externalNames, v.Name)
		}
	}

	var notFound []string
	for _, name := range externalNames {
		if !slices.ContainsFunc(d.planning.clusterState.Machines, func(m *scheduler.Machine) bool {
			return slices.ContainsFunc(m.Volumes, func(vol volume.Volume) bool {
				return vol.Name == name
			})
		}) {
			notFound = append(notFound, fmt.Sprintf("'%s'", name))
		}
	}

	if len(notFound) > 0 {
		return fmt.Errorf("external volumes not found: %s", strings.Join(notFound, ", "))
	}
	return nil
}

func (d *Deployment) Run(ctx context.Context) error {
	plan, err := d.Plan(ctx)
	if err != nil {
		return fmt.Errorf("create plan: %w", err)
	}

	return plan.Execute(ctx, d.Client)
}

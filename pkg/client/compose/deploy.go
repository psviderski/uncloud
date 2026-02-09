package compose

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/compose-spec/compose-go/v2/graph"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/docker/api/types/volume"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client/deploy"
	"github.com/psviderski/uncloud/pkg/client/deploy/scheduler"
)

const defaultHealthTimeout = 5 * time.Minute

type Client interface {
	api.DNSClient
	deploy.Client
}

// Plan represents a deployment plan for a compose project.
// It contains volume operations, per-service deployment plans, and the information
// needed to execute them in dependency order with health check waiting.
type Plan struct {
	VolumeOps    []*deploy.CreateVolumeOperation
	ServicePlans map[string]*deploy.Plan // keyed by service name
	project      *types.Project          // for dependency graph traversal
	client       Client                  // for execution and health checks
}

// Execute runs the deployment plan, creating volumes first, then deploying services
// in dependency order with health check waiting between dependent services.
func (p *Plan) Execute(ctx context.Context) error {
	// Create volumes first - they must exist before containers can mount them.
	for _, op := range p.VolumeOps {
		if err := op.Execute(ctx, p.client); err != nil {
			return fmt.Errorf("create volume: %w", err)
		}
	}

	// Deploy services in dependency order with parallelism.
	// graph.InDependencyOrder runs visitors concurrently for services at the same dependency level.
	return graph.InDependencyOrder(ctx, p.project,
		func(ctx context.Context, name string, _ types.ServiceConfig) error {
			plan, ok := p.ServicePlans[name]
			if !ok || len(plan.Operations) == 0 {
				return nil // no-op / up-to-date
			}

			if err := plan.Execute(ctx, p.client); err != nil {
				return err
			}

			// Wait based on the strictest condition any dependent requires.
			condition := p.getDependentCondition(name)
			return p.waitForCondition(ctx, name, condition)
		})
}

// OperationCount returns the total number of operations in the plan.
func (p *Plan) OperationCount() int {
	count := len(p.VolumeOps)
	for _, plan := range p.ServicePlans {
		count += len(plan.Operations)
	}
	return count
}

// getDependentCondition returns the strictest condition any dependent service requires.
// Priority: service_completed_successfully > service_healthy > service_started
func (p *Plan) getDependentCondition(serviceName string) string {
	strictest := ""
	for _, svc := range p.project.Services {
		dep, ok := svc.DependsOn[serviceName]
		if !ok {
			continue
		}
		switch dep.Condition {
		case types.ServiceConditionCompletedSuccessfully:
			// Highest priority - return immediately.
			return types.ServiceConditionCompletedSuccessfully
		case types.ServiceConditionHealthy:
			strictest = types.ServiceConditionHealthy
		}
		// service_started and empty string require no waiting, so we ignore them.
	}
	return strictest
}

// waitForCondition waits for the service to reach the required condition.
func (p *Plan) waitForCondition(ctx context.Context, serviceName, condition string) error {
	switch condition {
	case types.ServiceConditionStarted, "":
		// Service is already started, no need to wait.
		return nil
	case types.ServiceConditionHealthy:
		return p.waitForServiceHealthy(ctx, serviceName)
	case types.ServiceConditionCompletedSuccessfully:
		// TODO: service_completed_successfully requires restart policy support to work properly.
		// Currently all containers use restart: unless-stopped, so they restart immediately after exiting.
		return fmt.Errorf("depends_on condition 'service_completed_successfully' is not yet supported")
	default:
		return nil
	}
}

// waitForServiceHealthy polls until all containers for the service are healthy.
func (p *Plan) waitForServiceHealthy(ctx context.Context, serviceName string) error {
	deadline := time.Now().Add(defaultHealthTimeout)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		service, err := p.client.InspectService(ctx, serviceName)
		if err != nil {
			return fmt.Errorf("inspect service '%s': %w", serviceName, err)
		}

		if allContainersHealthy(service.Containers) {
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for service '%s' to become healthy", serviceName)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// allContainersHealthy returns true if all containers in the list are healthy.
func allContainersHealthy(containers []api.MachineServiceContainer) bool {
	if len(containers) == 0 {
		return false
	}
	for _, mc := range containers {
		if !mc.Container.Healthy() {
			return false
		}
	}
	return true
}

type Deployment struct {
	Client       Client
	Project      *types.Project
	SpecResolver *deploy.ServiceSpecResolver
	Strategy     deploy.Strategy
	state        *scheduler.ClusterState
	plan         *Plan
}

func NewDeployment(ctx context.Context, cli Client, project *types.Project) (*Deployment, error) {
	return NewDeploymentWithStrategy(ctx, cli, project, nil)
}

func NewDeploymentWithStrategy(ctx context.Context, cli Client, project *types.Project, strategy deploy.Strategy) (*Deployment, error) {
	state, err := scheduler.InspectClusterState(ctx, cli)
	if err != nil {
		return nil, fmt.Errorf("inspect cluster state: %w", err)
	}

	domain, err := cli.GetDomain(ctx)
	if err != nil && !errors.Is(err, api.ErrNotFound) {
		return nil, fmt.Errorf("get cluster domain: %w", err)
	}
	resolver := &deploy.ServiceSpecResolver{
		// If the domain is not found (not reserved), an empty domain is used for the resolver.
		ClusterDomain: domain,
	}

	return &Deployment{
		Client:       cli,
		Project:      project,
		SpecResolver: resolver,
		Strategy:     strategy,
		state:        state,
	}, nil
}

func (d *Deployment) Plan(ctx context.Context) (*Plan, error) {
	if d.plan != nil {
		return d.plan, nil
	}

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
		return nil, err
	}

	// Check external volumes and plan the creation of missing volumes before deploying services.
	// Updates the cluster state (d.state) with the scheduled volumes.
	volumeOps, err := d.planVolumes(serviceSpecs)
	if err != nil {
		return nil, err
	}

	// Create deployment plans for each service.
	servicePlans := make(map[string]*deploy.Plan)
	for _, spec := range serviceSpecs {
		// Pass the updated cluster state with the scheduled volumes to the deployment.
		deployment := deploy.NewDeploymentWithClusterState(d.Client, spec, d.Strategy, d.state)
		servicePlan, err := deployment.Plan(ctx)
		if err != nil {
			return nil, fmt.Errorf("create deployment plan for service '%s': %w", spec.Name, err)
		}
		servicePlans[spec.Name] = &servicePlan
	}

	d.plan = &Plan{
		VolumeOps:    volumeOps,
		ServicePlans: servicePlans,
		project:      d.Project,
		client:       d.Client,
	}
	return d.plan, nil
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
func (d *Deployment) planVolumes(serviceSpecs []api.ServiceSpec) ([]*deploy.CreateVolumeOperation, error) {
	if len(d.Project.Volumes) == 0 {
		// No volumes to check or create.
		return nil, nil
	}

	if err := d.checkExternalVolumesExist(); err != nil {
		return nil, err
	}

	// TODO: The scheduler should ideally work with the resolved service specs to correctly identify eligible machines.
	//  Figure out where the best place to resolve the specs is.
	volumeScheduler, err := scheduler.NewVolumeScheduler(d.state, serviceSpecs)
	if err != nil {
		return nil, fmt.Errorf("init volume scheduler: %w", err)
	}
	scheduledVolumes, err := volumeScheduler.Schedule()
	if err != nil {
		return nil, fmt.Errorf("schedule volumes: %w", err)
	}

	// Generate operations to create scheduled missing volumes.
	var ops []*deploy.CreateVolumeOperation
	for machineID, volumes := range scheduledVolumes {
		for _, v := range volumes {
			machineName := machineID
			if m, ok := d.state.Machine(machineID); ok {
				machineName = m.Info.Name
			}

			ops = append(ops, &deploy.CreateVolumeOperation{
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
		if !slices.ContainsFunc(d.state.Machines, func(m *scheduler.Machine) bool {
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
	return plan.Execute(ctx)
}

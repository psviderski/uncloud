package deploy

import (
	"context"
	"errors"
	"fmt"

	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client/deploy/operation"
	"github.com/psviderski/uncloud/pkg/client/deploy/scheduler"
)

type Client interface {
	api.ContainerClient
	api.DNSClient
	api.ImageClient
	api.MachineClient
	api.ServiceClient
	api.VolumeClient
}

// Deployment manages the process of creating or updating a service to match a desired state.
// It coordinates the validation, planning, and execution of deployment operations.
type Deployment struct {
	Service  *api.Service
	Spec     api.ServiceSpec
	Strategy Strategy
	cli      Client
	plan     *Plan
	// state is an optional current and planned cluster state used for scheduling decisions.
	state *scheduler.ClusterState
}

type Plan struct {
	ServiceID   string
	ServiceName string
	operation.SequenceOperation
}

// NewDeployment creates a new deployment for the given service specification.
// If strategy is nil, a default RollingStrategy will be used.
func NewDeployment(cli Client, spec api.ServiceSpec, strategy Strategy) *Deployment {
	if strategy == nil {
		strategy = &RollingStrategy{}
	}

	return &Deployment{
		Spec:     spec,
		Strategy: strategy,
		cli:      cli,
	}
}

// NewDeploymentWithClusterState creates a new deployment like NewDeployment but also with a provided current cluster
// state used for scheduling decisions.
func NewDeploymentWithClusterState(
	cli Client, spec api.ServiceSpec, strategy Strategy, state *scheduler.ClusterState,
) *Deployment {
	d := NewDeployment(cli, spec, strategy)
	d.state = state
	return d
}

// Plan returns a plan of operations to reconcile the service to the desired state.
// If a plan has already been created, the same plan will be returned.
func (d *Deployment) Plan(ctx context.Context) (Plan, error) {
	if d.plan != nil {
		return *d.plan, nil
	}

	// Validate the user-provided spec before resolving it.
	if err := d.Validate(ctx); err != nil {
		return Plan{}, fmt.Errorf("invalid deployment: %w", err)
	}

	clusterDomain, err := d.cli.GetDomain(ctx)
	if err != nil && !errors.Is(err, api.ErrNotFound) {
		return Plan{}, fmt.Errorf("get cluster domain: %w", err)
	}
	specResolver := &ServiceSpecResolver{
		// If the domain is not found (not reserved), an empty domain is used for the resolver.
		ClusterDomain: clusterDomain,
	}

	resolvedSpec, err := specResolver.Resolve(d.Spec)
	if err != nil {
		return Plan{}, fmt.Errorf("resolve service spec: %w", err)
	}

	if d.state == nil {
		d.state, err = scheduler.InspectClusterState(ctx, d.cli)
		if err != nil {
			return Plan{}, fmt.Errorf("inspect cluster state: %w", err)
		}
	}

	plan, err := d.Strategy.Plan(d.state, d.Service, resolvedSpec)
	if err != nil {
		return Plan{}, fmt.Errorf("create plan using %s strategy: %w", d.Strategy.Type(), err)
	}
	d.plan = &plan

	return plan, nil
}

// Validate checks if the deployment specification is valid.
func (d *Deployment) Validate(ctx context.Context) error {
	if err := d.Spec.Validate(); err != nil {
		return fmt.Errorf("invalid service spec: %w", err)
	}

	if d.Service == nil {
		svc, err := d.cli.InspectService(ctx, d.Spec.Name)
		if err == nil {
			d.Service = &svc
		} else if !errors.Is(err, api.ErrNotFound) {
			return fmt.Errorf("inspect service: %w", err)
		}
	}
	// d.Service is nil if the service doesn't exist yet (first deployment).
	if d.Service == nil {
		return nil
	}

	if d.Service.Name != d.Spec.Name {
		return errors.New("service name cannot be changed")
	}

	// Resolve the default mode if not specified.
	mode := d.Spec.Mode
	if mode == "" {
		mode = api.ServiceModeReplicated
	}

	if mode != d.Service.Mode {
		return errors.New("service mode cannot be changed")
	}

	return nil
}

// Run executes the deployment plan and returns the ID of the created or updated service.
// It will create a new plan if one hasn't been created yet. The deployment will either create a new service or update
// the existing one to match the desired specification.
// TODO: forbid to run the same deployment more than once.
func (d *Deployment) Run(ctx context.Context) (Plan, error) {
	plan, err := d.Plan(ctx)
	if err != nil {
		return plan, fmt.Errorf("create plan: %w", err)
	}

	return plan, plan.Execute(ctx, d.cli)
}

package client

import (
	"context"
	"errors"
	"fmt"
	"github.com/psviderski/uncloud/internal/api"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
)

// Deployment manages the process of creating or updating a service to match a desired state.
// It coordinates the validation, planning, and execution of deployment operations.
type Deployment struct {
	Service  *api.Service
	Spec     api.ServiceSpec
	Strategy Strategy
	cli      *Client
	plan     *Plan
}

type Plan struct {
	ServiceID   string
	ServiceName string
	SequenceOperation
}

// MachineFilter determines which machines participate in a deployment operation by returning true for
// machines that should be included.
type MachineFilter func(m *pb.MachineInfo) bool

var ErrNoMatchingMachines = errors.New("no machines match the filter")

// NewDeployment creates a new deployment for the given service specification.
// If strategy is nil, a default RollingStrategy will be used.
// TODO(refactor): do not return error
func (cli *Client) NewDeployment(spec api.ServiceSpec, strategy Strategy) (*Deployment, error) {
	if strategy == nil {
		strategy = &RollingStrategy{}
	}

	return &Deployment{
		Spec:     spec,
		Strategy: strategy,
		cli:      cli,
	}, nil
}

// Plan returns a plan of operations to reconcile the service to the desired state.
// If a plan has already been created, the same plan will be returned.
func (d *Deployment) Plan(ctx context.Context) (Plan, error) {
	if d.plan != nil {
		return *d.plan, nil
	}

	// Validate the new spec before planning.
	if err := d.Validate(ctx); err != nil {
		return Plan{}, fmt.Errorf("invalid deployment: %w", err)
	}

	plan, err := d.Strategy.Plan(ctx, d.cli, d.Service, d.Spec)
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
	if d.Spec.Name == "" {
		return errors.New("service name is required")
	}

	if d.Service == nil {
		svc, err := d.cli.InspectService(ctx, d.Spec.Name)
		if err == nil {
			d.Service = &svc
		} else if !errors.Is(err, ErrNotFound) {
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
	if d.Service.Mode != d.Spec.Mode {
		return errors.New("service mode cannot be changed")
	}
	if d.Spec.Mode == api.ServiceModeReplicated && d.Spec.Replicas < 1 {
		return errors.New("number of replicas must be at least 1")
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

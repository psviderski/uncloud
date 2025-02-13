package client

import (
	"context"
	"errors"
	"fmt"
	"uncloud/internal/api"
)

// Deployment manages the process of creating or updating a service to match a desired state.
// It coordinates the validation, planning, and execution of deployment operations.
type Deployment struct {
	Service  *api.Service
	Spec     api.ServiceSpec
	Strategy Strategy
	cli      *Client
	plan     Operation
}

// NewDeployment creates a new deployment for the given service specification.
// If strategy is nil, a default RollingStrategy will be used.
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
func (d *Deployment) Plan(ctx context.Context) (Operation, error) {
	if d.plan != nil {
		return d.plan, nil
	}

	// Validate the new spec before planning.
	if err := d.Validate(ctx); err != nil {
		return nil, fmt.Errorf("invalid deployment: %w", err)
	}

	plan, err := d.Strategy.Plan(ctx, d.cli, d.Service, d.Spec)
	if err != nil {
		return nil, fmt.Errorf("create plan using %T: %w", d.Strategy, err)
	}
	d.plan = plan

	return plan, nil
}

// Validate checks if the deployment specification is valid.
func (d *Deployment) Validate(ctx context.Context) error {
	if err := d.Spec.Validate(); err != nil {
		return fmt.Errorf("invalid service spec: %w", err)
	}

	if d.Service == nil && d.Spec.Name != "" {
		svc, err := d.cli.InspectService(ctx, d.Spec.Name)
		if err == nil {
			d.Service = &svc
		} else if !errors.Is(err, ErrNotFound) {
			return fmt.Errorf("inspect service: %w", err)
		}
	}
	// d.Service will be nil if the service doesn't exist yet (first deployment).
	if d.Service == nil {
		return nil
	}

	if d.Service.Name != d.Spec.Name {
		return errors.New("service name cannot be changed")
	}
	if d.Service.Mode != d.Spec.Mode {
		return errors.New("service mode cannot be changed")
	}

	return nil
}

// Run executes the deployment plan. It will create a new plan if one hasn't been created yet.
// The deployment will either create a new service or update an existing one to match the desired specification.
func (d *Deployment) Run(ctx context.Context) error {
	plan, err := d.Plan(ctx)
	if err != nil {
		return fmt.Errorf("plan: %w", err)
	}

	return plan.Execute(ctx, d.cli)
}

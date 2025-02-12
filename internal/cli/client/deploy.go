package client

import (
	"context"
	"errors"
	"fmt"
	"uncloud/internal/api"
)

// Strategy defines how a service should be deployed or updated. Different implementations can provide various
// deployment patterns such as rolling updates, blue/green deployments, etc.
type Strategy interface {
	// Plan returns the operation to reconcile the service to the desired state.
	// If the service does not exist (new deployment), svc will be nil.
	Plan(ctx context.Context, cli *Client, svc *api.Service, spec api.ServiceSpec) (Operation, error)
}

// Operation represents a single atomic operation in a deployment process.
// Operations can be composed to form complex deployment strategies.
type Operation interface {
	Execute(ctx context.Context, cli *Client) error
}

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

	fmt.Printf("Service: %v\n", d.Service)
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
	// TODO: create or get the plan, and run it.
	return nil
}

// RollingStrategy implements a rolling update deployment pattern where containers are updated one at a time
// to minimize service disruption.
type RollingStrategy struct{}

func (s *RollingStrategy) Plan(
	ctx context.Context, cli *Client, svc *api.Service, spec api.ServiceSpec,
) (Operation, error) {
	switch spec.Mode {
	case "", api.ServiceModeReplicated:
		return s.planReplicated(ctx, cli, svc, spec)
	case api.ServiceModeGlobal:
		return s.planGlobal(ctx, cli, svc, spec)
	default:
		return nil, fmt.Errorf("unsupported service mode: %s", spec.Mode)
	}
}

// planReplicated creates a plan for a replicated service deployment.
func (s *RollingStrategy) planReplicated(
	ctx context.Context, cli *Client, svc *api.Service, spec api.ServiceSpec,
) (Operation, error) {
	return nil, errors.New("not implemented")
}

// planGlobal creates a plan for a global service deployment.
func (s *RollingStrategy) planGlobal(
	ctx context.Context, cli *Client, svc *api.Service, spec api.ServiceSpec,
) (Operation, error) {
	// TODO
	// - Prepare a map of machineID to the current container
	// - Fetch a list of existing machines
	// - For each machine, check if there is a container on it
	// - If doesn't exist, add a RunContainerOperation
	// - If exists, check if there are host port bindings that conflict with the new spec
	// - If there are conflicts, add a RemoveContainerOperation and a RunContainerOperation
	// - If there are no conflicts, add a RunContainerOperation and a RemoveContainerOperation
	return nil, errors.New("not implemented")
}

// RunContainerOperation creates and starts a new container on a specific machine.
type RunContainerOperation struct {
	Spec      api.ServiceSpec
	MachineID string
}

func (o *RunContainerOperation) Execute(ctx context.Context, cli *Client) error {
	return nil
}

// RemoveContainerOperation stops and removes a container from a specific machine.
type RemoveContainerOperation struct {
	ContainerID string
	MachineID   string
}

func (o *RemoveContainerOperation) Execute(ctx context.Context, cli *Client) error {
	return nil
}

// SequenceOperation is a composite operation that executes a sequence of operations in order.
type SequenceOperation struct {
	Operations []Operation
}

func (o *SequenceOperation) Execute(ctx context.Context, cli *Client) error {
	for _, op := range o.Operations {
		if err := op.Execute(ctx, cli); err != nil {
			return err
		}
	}
	return nil
}

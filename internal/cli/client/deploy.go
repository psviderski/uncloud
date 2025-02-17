package client

import (
	"context"
	"errors"
	"fmt"
	"github.com/distribution/reference"
	"strings"
	"uncloud/internal/api"
	"uncloud/internal/machine/api/pb"
	"uncloud/internal/secret"
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
	ServiceID string
	SequenceOperation
}

// MachineFilter determines which machines participate in a deployment operation by returning true for
// machines that should be included.
type MachineFilter func(m *pb.MachineInfo) bool

// NewDeployment creates a new deployment for the given service specification.
// If strategy is nil, a default RollingStrategy will be used.
func (cli *Client) NewDeployment(spec api.ServiceSpec, strategy Strategy) (*Deployment, error) {
	if err := spec.Validate(); err != nil {
		return nil, fmt.Errorf("invalid service spec: %w", err)
	}
	if spec.Name == "" {
		// Generate a random service name from the image when not provided.
		img, err := reference.ParseDockerRef(spec.Container.Image)
		if err != nil {
			return nil, fmt.Errorf("invalid image: %w", err)
		}
		// Get the image name without the repository and tag/digest parts.
		imageName := reference.FamiliarName(img)
		// Get the last part of the image name (path), e.g. "nginx" from "bitnami/nginx".
		if i := strings.LastIndex(imageName, "/"); i != -1 {
			imageName = imageName[i+1:]
		}
		// Append a random suffix to the image name to generate an optimistically unique service name.
		suffix, err := secret.RandomAlphaNumeric(4)
		if err != nil {
			return nil, fmt.Errorf("generate random suffix: %w", err)
		}
		spec.Name = fmt.Sprintf("%s-%s", imageName, suffix)
	}

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
		return Plan{}, fmt.Errorf("create plan using %T: %w", d.Strategy, err)
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

	return nil
}

// Run executes the deployment plan and returns the ID of the created or updated service.
// It will create a new plan if one hasn't been created yet. The deployment will either create a new service or update
// the existing one to match the desired specification.
// TODO: forbid to run the same deployment more than once.
func (d *Deployment) Run(ctx context.Context) (string, error) {
	plan, err := d.Plan(ctx)
	if err != nil {
		return "", fmt.Errorf("plan: %w", err)
	}

	return plan.ServiceID, plan.Execute(ctx, d.cli)
}

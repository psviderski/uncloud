package client

import (
	"context"
	"errors"
	"fmt"
	"github.com/compose-spec/compose-go/v2/graph"
	"github.com/compose-spec/compose-go/v2/types"
	"uncloud/internal/api"
	"uncloud/internal/compose"
)

func (cli *Client) NewComposeDeployment(ctx context.Context, project *types.Project) (*ComposeDeployment, error) {
	domain, err := cli.GetDomain(ctx)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, fmt.Errorf("get cluster domain: %w", err)
	}

	resolver := &ServiceSpecResolver{
		// If the domain is not found (not reserved), an empty domain is used for the resolver.
		ClusterDomain: domain,
		// TODO: provide an image resolver.
	}

	return &ComposeDeployment{
		Client:       cli,
		Project:      project,
		SpecResolver: resolver,
	}, nil
}

type ComposeDeployment struct {
	Client       *Client
	Project      *types.Project
	SpecResolver *ServiceSpecResolver
	plan         *SequenceOperation
}

func (d *ComposeDeployment) Plan(ctx context.Context) (SequenceOperation, error) {
	if d.plan != nil {
		return *d.plan, nil
	}

	plan := SequenceOperation{}
	err := graph.InDependencyOrder(ctx, d.Project,
		func(ctx context.Context, name string, _ types.ServiceConfig) error {
			spec, err := d.ServiceSpec(name)
			if err != nil {
				return fmt.Errorf("convert compose service '%s' to service spec: %w", name, err)
			}

			// TODO: properly handle dependency conditions in the service deployment plan as the first operation.
			deploy, err := d.Client.NewDeployment(spec, nil)
			if err != nil {
				return fmt.Errorf("create deployment for service '%s': %w", name, err)
			}

			servicePlan, err := deploy.Plan(ctx)
			if err != nil {
				return fmt.Errorf("create deployment plan for service '%s': %w", name, err)
			}

			// Skip no-op (up-to-date) service plans.
			if len(servicePlan.Operations) > 0 {
				plan.Operations = append(plan.Operations, &servicePlan)
			}

			return nil
		})
	if err != nil {
		d.plan = &plan
	}

	return plan, err
}

// ServiceSpec returns the service specification for the given compose service that is ready for deployment.
func (d *ComposeDeployment) ServiceSpec(name string) (api.ServiceSpec, error) {
	service, err := d.Project.GetService(name)
	if err != nil {
		return api.ServiceSpec{}, fmt.Errorf("get config for compose service '%s': %w", name, err)
	}

	spec, err := compose.ServiceSpecFromCompose(name, service)
	if err != nil {
		return spec, fmt.Errorf("convert compose service '%s' to service spec: %w", name, err)
	}

	// TODO: resolve the image to a digest and supported platforms using an image resolver that broadcasts requests
	//  to all machines in the cluster.
	// TODO: configure placement filter based on the supported platforms of the image.
	if err = d.SpecResolver.Resolve(&spec); err != nil {
		return spec, fmt.Errorf("resolve service spec '%s': %w", name, err)
	}

	return spec, nil
}

func (d *ComposeDeployment) Run(ctx context.Context) error {
	plan, err := d.Plan(ctx)
	if err != nil {
		return fmt.Errorf("create plan: %w", err)
	}

	return plan.Execute(ctx, d.Client)
}

package compose

import (
	"context"
	"errors"
	"fmt"

	"github.com/compose-spec/compose-go/v2/graph"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client/deploy"
)

type Client interface {
	api.DNSClient
	deploy.Client
}

type Deployment struct {
	Client       Client
	Project      *types.Project
	SpecResolver *deploy.ServiceSpecResolver
	plan         *deploy.SequenceOperation
}

func NewDeployment(ctx context.Context, cli Client, project *types.Project) (*Deployment, error) {
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
	}, nil
}

func (d *Deployment) Plan(ctx context.Context) (deploy.SequenceOperation, error) {
	if d.plan != nil {
		return *d.plan, nil
	}

	plan := deploy.SequenceOperation{}
	err := graph.InDependencyOrder(ctx, d.Project,
		func(ctx context.Context, name string, _ types.ServiceConfig) error {
			spec, err := d.ServiceSpec(name)
			if err != nil {
				return fmt.Errorf("convert compose service '%s' to service spec: %w", name, err)
			}

			// TODO: properly handle depends_on conditions in the service deployment plan as the first operation.
			deployment := deploy.NewDeployment(d.Client, spec, nil)
			if err != nil {
				return fmt.Errorf("create deployment for service '%s': %w", name, err)
			}

			servicePlan, err := deployment.Plan(ctx)
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
func (d *Deployment) ServiceSpec(name string) (api.ServiceSpec, error) {
	service, err := d.Project.GetService(name)
	if err != nil {
		return api.ServiceSpec{}, fmt.Errorf("get config for compose service '%s': %w", name, err)
	}

	spec, err := ServiceSpecFromCompose(name, service)
	if err != nil {
		return spec, fmt.Errorf("convert compose service '%s' to service spec: %w", name, err)
	}

	// TODO: resolve the image to a digest and supported platforms using an image resolver that broadcasts requests
	//  to all machines in the cluster. If service.PullPolicy is "missing":
	//    - Broadcast request if any machine contains a particular image and resolve it to image@digest.
	//    - If not found, broadcast request to resolve an image using a registry, and resolve it to image@digest.
	// TODO: configure placement filter based on the supported platforms of the image.

	// TODO: maybe instantiate ImageResolver here based on PullPolicy of each service?

	return spec, nil
}

func (d *Deployment) Run(ctx context.Context) error {
	plan, err := d.Plan(ctx)
	if err != nil {
		return fmt.Errorf("create plan: %w", err)
	}

	return plan.Execute(ctx, d.Client)
}

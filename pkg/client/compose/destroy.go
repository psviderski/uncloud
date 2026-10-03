package compose

import (
	"context"
	"fmt"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client/deploy"
	"github.com/psviderski/uncloud/pkg/client/deploy/scheduler"
)

type Destruction struct {
	Client   Client
	Project  *types.Project
	Strategy deploy.Strategy
	state    *scheduler.ClusterState
	plan     *Plan
}

func NewDestruction(ctx context.Context, cli Client, project *types.Project) (*Destruction, error) {
	return NewDestructionWithStrategy(ctx, cli, project, nil)
}

func NewDestructionWithStrategy(ctx context.Context, cli Client, project *types.Project, strategy deploy.Strategy) (*Destruction, error) {
	state, err := scheduler.InspectClusterState(ctx, cli)
	if err != nil {
		return nil, fmt.Errorf("inspect cluster state: %w", err)
	}

	return &Destruction{
		Client:   cli,
		Project:  project,
		Strategy: strategy,
		state:    state,
	}, nil
}

func (d *Destruction) Plan(ctx context.Context) (Plan, error) {
	if d.plan != nil {
		return *d.plan, nil
	}
	var plan Plan

	var serviceSpecs []api.ServiceSpec
	for _, svc := range d.Project.Services {
		spec, err := d.ServiceSpec(svc.Name)
		if err != nil {
			return plan, err
		}
		serviceSpecs = append(serviceSpecs, spec)
	}

	for _, spec := range serviceSpecs {
		deployment := deploy.NewDeploymentWithClusterState(d.Client, spec, d.Strategy, d.state)
		servicePlan, err := deployment.Plan(ctx)
		if err != nil {
			return plan, fmt.Errorf("create deployment plan for service '%s': %w", spec.Name, err)
		}

		if len(servicePlan.Operations) > 0 {
			plan.Services = append(plan.Services, &servicePlan)
		}
	}

	d.plan = &plan
	return plan, nil
}

func (d *Destruction) ServiceSpec(name string) (api.ServiceSpec, error) {
	spec, err := ServiceSpecFromCompose(d.Project, name)
	if err != nil {
		return spec, fmt.Errorf("convert compose service '%s' to service spec: %w", name, err)
	}

	return spec, nil
}

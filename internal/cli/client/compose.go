package client

import (
	"context"
	"fmt"
	"github.com/compose-spec/compose-go/v2/graph"
	"github.com/compose-spec/compose-go/v2/types"
	"uncloud/internal/compose"
)

func PlanComposeDeployment(ctx context.Context, project *types.Project, cli *Client) (SequenceOperation, error) {
	plan := SequenceOperation{}
	err := graph.InDependencyOrder(ctx, project,
		func(ctx context.Context, name string, service types.ServiceConfig) error {
			spec, err := compose.ServiceSpecFromCompose(name, service)
			if err != nil {
				return fmt.Errorf("convert compose service '%s' to service spec: %w", name, err)
			}
			if spec, err = cli.PrepareDeploymentSpec(ctx, spec); err != nil {
				return fmt.Errorf("prepare service '%s' spec ready for deployment: %w", name, err)
			}

			// TODO: properly handle dependency conditions in the service deployment plan as the first operation.
			deploy, err := cli.NewDeployment(spec, nil)
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

	return plan, err
}

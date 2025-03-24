package client

import (
	"context"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client/deploy"
)

// NewDeployment creates a new deployment for the given service specification.
// If strategy is nil, a default deploy.RollingStrategy will be used.
func (cli *Client) NewDeployment(
	ctx context.Context, spec api.ServiceSpec, strategy deploy.Strategy,
) (*deploy.Deployment, error) {
	return deploy.NewDeployment(ctx, cli, spec, strategy)
}

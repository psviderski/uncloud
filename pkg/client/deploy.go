package client

import (
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/deploy"
)

// NewDeployment creates a new deployment for the given service specification.
// If strategy is nil, a default deploy.RollingStrategy will be used.
func (cli *Client) NewDeployment(spec api.ServiceSpec, strategy deploy.Strategy) *deploy.Deployment {
	return deploy.NewDeployment(cli, spec, strategy)
}

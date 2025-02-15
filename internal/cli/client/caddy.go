package client

import (
	"uncloud/internal/api"
)

const CaddyServiceName = "caddy"

// NewCaddyDeployment creates a new deployment for a Caddy reverse proxy service.
// The service is deployed in global mode to all machines in the cluster. If the image is not provided, the latest
// version of the official Caddy Docker image is used.
func (cli *Client) NewCaddyDeployment(image string) (*Deployment, error) {
	if image == "" {
		// TODO: fetch the latest version tag from the Docker Hub registry.
		image = "caddy:latest"
	}

	spec := api.ServiceSpec{
		Container: api.ContainerSpec{
			Command: []string{"caddy", "run", "-c", "/config/caddy.json", "--watch"},
			Image:   image,
			Volumes: []string{"/var/lib/uncloud/caddy:/config"},
		},
		Mode: api.ServiceModeGlobal,
		Name: CaddyServiceName,
		Ports: []api.PortSpec{
			{
				PublishedPort: 80,
				ContainerPort: 80,
				Protocol:      api.ProtocolTCP,
				Mode:          api.PortModeHost,
			},
			{
				PublishedPort: 443,
				ContainerPort: 443,
				Protocol:      api.ProtocolTCP,
				Mode:          api.PortModeHost,
			},
		},
	}

	return cli.NewDeployment(spec, &RollingStrategy{})
}

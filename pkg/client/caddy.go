package client

import (
	"fmt"
	"regexp"

	"github.com/Masterminds/semver"
	"github.com/distribution/reference"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client/deploy"
)

const (
	CaddyServiceName = "caddy"
	// CaddyImage is the official Caddy Docker image on Docker Hub: https://hub.docker.com/_/caddy
	CaddyImage = "caddy"
)

var caddyImageTagRegex = regexp.MustCompile(`^2\.\d+\.\d+$`)

// NewCaddyDeployment creates a new deployment for a Caddy reverse proxy service.
// The service is deployed in global mode to all machines in the cluster. If the image is not provided, the latest
// version of the official Caddy Docker image is used.
func (cli *Client) NewCaddyDeployment(image, config string, placement api.Placement) (*deploy.Deployment, error) {
	if image == "" {
		latest, err := LatestCaddyImage()
		if err != nil {
			return nil, fmt.Errorf("look up latest Caddy image: %w", err)
		}

		image = reference.FamiliarString(latest)
	}

	spec := api.ServiceSpec{
		Container: api.ContainerSpec{
			Command: []string{"caddy", "run", "-c", "/config/Caddyfile"},
			Env: map[string]string{
				"CADDY_ADMIN": "unix//run/caddy/admin.sock",
			},
			Image: image,
			VolumeMounts: []api.VolumeMount{
				{
					VolumeName:    "data",
					ContainerPath: "/config",
				},
				{
					VolumeName:    "data",
					ContainerPath: "/data",
				},
				{
					VolumeName:    "run",
					ContainerPath: "/run/caddy",
				},
			},
		},
		Mode:      api.ServiceModeGlobal,
		Name:      CaddyServiceName,
		Placement: placement,
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
			// Needed for HTTP/3 (QUIC)
			{
				PublishedPort: 443,
				ContainerPort: 443,
				Protocol:      api.ProtocolUDP,
				Mode:          api.PortModeHost,
			},
		},
		Volumes: []api.VolumeSpec{
			{
				Name: "data",
				Type: api.VolumeTypeBind,
				BindOptions: &api.BindOptions{
					HostPath: "/var/lib/uncloud/caddy",
				},
			},
			{
				Name: "run",
				Type: api.VolumeTypeBind,
				BindOptions: &api.BindOptions{
					HostPath:       "/run/uncloud/caddy",
					CreateHostPath: true,
				},
			},
		},
	}

	if config != "" {
		spec.Caddy = &api.CaddySpec{
			Config: config,
		}
	}

	return cli.NewDeployment(spec, nil), nil
}

// LatestCaddyImage returns the latest image of the official Caddy Docker image on Docker Hub.
// The latest image is determined by the latest version tag 2.x.x.
func LatestCaddyImage() (reference.NamedTagged, error) {
	repo, err := name.NewRepository(CaddyImage)
	if err != nil {
		return nil, fmt.Errorf("parse image: %w", err)
	}
	tags, err := remote.List(repo)
	if err != nil {
		return nil, fmt.Errorf("list image tags: %w", err)
	}

	// Default to the 'latest' tag but try to find the latest version tag 2.x.x.
	latestTag := "latest"
	var latestVersion *semver.Version
	for _, t := range tags {
		if !caddyImageTagRegex.MatchString(t) {
			continue
		}

		v, err := semver.NewVersion(t)
		if err != nil {
			continue
		}
		if latestVersion == nil || v.GreaterThan(latestVersion) {
			latestVersion = v
			latestTag = t
		}
	}

	image, err := reference.ParseDockerRef(CaddyImage)
	if err != nil {
		return nil, fmt.Errorf("parse image: %w", err)
	}
	imageWithTag, err := reference.WithTag(image, latestTag)
	if err != nil {
		return nil, fmt.Errorf("set image tag: %w", err)
	}

	return imageWithTag, nil
}

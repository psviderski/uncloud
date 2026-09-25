package client

import (
	"context"
	"fmt"
	"regexp"

	"github.com/Masterminds/semver"
	"github.com/distribution/reference"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/psviderski/uncloud/api/pb"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client/deploy"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	CaddyServiceName = "caddy"
	// CaddyImage is the official Caddy Docker image on Docker Hub: https://hub.docker.com/_/caddy
	CaddyImage = "caddy"
)

var caddyImageTagRegex = regexp.MustCompile(`^2\.\d+\.\d+$`)

// CaddyClient provides Caddy operations over its parent Client's connection.
// The parent client owns the connection and must be used to close it.
type CaddyClient struct {
	// Storage provides low-level access to Caddy's cluster-backed storage.
	Storage pb.CaddyStorageClient
	grpc    pb.CaddyClient
	client  *Client
}

// CaddyConfigOptions controls which machine's saved Caddy configuration is retrieved.
type CaddyConfigOptions struct {
	// Machine is the machine name or ID. If empty, the configuration is retrieved from the machine the client
	// is connected to.
	Machine string
}

// Config retrieves the saved Caddy configuration from the machine selected by opts.
func (c *CaddyClient) Config(ctx context.Context, opts CaddyConfigOptions) (api.CaddyConfig, error) {
	if opts.Machine != "" {
		ctx = ProxySingleMachineContext(ctx, opts.Machine)
	}

	resp, err := c.grpc.GetConfig(ctx, &emptypb.Empty{})
	if err != nil {
		return api.CaddyConfig{}, fmt.Errorf("get Caddy config: %w", err)
	}
	config := api.CaddyConfig{
		Caddyfile:               resp.Caddyfile,
		LastReconciliationError: resp.LastReconciliationError,
	}
	if resp.ModifiedAt != nil {
		if err := resp.ModifiedAt.CheckValid(); err != nil {
			return api.CaddyConfig{}, fmt.Errorf("invalid Caddy config modification timestamp: %w", err)
		}
		config.ModifiedAt = resp.ModifiedAt.AsTime()
	}

	return config, nil
}

// CaddyDeploymentOptions configures a Caddy reverse proxy deployment.
type CaddyDeploymentOptions struct {
	// Image defaults to the latest stable 2.x.x official Caddy image.
	Image string
	// Config contains an optional global Caddyfile.
	Config    string
	Placement api.Placement
}

// NewDeployment creates a new deployment for a Caddy reverse proxy service.
// The service is deployed in global mode to all machines in the cluster. If the image is not provided, the latest
// version of the official Caddy Docker image is used.
func (c *CaddyClient) NewDeployment(ctx context.Context, opts CaddyDeploymentOptions) (*deploy.Deployment, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	image := opts.Image
	if image == "" {
		latest, err := latestCaddyImage(ctx)
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
				{
					VolumeName:    "uncloud-api",
					ContainerPath: "/run/uncloud/api",
					ReadOnly:      true,
				},
			},
		},
		Mode:      api.ServiceModeGlobal,
		Name:      CaddyServiceName,
		Placement: opts.Placement,
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
			// Bind the Uncloud API socket so caddy.storage.uncloud module can use it to store assets in the cluster.
			{
				Name: "uncloud-api",
				Type: api.VolumeTypeBind,
				BindOptions: &api.BindOptions{
					// Mount the parent directory that contains the socket so it lets the container see a replacement
					// socket after the Uncloud daemon restarts (in case it doesn't use a systemd-activated socket).
					HostPath: "/run/uncloud/api",
				},
			},
		},
	}

	if opts.Config != "" {
		spec.Caddy = &api.CaddySpec{
			Config: opts.Config,
		}
	}

	return c.client.NewDeployment(spec, nil), nil
}

// latestCaddyImage returns the latest image of the official Caddy Docker image on Docker Hub.
// The latest image is determined by the latest version tag 2.x.x.
func latestCaddyImage(ctx context.Context) (reference.NamedTagged, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	repo, err := name.NewRepository(CaddyImage)
	if err != nil {
		return nil, fmt.Errorf("parse image: %w", err)
	}
	tags, err := remote.List(repo, remote.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("list image tags: %w", err)
	}

	image, err := reference.ParseDockerRef(CaddyImage)
	if err != nil {
		return nil, fmt.Errorf("parse image: %w", err)
	}
	imageWithTag, err := reference.WithTag(image, latestCaddyTag(tags))
	if err != nil {
		return nil, fmt.Errorf("set image tag: %w", err)
	}

	return imageWithTag, nil
}

// latestCaddyTag selects the newest stable 2.x.x tag, falling back to latest.
func latestCaddyTag(tags []string) string {
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

	return latestTag
}

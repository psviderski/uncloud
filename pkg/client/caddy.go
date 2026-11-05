package client

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Masterminds/semver"
	"github.com/distribution/reference"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/psviderski/uncloud/api/pb"
	"github.com/psviderski/uncloud/internal/version"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client/deploy"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	CaddyServiceName = "caddy"
	// CaddyImage is the Uncloud Caddy image that is deployed by default. It includes the Uncloud storage module for
	// sharing TLS certificates across machines: https://github.com/unlabs-dev/caddy-uncloud
	CaddyImage = "ghcr.io/unlabs-dev/caddy-uncloud"
	// DefaultCaddyConfig is the default global Caddyfile. It must be compatible with all versions of CaddyImage allowed
	// by caddyImageConstraint.
	DefaultCaddyConfig = "{\n\tstorage uncloud\n}"
	// CaddyStorageFileSystem is the name of the default Caddy storage module that stores TLS assets locally.
	CaddyStorageFileSystem = "file_system"
	// CaddyStorageUncloud is the name of the Caddy storage module that stores TLS assets in the cluster.
	CaddyStorageUncloud = "uncloud"
	// MinCaddyDaemonVersion is the minimum daemon version required for 'uc caddy deploy'. Older daemons don't store
	// service labels that mark the service as managed by this command. Distributed locks were introduced in 0.21.0,
	// which is the minimum required by the cluster storage even on machines that don't run the Caddy service.
	MinCaddyDaemonVersion = "0.22.0"

	// officialCaddyImage is the official Caddy Docker image on Docker Hub: https://hub.docker.com/_/caddy
	officialCaddyImage = "docker.io/library/caddy"
)

var (
	// caddyImageConstraint limits CaddyImage to the major version that is known to be compatible with
	// DefaultCaddyConfig.
	caddyImageConstraint = mustNewConstraint("<1.0.0")
	// caddyImageTagRegex matches the exact version tags of CaddyImage, skipping floating tags and image variants.
	caddyImageTagRegex = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
	caddyStorageRegex  = regexp.MustCompile(`(?m)^[ \t]*storage[ \t]+(\S+)`)
)

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

// CaddyDeploymentOptions configures a Caddy reverse proxy deployment. The zero value deploys the latest version of
// CaddyImage with DefaultCaddyConfig to all machines.
type CaddyDeploymentOptions struct {
	// Image defaults to the latest version of CaddyImage. A non-empty value is marked as custom.
	// Custom images must include curl and start Caddy with /etc/caddy/Caddyfile. They must also include the Uncloud
	// storage module if the config uses 'storage uncloud'.
	Image string
	// Config is the global Caddyfile. Nil means DefaultCaddyConfig. Any other value, even empty, is marked as custom.
	Config    *string
	Placement api.Placement
}

// ConfigOrDefault returns the global Caddyfile of the deployment.
func (o CaddyDeploymentOptions) ConfigOrDefault() string {
	if o.Config != nil {
		return *o.Config
	}
	return DefaultCaddyConfig
}

// NewDeployment creates a new managed deployment for a Caddy reverse proxy service.
// The service is deployed in global mode to all machines in the cluster or the machines set in the placement.
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
	config := opts.ConfigOrDefault()
	if err := validateCaddyImageStorage(image, config); err != nil {
		return nil, err
	}

	spec := api.ServiceSpec{
		Container: api.ContainerSpec{
			Env: map[string]string{
				"CADDY_ADMIN": "unix//run/caddy/admin.sock",
			},
			Healthcheck: &api.HealthcheckSpec{
				Test: []string{
					"CMD",
					"curl", "-fsS",
					"-o", "/dev/null",
					"--unix-socket", "/run/caddy/admin.sock",
					"http://localhost/config/",
				},
				Interval:      30 * time.Second,
				Timeout:       5 * time.Second,
				Retries:       3,
				StartPeriod:   10 * time.Second,
				StartInterval: 1 * time.Second,
			},
			Image: image,
			VolumeMounts: []api.VolumeMount{
				{
					VolumeName:    "data",
					ContainerPath: "/etc/caddy",
					ReadOnly:      true,
				},
				{
					// Keep local TLS assets persistent while Caddy uses the default file system storage.
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

	if config != "" {
		spec.Caddy = &api.CaddySpec{
			Config: config,
		}
	}

	spec.Labels = map[string]string{api.LabelCaddyManaged: ""}
	if opts.Image != "" {
		spec.Labels[api.LabelCaddyImageCustom] = ""
	}
	if opts.Config != nil {
		spec.Labels[api.LabelCaddyConfigCustom] = ""
	}

	return c.client.NewDeployment(spec, nil), nil
}

// CheckDaemonVersions returns an error naming the available machines whose daemons are older than
// MinCaddyDaemonVersion. Machines that are down are skipped because a deployment doesn't touch them.
func (c *CaddyClient) CheckDaemonVersions(ctx context.Context) error {
	machines, err := c.client.ListMachines(ctx, &api.MachineFilter{Available: true})
	if err != nil {
		return fmt.Errorf("list machines: %w", err)
	}

	if outdated := outdatedCaddyDaemons(machines); len(outdated) > 0 {
		return fmt.Errorf("deploying Caddy requires Uncloud daemon %s or newer on all machines, upgrade these "+
			"machines and try again: %s. See https://github.com/psviderski/uncloud/releases/latest",
			MinCaddyDaemonVersion, strings.Join(outdated, ", "))
	}
	return nil
}

// outdatedCaddyDaemons returns descriptions of the machines with outdated daemons. Prereleases and development builds
// aren't considered outdated because their versions don't show whether they're older than a stable release.
func outdatedCaddyDaemons(machines api.MachineMembersList) []string {
	minVersion := semver.MustParse(MinCaddyDaemonVersion)

	var outdated []string
	for _, m := range machines {
		v, err := semver.NewVersion(m.Machine.DaemonVersion)
		if err != nil || v.Prerelease() != "" {
			continue
		}
		if v.LessThan(minVersion) {
			outdated = append(outdated, fmt.Sprintf("%s (%s)", m.Machine.Name, m.Machine.DaemonVersion))
		}
	}

	return outdated
}

// CaddyStorageModule returns the name of the storage module set in the global options of a Caddyfile.
func CaddyStorageModule(config string) string {
	if match := caddyStorageRegex.FindStringSubmatch(config); match != nil {
		return match[1]
	}
	return CaddyStorageFileSystem
}

// IsDefaultCaddyImage returns whether the image is any version of CaddyImage.
func IsDefaultCaddyImage(image string) bool {
	return imageName(image) == CaddyImage
}

// IsNewerCaddyImage returns whether the image is a newer version of CaddyImage than the other image.
// It returns false if any of the images isn't a version of CaddyImage.
func IsNewerCaddyImage(image, other string) bool {
	version, otherVersion := caddyImageVersion(image), caddyImageVersion(other)
	return version != nil && otherVersion != nil && version.GreaterThan(otherVersion)
}

// caddyImageVersion returns the version in the image tag if the image is a version of CaddyImage, or nil otherwise.
func caddyImageVersion(image string) *semver.Version {
	named, err := reference.ParseNormalizedNamed(image)
	if err != nil || named.Name() != CaddyImage {
		return nil
	}
	tagged, ok := named.(reference.Tagged)
	if !ok {
		return nil
	}
	version, err := semver.NewVersion(tagged.Tag())
	if err != nil {
		return nil
	}

	return version
}

// imageName returns the normalised name of the image without its tag or digest.
// It returns an empty string if the image reference is invalid.
func imageName(image string) string {
	named, err := reference.ParseNormalizedNamed(image)
	if err != nil {
		return ""
	}
	return named.Name()
}

// validateCaddyImageStorage rejects the official Caddy image with a config that uses the Uncloud storage module
// because it doesn't include this module. Otherwise, a new container could start from the previously saved Caddyfile
// and pass its health check, but its routes would stop updating as Caddy would reject the new config.
func validateCaddyImageStorage(image, config string) error {
	if imageName(image) == officialCaddyImage && CaddyStorageModule(config) == CaddyStorageUncloud {
		return fmt.Errorf("official Caddy image '%s' doesn't support 'storage uncloud' in the global Caddy config: "+
			"use an image that includes the Uncloud storage module or a Caddyfile without this option. "+
			"See %s/concepts/ingress/cluster-storage-for-caddy", image, version.DocsURL)
	}
	return nil
}

// latestCaddyImage returns the latest version of CaddyImage allowed by caddyImageConstraint.
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

// latestCaddyTag selects the newest tag matching caddyImageTagRegex and caddyImageConstraint, falling back to latest.
func latestCaddyTag(tags []string) string {
	latestTag := "latest"
	var latestVersion *semver.Version
	for _, t := range tags {
		if !caddyImageTagRegex.MatchString(t) {
			continue
		}

		v, err := semver.NewVersion(t)
		if err != nil || !caddyImageConstraint.Check(v) {
			continue
		}
		if latestVersion == nil || v.GreaterThan(latestVersion) {
			latestVersion = v
			latestTag = t
		}
	}

	return latestTag
}

// mustNewConstraint parses a version constraint and panics if it's invalid.
func mustNewConstraint(constraint string) *semver.Constraints {
	c, err := semver.NewConstraint(constraint)
	if err != nil {
		panic(fmt.Sprintf("invalid version constraint '%s': %v", constraint, err))
	}
	return c
}

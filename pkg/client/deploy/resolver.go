package deploy

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/distribution/reference"
	"github.com/docker/docker/api/types"
	"github.com/opencontainers/go-digest"
	"github.com/psviderski/uncloud/internal/secret"
	"github.com/psviderski/uncloud/pkg/api"
	"google.golang.org/grpc/codes"
)

// ServiceSpecResolver transforms user-provided service specs into deployment-ready form.
type ServiceSpecResolver struct {
	ClusterDomain string
	ImageResolver *ImageDigestResolver
}

// Resolve transforms a service spec into its fully resolved form ready for deployment.
func (r *ServiceSpecResolver) Resolve(spec api.ServiceSpec) (api.ServiceSpec, error) {
	if err := spec.Validate(); err != nil {
		return spec, fmt.Errorf("invalid service spec: %w", err)
	}

	spec = spec.Clone()

	steps := []func(*api.ServiceSpec) error{
		r.applyDefaults,
		r.resolveServiceName,
		r.expandIngressPorts,
		r.resolveImageDigest,
	}

	for _, step := range steps {
		if err := step(&spec); err != nil {
			return spec, err
		}
	}

	return spec, nil
}

func (r *ServiceSpecResolver) applyDefaults(spec *api.ServiceSpec) error {
	if spec.Mode == "" {
		spec.Mode = api.ServiceModeReplicated
	}
	// Ensure the replicated service has at least one replica.
	if spec.Mode == api.ServiceModeReplicated && spec.Replicas == 0 {
		spec.Replicas = 1
	}

	if spec.Container.PullPolicy == "" {
		spec.Container.PullPolicy = api.PullPolicyMissing
	}

	return nil
}

// resolveServiceName generates a service name from the image when not provided.
func (r *ServiceSpecResolver) resolveServiceName(spec *api.ServiceSpec) error {
	if spec.Name != "" {
		return nil
	}

	var err error
	spec.Name, err = GenerateServiceName(spec.Container.Image)
	return err
}

// expandIngressPorts processes HTTP(S) ingress ports in a service spec by:
// 1. Setting a default hostname (service-name.cluster-domain) for ports without a hostname.
// 2. Duplicating a port with a cluster domain hostname for ports with external domains.
// This ensures every ingress port is accessible via the cluster domain, while preserving any custom domains specified
// by the user.
func (r *ServiceSpecResolver) expandIngressPorts(spec *api.ServiceSpec) error {
	for i, port := range spec.Ports {
		if port.Protocol != api.ProtocolHTTP && port.Protocol != api.ProtocolHTTPS {
			continue
		}

		if port.Hostname == "" {
			if r.ClusterDomain == "" {
				return fmt.Errorf("cluster domain must be reserved to generate hostname for ingress port: %d/%s",
					port.ContainerPort, port.Protocol)
			}
			// Assign the default hostname (service-name.cluster-domain).
			spec.Ports[i].Hostname = fmt.Sprintf("%s.%s", spec.Name, r.ClusterDomain)
		} else {
			if r.ClusterDomain == "" {
				// When no cluster domain is reserved, use only the provided hostname.
				continue
			}

			if strings.HasSuffix(port.Hostname, "."+r.ClusterDomain) {
				// If the hostname is already a cluster subdomain, use as is.
				continue
			}
			// For external domains, duplicate the port with a service-name.cluster-domain hostname so the service
			// can be accessed via both hostnames.
			newPort := port
			newPort.Hostname = fmt.Sprintf("%s.%s", spec.Name, r.ClusterDomain)
			spec.Ports = append(spec.Ports, newPort)
		}
	}

	return nil
}

func (r *ServiceSpecResolver) resolveImageDigest(spec *api.ServiceSpec) error {
	if r.ImageResolver == nil {
		// Skip digest resolution when no resolver is provided.
		return nil
	}

	image, err := r.ImageResolver.Resolve(spec.Container.Image, spec.Container.PullPolicy)
	if err != nil {
		return fmt.Errorf("resolve image digest: %w", err)
	}
	spec.Container.Image = image

	return nil
}

func GenerateServiceName(image string) (string, error) {
	img, err := reference.ParseDockerRef(image)
	if err != nil {
		return "", fmt.Errorf("invalid image '%s': %w", image, err)
	}
	// Get the image name without the repository and tag/digest parts.
	imageName := reference.FamiliarName(img)
	// Get the last part of the image name (path), e.g. "nginx" from "bitnami/nginx".
	if i := strings.LastIndex(imageName, "/"); i != -1 {
		imageName = imageName[i+1:]
	}
	// Append a random suffix to the image name to generate an optimistically unique service name.
	suffix, err := secret.RandomAlphaNumeric(4)
	if err != nil {
		return "", fmt.Errorf("generate random suffix: %w", err)
	}
	return fmt.Sprintf("%s-%s", imageName, suffix), nil
}

type ImageResolverClient interface {
	api.ImageClient
	api.MachineClient
}

// TODO(lhf): as of April 2025, ImageDigestResolver is not used in the codebase and considered more harmful
// than helpful. It's safe to remove it.
type ImageDigestResolver struct {
	Ctx    context.Context
	Client ImageResolverClient
}

// Resolve resolves the image to the image with the digest according to the pull policy:
//   - always: Fetch the latest digest for the image tag in the registry.
//   - missing: Find the latest image matching the tag on any machine and use its digest, if it exists.
//     When there is no matching image on any machine, it behaves like 'always'.
//   - never: !Not implemented! Similar to 'missing' but when there is no matching image on any machine,
//     it returns an error.
//
// If the image is already pinned to a digest, it is returned as is.
func (r *ImageDigestResolver) Resolve(image, policy string) (string, error) {
	if r.Ctx == nil {
		r.Ctx = context.Background()
	}

	ref, err := reference.ParseNormalizedNamed(image)
	if err != nil {
		return "", fmt.Errorf("parse image: %w", err)
	}
	if _, ok := ref.(reference.Canonical); ok {
		// The image is already pinned to a digest.
		return image, nil
	}

	switch policy {
	case api.PullPolicyAlways:
		return r.resolveAlways(image)
	case api.PullPolicyMissing:
		return r.resolveMissing(image)
	case api.PullPolicyNever:
		return "", fmt.Errorf("pull policy '%s' is not supported yet", policy)
	}
	return image, nil
}

// resolveAlways resolves the image to the image with the digest by querying the registry from all machines.
func (r *ImageDigestResolver) resolveAlways(image string) (string, error) {
	// TODO: broadcast to a subset of machines in large clusters to avoid being rate-limited by the registry.
	ctx, _, err := api.ProxyMachinesContext(r.Ctx, r.Client, nil)
	if err != nil {
		return "", fmt.Errorf("create request context to broadcast to all machines: %w", err)
	}

	remoteImages, err := r.Client.InspectRemoteImage(ctx, image)
	if err != nil {
		return "", fmt.Errorf("inspect image '%s' in registry from all machines: %w", image, err)
	}
	if len(remoteImages) == 0 {
		return "", fmt.Errorf("inspect image '%s' in registry from all machines: unexpected empty response", image)
	}

	for _, ri := range remoteImages {
		if ri.Metadata != nil && ri.Metadata.Error != "" {
			// Save the last error to return it if all machines fail to inspect the image.
			err = fmt.Errorf("inspect image '%s' in registry on machine '%s': %s",
				image, ri.Metadata.Machine, ri.Metadata.Error)
			continue
		}

		return reference.FamiliarString(ri.Image.Reference), nil
	}

	return "", err
}

func (r *ImageDigestResolver) resolveMissing(image string) (string, error) {
	ctx, _, err := api.ProxyMachinesContext(r.Ctx, r.Client, nil)
	if err != nil {
		return "", fmt.Errorf("create request context to broadcast to all machines: %w", err)
	}

	machineImages, err := r.Client.InspectImage(ctx, image)
	if err != nil {
		if errors.Is(err, api.ErrNotFound) {
			// If the image is missing on all machines, the 'missing' policy is equivalent to 'always'.
			return r.resolveAlways(image)
		}
		return "", fmt.Errorf("inspect image '%s' on all machines: %w", image, err)
	}

	var availableImages []types.ImageInspect
	for _, mi := range machineImages {
		// Metadata can be nil if the request was proxied to only one machine.
		if mi.Metadata != nil && mi.Metadata.Error != "" {
			if codes.Code(mi.Metadata.Status.Code) != codes.NotFound {
				fmt.Printf("WARNING: failed to inspect image '%s' on machine '%s': %s\n",
					image, mi.Metadata.Machine, mi.Metadata.Error)
			}
			continue
		}

		availableImages = append(availableImages, mi.Image)
	}

	if len(availableImages) == 0 {
		// If the image is missing on all machines, the 'missing' policy is equivalent to 'always'.
		return r.resolveAlways(image)
	}

	// Find the latest image with a RepoDigest.
	var latestDigest digest.Digest
	var latestCreated time.Time
	for _, img := range availableImages {
		if len(img.RepoDigests) == 0 {
			continue
		}

		// TODO: handle multiple RepoDigests. This could happen for example if the same image was pulled twice using
		// 	both its index (multi-arch) digest and manifest (platform-specific) digest:
		//  {
		//    "Id": "sha256:6fee7566e4273ee6078f08e167e36434b35f72152232a5e6f1446288817dabe5",
		//    "RepoTags": [
		//        "traefik/whoami:latest"
		//    ],
		//    "RepoDigests": [
		//        "traefik/whoami@sha256:200689790a0a0ea48ca45992e0450bc26ccab5307375b41c84dfc4f2475937ab",
		//        "traefik/whoami@sha256:4f90b33ddca9c4d4f06527070d6e503b16d71016edea036842be2a84e60c91cb"
		//    ],
		//    ...
		//  }
		//  Should the registry be queried to find out which digest to use?
		repoDigest := img.RepoDigests[0]
		created, err := time.Parse(time.RFC3339Nano, img.Created)
		if err != nil {
			continue
		}

		if created.After(latestCreated) {
			ref, err := reference.ParseNormalizedNamed(repoDigest)
			if err != nil {
				continue
			}
			if c, ok := ref.(reference.Canonical); ok {
				latestDigest = c.Digest()
				latestCreated = created
			}
		}
	}

	if latestDigest != "" {
		return imageWithDigest(image, latestDigest)
	}

	// Don't pin the digest if no RepoDigests were found. This means the available images were not pulled from
	// a registry but built locally or loaded from an archive. In this case, the available images (could be multiple
	// for different platforms) should be copied to other machines to be able to run service containers on them.
	return image, nil
}

// imageWithDigest adds a digest to an image string if it doesn't already contain one.
func imageWithDigest(image string, dig digest.Digest) (string, error) {
	ref, err := reference.ParseNormalizedNamed(image)
	if err != nil {
		return "", fmt.Errorf("parse image: %w", err)
	}

	if _, ok := ref.(reference.Canonical); !ok {
		// Preserves the original tag if present.
		img, err := reference.WithDigest(ref, dig)
		if err != nil {
			return "", fmt.Errorf("add digest to image: %w", err)
		}
		return reference.FamiliarString(img), nil
	}

	return image, nil
}

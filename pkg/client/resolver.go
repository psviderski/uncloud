package client

import (
	"fmt"
	"github.com/distribution/reference"
	"github.com/psviderski/uncloud/internal/secret"
	"github.com/psviderski/uncloud/pkg/api"
	"strings"
)

type ImageDigestResolver interface {
	Resolve(image string) (string, error)
}

// ServiceSpecResolver transforms user-provided service specs into deployment-ready form.
type ServiceSpecResolver struct {
	ClusterDomain string
	ImageResolver ImageDigestResolver
}

// Resolve transforms a service spec into its fully resolved form ready for deployment.
func (r *ServiceSpecResolver) Resolve(spec *api.ServiceSpec) error {
	if err := spec.Validate(); err != nil {
		return fmt.Errorf("invalid service spec: %w", err)
	}

	steps := []func(*api.ServiceSpec) error{
		r.applyDefaults,
		r.resolveServiceName,
		r.resolveImageDigest,
		r.expandIngressPorts,
	}

	for _, step := range steps {
		if err := step(spec); err != nil {
			return err
		}
	}

	return nil
}

func (r *ServiceSpecResolver) applyDefaults(spec *api.ServiceSpec) error {
	if spec.Mode == "" {
		spec.Mode = api.ServiceModeReplicated
	}
	// Ensure the replicated service has at least one replica.
	if spec.Mode == api.ServiceModeReplicated && spec.Replicas == 0 {
		spec.Replicas = 1
	}

	return nil
}

func (r *ServiceSpecResolver) resolveServiceName(spec *api.ServiceSpec) error {
	if spec.Name != "" {
		return nil
	}

	// Generate a random service name from the image when not provided.
	img, err := reference.ParseDockerRef(spec.Container.Image)
	if err != nil {
		return fmt.Errorf("invalid image: %w", err)
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
		return fmt.Errorf("generate random suffix: %w", err)
	}
	spec.Name = fmt.Sprintf("%s-%s", imageName, suffix)

	return nil
}

func (r *ServiceSpecResolver) resolveImageDigest(spec *api.ServiceSpec) error {
	if r.ImageResolver == nil {
		// Skip digest resolution when no resolver is provided.
		return nil
	}

	imageDigest, err := r.ImageResolver.Resolve(spec.Container.Image)
	if err != nil {
		return fmt.Errorf("resolve image digest: %w", err)
	}
	spec.Container.Image = imageDigest

	return nil
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

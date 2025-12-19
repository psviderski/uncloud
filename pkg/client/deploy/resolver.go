package deploy

import (
	"fmt"
	"math/rand/v2"
	"strings"

	"github.com/distribution/reference"
	"github.com/psviderski/uncloud/internal/secret"
	"github.com/psviderski/uncloud/pkg/api"
)

const (
	// TCPPortRangeMin is the minimum port number for random TCP port allocation.
	TCPPortRangeMin = 30000
	// TCPPortRangeMax is the maximum port number for random TCP port allocation.
	TCPPortRangeMax = 39999
)

// ServiceSpecResolver transforms user-provided service specs into deployment-ready form.
type ServiceSpecResolver struct {
	ClusterDomain string
	// UsedTCPPorts is a set of TCP ports already in use by other services. Used to avoid conflicts
	// when allocating random ports for TCP ingress.
	UsedTCPPorts map[uint16]struct{}
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
		r.allocateTCPPorts,
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

// allocateTCPPorts assigns random published ports to TCP ingress ports that don't have one specified.
func (r *ServiceSpecResolver) allocateTCPPorts(spec *api.ServiceSpec) error {
	for i, port := range spec.Ports {
		// Only handle TCP ingress ports without a published port.
		if port.Protocol != api.ProtocolTCP {
			continue
		}
		if port.Mode != "" && port.Mode != api.PortModeIngress {
			continue
		}
		if port.PublishedPort != 0 {
			// User specified a port, use it as-is.
			continue
		}

		// Allocate a random port from the range.
		allocatedPort, err := r.allocateRandomPort()
		if err != nil {
			return fmt.Errorf("allocate TCP port for container port %d: %w", port.ContainerPort, err)
		}
		spec.Ports[i].PublishedPort = allocatedPort
	}
	return nil
}

// allocateRandomPort picks a random available port from the TCP port range.
func (r *ServiceSpecResolver) allocateRandomPort() (uint16, error) {
	rangeSize := TCPPortRangeMax - TCPPortRangeMin + 1

	// If no used ports map, initialize it.
	if r.UsedTCPPorts == nil {
		r.UsedTCPPorts = make(map[uint16]struct{})
	}

	// Check if we've exhausted the range.
	if len(r.UsedTCPPorts) >= rangeSize {
		return 0, fmt.Errorf("no available TCP ports in range %d-%d", TCPPortRangeMin, TCPPortRangeMax)
	}

	// Try random ports until we find an available one.
	// With 10,000 ports in the range, collisions should be rare.
	for attempts := 0; attempts < 100; attempts++ {
		port := uint16(TCPPortRangeMin + rand.IntN(rangeSize))
		if _, used := r.UsedTCPPorts[port]; !used {
			r.UsedTCPPorts[port] = struct{}{}
			return port, nil
		}
	}

	// Fallback: linear scan for an available port.
	for port := uint16(TCPPortRangeMin); port <= TCPPortRangeMax; port++ {
		if _, used := r.UsedTCPPorts[port]; !used {
			r.UsedTCPPorts[port] = struct{}{}
			return port, nil
		}
	}

	return 0, fmt.Errorf("no available TCP ports in range %d-%d", TCPPortRangeMin, TCPPortRangeMax)
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

package compose

import (
	"fmt"
	"net/netip"
	"strconv"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/psviderski/uncloud/pkg/api"
)

const PortsExtensionKey = "x-ports"

type PortsSource []string

// transformServicesPortsExtension transforms both standard 'ports' and 'x-ports' to PortSpecs.
func transformServicesPortsExtension(project *types.Project) (*types.Project, error) {
	return project.WithServicesTransform(func(name string, service types.ServiceConfig) (types.ServiceConfig, error) {
		// Check for mutual exclusivity
		hasStandardPorts := len(service.Ports) > 0
		hasXPorts := service.Extensions[PortsExtensionKey] != nil

		if hasStandardPorts && hasXPorts {
			return service, fmt.Errorf("service %q cannot specify both 'ports' and 'x-ports' directives, use only one", name)
		}

		var (
			specs []api.PortSpec
			err   error
		)

		if hasStandardPorts {
			// Convert standard ports directly to api.PortSpec
			specs, err = convertStandardPortsToPortSpecs(service.Ports)
			if err != nil {
				return service, fmt.Errorf("convert standard ports for service %q: %w", name, err)
			}
		} else if hasXPorts {
			// Use existing x-ports string-based processing for backward compatibility
			var portsSource PortsSource
			var ok bool
			portsSource, ok = service.Extensions[PortsExtensionKey].(PortsSource)
			if !ok {
				return service, nil
			}

			// Parse the port strings using existing logic
			specs, err = transformPortsExtension(portsSource)
			if err != nil {
				return service, err
			}
		} else {
			// No ports specified
			return service, nil
		}

		// Ensure extensions map exists
		if service.Extensions == nil {
			service.Extensions = make(types.Extensions)
		}
		service.Extensions[PortsExtensionKey] = specs
		return service, nil
	})
}

func transformPortsExtension(ports PortsSource) ([]api.PortSpec, error) {
	var specs []api.PortSpec
	for _, port := range ports {
		spec, err := api.ParsePortSpec(port)
		if err != nil {
			return specs, fmt.Errorf("parse port %q: %w", port, err)
		}
		specs = append(specs, spec)
	}

	return specs, nil
}

// expandPortRange expands a port range like "3000-3005" into individual port configs
func expandPortRange(basePort types.ServicePortConfig) ([]types.ServicePortConfig, error) {
	if !strings.Contains(basePort.Published, "-") {
		return []types.ServicePortConfig{basePort}, nil
	}

	parts := strings.Split(basePort.Published, "-")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid port range format %q", basePort.Published)
	}

	startPort, err := strconv.ParseUint(strings.TrimSpace(parts[0]), 10, 16)
	if err != nil {
		return nil, fmt.Errorf("invalid start port in range %q: %w", basePort.Published, err)
	}

	endPort, err := strconv.ParseUint(strings.TrimSpace(parts[1]), 10, 16)
	if err != nil {
		return nil, fmt.Errorf("invalid end port in range %q: %w", basePort.Published, err)
	}

	if startPort >= endPort {
		return nil, fmt.Errorf("invalid port range %q: start port must be less than end port", basePort.Published)
	}

	// Expand range into individual ports
	var expandedPorts []types.ServicePortConfig
	portDiff := endPort - startPort

	for i := uint64(0); i <= portDiff; i++ {
		expandedPort := basePort // Copy the base configuration

		// Set published port
		publishedPort := startPort + i
		expandedPort.Published = strconv.FormatUint(publishedPort, 10)

		// Set target port (aka container port)
		// If target port range matches published port range, expand it too
		// Otherwise, keep the same target port for all
		if basePort.Target != 0 {
			// For ranges, Docker Compose maps ports 1:1 by default
			expandedPort.Target = basePort.Target + uint32(i)
		}

		expandedPorts = append(expandedPorts, expandedPort)
	}

	return expandedPorts, nil
}

// convertServicePortConfigToPortSpec converts a single ServicePortConfig directly to api.PortSpec
func convertServicePortConfigToPortSpec(port types.ServicePortConfig) (api.PortSpec, error) {
	spec := api.PortSpec{
		ContainerPort: uint16(port.Target),
		Protocol:      port.Protocol,
		Mode:          port.Mode,
	}

	// Apply defaults according to Docker Compose specification
	if spec.Protocol == "" {
		spec.Protocol = "tcp"
	}
	if spec.Mode == "" {
		spec.Mode = "ingress"
	}

	// Set published port if specified
	if port.Published != "" {
		publishedPort, err := strconv.ParseUint(port.Published, 10, 16)
		if err != nil {
			return spec, fmt.Errorf("invalid published port %q: %w", port.Published, err)
		}
		spec.PublishedPort = uint16(publishedPort)
	}

	// Set host IP if specified
	if port.HostIP != "" {
		hostIP, err := netip.ParseAddr(port.HostIP)
		if err != nil {
			return spec, fmt.Errorf("invalid host IP %q: %w", port.HostIP, err)
		}
		spec.HostIP = hostIP
	}

	// Validate the resulting spec
	if err := spec.Validate(); err != nil {
		return spec, fmt.Errorf("invalid port configuration: %w", err)
	}

	return spec, nil
}

// convertStandardPortsToPortSpecs converts standard go-compose ports directly to api.PortSpecs
func convertStandardPortsToPortSpecs(ports []types.ServicePortConfig) ([]api.PortSpec, error) {
	var specs []api.PortSpec

	for _, port := range ports {
		// Check if this port uses a range that should be expanded
		expandedPorts, err := expandPortRange(port)
		if err != nil {
			return nil, err
		}

		// Convert each expanded port directly to api.PortSpec
		for _, expandedPort := range expandedPorts {
			spec, err := convertServicePortConfigToPortSpec(expandedPort)
			if err != nil {
				return nil, err
			}
			specs = append(specs, spec)
		}
	}

	return specs, nil
}

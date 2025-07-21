package compose

import (
	"fmt"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/psviderski/uncloud/pkg/api"
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
			return service, fmt.Errorf("service %q cannot specify both 'ports' and 'x-ports' directives, use only one",
				name)
		}

		var (
			specs []api.PortSpec
			err   error
		)

		if hasStandardPorts {
			// Convert standard ports directly to api.PortSpec
			specs, err = convertStandardPortsToPortSpecs(service.Ports)
			if err != nil {
				return service, fmt.Errorf("convert standard 'ports' for service '%s': %w", name, err)
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

		// Ensure extensions map exists before setting the port specs
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

// convertServicePortConfigToPortSpec converts types.ServicePortConfig directly to api.PortSpec
func convertServicePortConfigToPortSpec(port types.ServicePortConfig) (api.PortSpec, error) {
	spec := api.PortSpec{
		ContainerPort: uint16(port.Target),
		Protocol:      port.Protocol,
		Mode:          port.Mode,
	}
	// Compose parser sets the default protocol to "tcp" and mode to "ingress". We still explicitly set these values
	// to avoid relying on implicit behavior and improve code robustness.
	if spec.Protocol == "" {
		spec.Protocol = api.ProtocolTCP
	}
	if spec.Mode == "" {
		spec.Mode = api.PortModeIngress
	}

	// Set published port if specified
	if port.Published != "" {
		if strings.Contains(port.Published, "-") {
			// 'a-b:x' format is not automatically expanded by the compose parser and our PortSpec does not support port
			// ranges for now.
			return spec, fmt.Errorf("port range '%s' for published port is not supported, use a single port",
				port.Published)
		}
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

// convertStandardPortsToPortSpecs converts []types.ServicePortConfig directly to api.PortSpecs.
func convertStandardPortsToPortSpecs(ports []types.ServicePortConfig) ([]api.PortSpec, error) {
	var specs = make([]api.PortSpec, 0, len(ports))

	for _, port := range ports {
		spec, err := convertServicePortConfigToPortSpec(port)
		if err != nil {
			return nil, err
		}
		specs = append(specs, spec)
	}

	return specs, nil
}

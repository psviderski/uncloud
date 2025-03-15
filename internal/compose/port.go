package compose

import (
	"fmt"
	"github.com/compose-spec/compose-go/v2/types"
	"uncloud/internal/api"
)

const PortsExtensionKey = "x-ports"

type PortsSource []string

// TransformServicesPortsExtension transforms the ports extension of all services in the project by replacing a string
// representation of each port with a parsed PortSpec.
func transformServicesPortsExtension(project *types.Project) error {
	for name, service := range project.Services {
		ports, ok := service.Extensions[PortsExtensionKey].(PortsSource)
		if !ok {
			continue
		}

		specs, err := transformPortsExtension(ports)
		if err != nil {
			return err
		}

		service.Extensions[PortsExtensionKey] = specs
		project.Services[name] = service
	}

	return nil
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

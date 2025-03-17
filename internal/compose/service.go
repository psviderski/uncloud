package compose

import (
	"fmt"
	"github.com/compose-spec/compose-go/v2/types"
	"uncloud/internal/api"
)

func ServiceSpecFromCompose(name string, service types.ServiceConfig) (api.ServiceSpec, error) {
	// TODO: resolve the image to a digest and supported platforms using an image resolver that broadcasts requests
	//  to all machines in the cluster.
	// TODO: configure placement filter based on the supported platforms of the image.
	spec := api.ServiceSpec{
		Container: api.ContainerSpec{
			Command: service.Command,
			Image:   service.Image,
			Init:    service.Init,
			// TODO: env
			// TODO: volumes
		},
		Name: name,
	}

	if ports, ok := service.Extensions[PortsExtensionKey].([]api.PortSpec); ok {
		spec.Ports = ports
	}

	if service.Deploy != nil {
		switch service.Deploy.Mode {
		case "global":
			spec.Mode = api.ServiceModeGlobal
		case "", "replicated":
			spec.Mode = api.ServiceModeReplicated
			if service.Deploy.Replicas != nil {
				spec.Replicas = uint(*service.Deploy.Replicas)
			}
		default:
			return spec, fmt.Errorf("unsupported deploy mode: %s", service.Deploy.Mode)
		}
	}

	return spec, nil
}

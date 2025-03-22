package compose

import (
	"fmt"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/psviderski/uncloud/internal/api"
)

func ServiceSpecFromCompose(name string, service types.ServiceConfig) (api.ServiceSpec, error) {
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

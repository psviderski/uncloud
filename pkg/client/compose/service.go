package compose

import (
	"fmt"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/psviderski/uncloud/pkg/api"
)

func ServiceSpecFromCompose(name string, service types.ServiceConfig) (api.ServiceSpec, error) {
	pullPolicy := ""
	switch service.PullPolicy {
	case types.PullPolicyAlways:
		pullPolicy = api.PullPolicyAlways
	case "", types.PullPolicyMissing, types.PullPolicyIfNotPresent:
		pullPolicy = api.PullPolicyMissing
	case types.PullPolicyNever:
		pullPolicy = api.PullPolicyNever
	default:
		return api.ServiceSpec{}, fmt.Errorf("unsupported pull policy: '%s'", service.PullPolicy)
	}

	spec := api.ServiceSpec{
		Container: api.ContainerSpec{
			Command:    service.Command,
			Image:      service.Image,
			Init:       service.Init,
			PullPolicy: pullPolicy,
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

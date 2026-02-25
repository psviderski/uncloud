package compose

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/opencontainers/go-digest"
	"github.com/psviderski/uncloud/pkg/api"
	cdi "tags.cncf.io/container-device-interface/pkg/parser"
)

func ServiceSpecFromCompose(project *types.Project, serviceName string) (api.ServiceSpec, error) {
	service, err := project.GetService(serviceName)
	if err != nil {
		return api.ServiceSpec{}, fmt.Errorf("get config for compose service '%s': %w", serviceName, err)
	}

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

	env := make(map[string]string, len(service.Environment))
	for k, v := range service.Environment {
		if v == nil {
			// nil value means the variable misses a value in the compose file, and it hasn't been resolved
			// to a variable from the local environment running this code.
			continue
		}
		env[k] = *v
	}

	spec := api.ServiceSpec{
		Container: api.ContainerSpec{
			CapAdd:      service.CapAdd,
			CapDrop:     service.CapDrop,
			Command:     service.Command,
			Entrypoint:  service.Entrypoint,
			Env:         env,
			Healthcheck: healthcheckFromCompose(service.HealthCheck),
			Image:       service.Image,
			Init:        service.Init,
			Privileged:  service.Privileged,
			PullPolicy:  pullPolicy,
			Resources:   resourcesFromCompose(service),
			Sysctls:     service.Sysctls,
			User:        service.User,
		},
		Name: serviceName,
		Mode: api.ServiceModeReplicated,
	}

	// Map x-caddy extension to spec.Caddy if specified.
	if caddy, ok := service.Extensions[CaddyExtensionKey].(Caddy); ok && caddy.Config != "" {
		spec.Caddy = &api.CaddySpec{
			Config: caddy.Config,
		}
	}
	if ports, ok := service.Extensions[PortsExtensionKey].([]api.PortSpec); ok {
		spec.Ports = ports
	}

	if machines, ok := service.Extensions[MachinesExtensionKey].(MachinesSource); ok {
		spec.Placement.Machines = machines
	}

	// Map LogDriver if specified
	if service.Logging != nil && service.Logging.Driver != "" {
		spec.Container.LogDriver = &api.LogDriver{
			Name:    service.Logging.Driver,
			Options: service.Logging.Options,
		}
	}

	if service.Scale != nil {
		spec.Replicas = uint(*service.Scale)
	}

	if service.Deploy != nil {
		switch service.Deploy.Mode {
		case "global":
			spec.Mode = api.ServiceModeGlobal
		case "", "replicated":
			if service.Deploy.Replicas != nil {
				spec.Replicas = uint(*service.Deploy.Replicas)
			}
		default:
			return spec, fmt.Errorf("unsupported deploy mode: '%s'", service.Deploy.Mode)
		}

		// Parse update_config.order
		if cfg := service.Deploy.UpdateConfig; cfg != nil {
			switch cfg.Order {
			case "":
				// No order specified, use default behavior.
			case "start-first":
				spec.UpdateConfig.Order = api.UpdateOrderStartFirst
			case "stop-first":
				spec.UpdateConfig.Order = api.UpdateOrderStopFirst
			default:
				return spec, fmt.Errorf("unsupported update_config.order: '%s'", cfg.Order)
			}
		}
	}

	// TODO: can service.tmpfs be handled as tmpfs volume mounts as well?
	volumeSpecs, volumeMounts, err := volumeSpecsFromCompose(project.Volumes, service.Volumes)
	if err != nil {
		return spec, err
	}

	spec.Volumes = volumeSpecs
	spec.Container.VolumeMounts = volumeMounts

	// Parse configs
	configSpecs, configMounts, err := configSpecsFromCompose(project.Configs, service.Configs, project.WorkingDir)
	if err != nil {
		return spec, err
	}

	spec.Configs = configSpecs
	spec.Container.ConfigMounts = configMounts

	return spec, nil
}

func healthcheckFromCompose(hc *types.HealthCheckConfig) *api.HealthcheckSpec {
	if hc == nil {
		return nil
	}
	if hc.Disable {
		return &api.HealthcheckSpec{Disable: true}
	}

	spec := &api.HealthcheckSpec{Test: hc.Test}
	if hc.Interval != nil {
		spec.Interval = time.Duration(*hc.Interval)
	}
	if hc.Timeout != nil {
		spec.Timeout = time.Duration(*hc.Timeout)
	}
	if hc.StartPeriod != nil {
		spec.StartPeriod = time.Duration(*hc.StartPeriod)
	}
	if hc.StartInterval != nil {
		spec.StartInterval = time.Duration(*hc.StartInterval)
	}
	if hc.Retries != nil {
		spec.Retries = uint(*hc.Retries)
	}

	return spec
}

func resourcesFromCompose(service types.ServiceConfig) api.ContainerResources {
	resources := api.ContainerResources{
		CPU:               int64(service.CPUS * 1e9),
		Memory:            int64(service.MemLimit),
		MemoryReservation: int64(service.MemReservation),
		Ulimits:           ulimitsFromCompose(service.Ulimits),
	}

	// Convert device mappings, separating CDI devices from regular device mappings.
	// CDI devices are identified when Source == Target and the source is a qualified CDI name.
	var cdiDeviceNames []string
	for _, dev := range service.Devices {
		if dev.Source == dev.Target && cdi.IsQualifiedName(dev.Source) {
			cdiDeviceNames = append(cdiDeviceNames, dev.Source)
			continue
		}
		resources.Devices = append(resources.Devices, api.DeviceMapping{
			HostPath:          dev.Source,
			ContainerPath:     dev.Target,
			CgroupPermissions: dev.Permissions,
		})
	}
	if len(cdiDeviceNames) > 0 {
		resources.DeviceReservations = append(resources.DeviceReservations, container.DeviceRequest{
			Driver:    "cdi",
			DeviceIDs: cdiDeviceNames,
		})
	}

	// Convert GPU device requests from compose format, appending "gpu" capability.
	resources.DeviceReservations = append(resources.DeviceReservations,
		deviceReservationsFromCompose(service.Gpus, "gpu")...)

	// Map resources from deploy section if specified.
	if service.Deploy != nil {
		if service.Deploy.Resources.Limits != nil {
			if service.Deploy.Resources.Limits.NanoCPUs > 0 {
				// It seems Limits.NanoCPUs is actually not nano CPUs but a CPU fraction.
				resources.CPU = int64(service.Deploy.Resources.Limits.NanoCPUs * 1e9)
			}
			if service.Deploy.Resources.Limits.MemoryBytes > 0 {
				resources.Memory = int64(service.Deploy.Resources.Limits.MemoryBytes)
			}
		}
		if service.Deploy.Resources.Reservations != nil {
			if service.Deploy.Resources.Reservations.MemoryBytes > 0 {
				resources.MemoryReservation = int64(service.Deploy.Resources.Reservations.MemoryBytes)
			}
			// Handle arbitrary device reservations (same structure as Gpus above).
			resources.DeviceReservations = append(resources.DeviceReservations,
				deviceReservationsFromCompose(service.Deploy.Resources.Reservations.Devices)...)
		}
	}

	return resources
}

// Converts compose-go DeviceRequest format to Docker API DeviceRequest format.
// Additional capabilities can be appended via extraCapabilities (e.g., "gpu" for service.Gpus).
func deviceReservationsFromCompose(devices []types.DeviceRequest, extraCapabilities ...string) []container.DeviceRequest {
	if devices == nil {
		return nil
	}

	requests := make([]container.DeviceRequest, 0, len(devices))
	for _, deviceRequest := range devices {
		// Docker expects an OR'd list of AND'd capabilities (e.g. [][]string),
		// but compose-go provides a single AND'd list (e.g. []string).
		capabilities := [][]string{append(deviceRequest.Capabilities, extraCapabilities...)}

		spec := container.DeviceRequest{
			Driver:       deviceRequest.Driver,
			Count:        int(deviceRequest.Count),
			DeviceIDs:    deviceRequest.IDs,
			Capabilities: capabilities,
			Options:      deviceRequest.Options,
		}
		requests = append(requests, spec)
	}
	return requests
}

func volumeSpecsFromCompose(
	volumes types.Volumes, serviceVolumes []types.ServiceVolumeConfig,
) ([]api.VolumeSpec, []api.VolumeMount, error) {
	volumeSpecs := make(map[string]api.VolumeSpec)
	var volumeMounts []api.VolumeMount

	for _, serviceVolume := range serviceVolumes {
		var volSpec api.VolumeSpec

		switch serviceVolume.Type {
		case types.VolumeTypeBind:
			volSpec = bindVolumeSpecFromCompose(serviceVolume)
		case types.VolumeTypeVolume:
			volSpec = dockerVolumeSpecFromCompose(serviceVolume, volumes[serviceVolume.Source])
		case types.VolumeTypeTmpfs:
			volSpec = tmpfsVolumeSpecFromCompose(serviceVolume)
		default:
			return nil, nil, fmt.Errorf("unsupported volume type: '%s'", serviceVolume.Type)
		}

		if existing, ok := volumeSpecs[volSpec.Name]; ok {
			if !existing.Equals(volSpec) {
				return nil, nil, fmt.Errorf("volume '%s' is used multiple times with different options", volSpec.Name)
			}
		} else {
			volumeSpecs[volSpec.Name] = volSpec
		}

		volumeMounts = append(volumeMounts, api.VolumeMount{
			VolumeName:    volSpec.Name,
			ContainerPath: serviceVolume.Target,
			ReadOnly:      serviceVolume.ReadOnly,
		})
	}

	return slices.Collect(maps.Values(volumeSpecs)), volumeMounts, nil
}

func bindVolumeSpecFromCompose(serviceVolume types.ServiceVolumeConfig) api.VolumeSpec {
	// compose-go parser deduplicates volumes by the target path so it's safe to use it as the unique name.
	name := "bind-" + digest.SHA256.FromString(serviceVolume.Target).Encoded()
	spec := api.VolumeSpec{
		Name: name,
		Type: api.VolumeTypeBind,
		BindOptions: &api.BindOptions{
			HostPath: serviceVolume.Source,
		},
	}
	if serviceVolume.Bind != nil {
		spec.BindOptions.CreateHostPath = serviceVolume.Bind.CreateHostPath
		spec.BindOptions.Propagation = mount.Propagation(serviceVolume.Bind.Propagation)
		spec.BindOptions.Recursive = serviceVolume.Bind.Recursive
	}

	return spec
}

func dockerVolumeSpecFromCompose(serviceVolume types.ServiceVolumeConfig, volume types.VolumeConfig) api.VolumeSpec {
	spec := api.VolumeSpec{
		Name: serviceVolume.Source,
		Type: api.VolumeTypeVolume,
		VolumeOptions: &api.VolumeOptions{
			Name: volume.Name,
		},
	}

	if serviceVolume.Volume != nil {
		spec.VolumeOptions.NoCopy = serviceVolume.Volume.NoCopy
		spec.VolumeOptions.SubPath = serviceVolume.Volume.Subpath
	}

	if !volume.External {
		if volume.Driver != "" || len(volume.DriverOpts) > 0 {
			spec.VolumeOptions.Driver = &mount.Driver{
				Name:    volume.Driver,
				Options: volume.DriverOpts,
			}
		}

		labels := mergeLabels(volume.Labels, volume.CustomLabels)
		if len(labels) > 0 {
			spec.VolumeOptions.Labels = labels
		}
	}

	return spec
}

func mergeLabels(labels ...types.Labels) types.Labels {
	merged := types.Labels{}
	for _, l := range labels {
		for k, v := range l {
			merged[k] = v
		}
	}
	return merged
}

func tmpfsVolumeSpecFromCompose(serviceVolume types.ServiceVolumeConfig) api.VolumeSpec {
	// compose-go parser deduplicates volumes by the target path so it's safe to use it as the unique name.
	name := "tmpfs-" + digest.SHA256.FromString(serviceVolume.Target).Encoded()
	spec := api.VolumeSpec{
		Name: name,
		Type: api.VolumeTypeTmpfs,
		TmpfsOptions: &mount.TmpfsOptions{
			SizeBytes: int64(serviceVolume.Tmpfs.Size),
			Mode:      os.FileMode(serviceVolume.Tmpfs.Mode),
		},
	}

	return spec
}

func ulimitsFromCompose(ulimits map[string]*types.UlimitsConfig) map[string]api.Ulimit {
	if len(ulimits) == 0 {
		return nil
	}

	res := make(map[string]api.Ulimit, len(ulimits))
	for name, u := range ulimits {
		soft := u.Soft
		hard := u.Hard
		if u.Single != 0 {
			if soft == 0 {
				soft = u.Single
			}
			if hard == 0 {
				hard = u.Single
			}
		}

		res[name] = api.Ulimit{
			Soft: int64(soft),
			Hard: int64(hard),
		}
	}

	return res
}

// validateServicesExtensions validates extension combinations across all services in the project.
func validateServicesExtensions(project *types.Project) error {
	for _, service := range project.Services {
		// Check for x-caddy and x-ports conflict, unless all ports are host mode.
		hasCaddy := false
		if caddy, ok := service.Extensions[CaddyExtensionKey].(Caddy); ok && caddy.Config != "" {
			hasCaddy = true
		}

		if ports, ok := service.Extensions[PortsExtensionKey].([]api.PortSpec); ok && len(ports) > 0 && hasCaddy {
			// Check if all ports are in host mode.
			hasIngressPort := false
			for _, p := range ports {
				if p.Mode == "" || p.Mode == api.PortModeIngress {
					hasIngressPort = true
					break
				}
			}
			if hasIngressPort {
				return fmt.Errorf("service '%s': ingress ports in 'x-ports' and 'x-caddy' cannot be specified "+
					"simultaneously: Caddy config is auto-generated from ingress ports, use only one of them. "+
					"Host mode ports in 'x-caddy' can be used with 'x-caddy'", service.Name)
			}
		}
	}

	return nil
}

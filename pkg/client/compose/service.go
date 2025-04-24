package compose

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/docker/api/types/mount"
	"github.com/opencontainers/go-digest"
	"github.com/psviderski/uncloud/pkg/api"
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
			Command:    service.Command,
			Entrypoint: service.Entrypoint,
			Env:        env,
			Image:      service.Image,
			Init:       service.Init,
			Privileged: service.Privileged,
			PullPolicy: pullPolicy,
			Resources:  resourcesFromCompose(service),
			User:       service.User,
		},
		Name: serviceName,
		Mode: api.ServiceModeReplicated,
		// TODO: implement and map x-machines to Placement.
	}

	if ports, ok := service.Extensions[PortsExtensionKey].([]api.PortSpec); ok {
		spec.Ports = ports
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
	}

	// TODO: can service.tmpfs be handled as tmpfs volume mounts as well?
	volumeSpecs, volumeMounts, err := volumeSpecsFromCompose(project.Volumes, service.Volumes)
	if err != nil {
		return spec, err
	}

	spec.Volumes = volumeSpecs
	spec.Container.VolumeMounts = volumeMounts

	return spec, nil
}

func resourcesFromCompose(service types.ServiceConfig) api.ContainerResources {
	resources := api.ContainerResources{
		CPU:               int64(service.CPUS * 1e9),
		Memory:            int64(service.MemLimit),
		MemoryReservation: int64(service.MemReservation),
	}

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
		}
	}

	return resources
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
			Name: strings.TrimPrefix(volume.Name, FakeProjectName+"_"),
		},
	}

	if serviceVolume.Volume != nil {
		spec.VolumeOptions.NoCopy = serviceVolume.Volume.NoCopy
		spec.VolumeOptions.SubPath = serviceVolume.Volume.Subpath
	}

	if !volume.External {
		if volume.Driver != "" {
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

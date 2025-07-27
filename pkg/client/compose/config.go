package compose

import (
	"fmt"
	"maps"
	"slices"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/psviderski/uncloud/pkg/api"
)

func configSpecsFromCompose(
	configs types.Configs, serviceConfigs []types.ServiceConfigObjConfig,
) ([]api.ConfigSpec, []api.ConfigMount, error) {
	configSpecs := make(map[string]api.ConfigSpec)
	var configMounts []api.ConfigMount

	for _, serviceConfig := range serviceConfigs {
		var spec api.ConfigSpec

		if projectConfig, exists := configs[serviceConfig.Source]; exists {
			// Project-level config
			spec = api.ConfigSpec{
				Name:     serviceConfig.Source,
				File:     projectConfig.File,
				External: bool(projectConfig.External),
				Labels:   make(map[string]string),
			}
			// Copy labels if they exist
			for k, v := range projectConfig.Labels {
				spec.Labels[k] = v
			}
		} else {
			// Inline config reference
			spec = api.ConfigSpec{
				Name: serviceConfig.Source,
			}
		}

		if existing, ok := configSpecs[spec.Name]; ok {
			if !existing.Equals(spec) {
				return nil, nil, fmt.Errorf("config '%s' is used multiple times with different options", spec.Name)
			}
		} else {
			configSpecs[spec.Name] = spec
		}

		// Create config mount
		target := serviceConfig.Target
		if target == "" {
			target = "/" + serviceConfig.Source // Default mount path
		}

		mount := api.ConfigMount{
			Source: spec.Name,
			Target: target,
			UID:    serviceConfig.UID,
			GID:    serviceConfig.GID,
		}

		if serviceConfig.Mode != nil {
			mode := uint32(*serviceConfig.Mode)
			mount.Mode = &mode
		}

		configMounts = append(configMounts, mount)
	}

	return slices.Collect(maps.Values(configSpecs)), configMounts, nil
}

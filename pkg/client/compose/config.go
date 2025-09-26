package compose

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/psviderski/uncloud/pkg/api"
)

// TODO: add support for short syntax configs
func configSpecsFromCompose(
	configs types.Configs, serviceConfigs []types.ServiceConfigObjConfig, workingDir string,
) ([]api.ConfigSpec, []api.ConfigMount, error) {
	var configSpecs []api.ConfigSpec
	var configMounts []api.ConfigMount

	for _, serviceConfig := range serviceConfigs {
		var spec api.ConfigSpec

		if projectConfig, exists := configs[serviceConfig.Source]; exists {
			if projectConfig.External {
				return nil, nil, fmt.Errorf("external configs are not supported: %s",
					serviceConfig.Source)
			}

			spec = api.ConfigSpec{
				Name:    serviceConfig.Source,
				Content: []byte(projectConfig.Content),
			}

			// If File is specified, read the file contents
			if projectConfig.File != "" {
				configPath := projectConfig.File
				// TODO: handle this in a separate function?
				if !filepath.IsAbs(configPath) {
					configPath = filepath.Join(workingDir, configPath)
				}

				fileContent, err := os.ReadFile(configPath)
				if err != nil {
					return nil, nil, fmt.Errorf("read config from file '%s': %w", projectConfig.File, err)
				}
				spec.Content = fileContent
			}
		} else {
			return nil, nil, fmt.Errorf("config '%s' not found in project configs", serviceConfig.Source)
		}

		configSpecs = append(configSpecs, spec)

		// Create config mount
		target := serviceConfig.Target
		if target == "" {
			target = "/" + serviceConfig.Source // Default mount path
		}

		mount := api.ConfigMount{
			ConfigName:    spec.Name,
			ContainerPath: target,
			Uid:           serviceConfig.UID,
			Gid:           serviceConfig.GID,
		}

		if serviceConfig.Mode != nil {
			mode := os.FileMode(*serviceConfig.Mode)
			mount.Mode = &mode
		}

		configMounts = append(configMounts, mount)
	}

	return configSpecs, configMounts, nil
}

package compose

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/psviderski/uncloud/pkg/api"
)

func secretSpecsFromCompose(secrets types.Secrets, serviceSecrets []types.ServiceSecretConfig, workingDir string) ([]api.SecretSpec, []api.SecretMount, error) {
	var secretSpecs []api.SecretSpec
	var secretMounts []api.SecretMount

	for _, serviceSecret := range serviceSecrets {
		var spec api.SecretSpec

		projectSecret, exists := secrets[serviceSecret.Source]
		if !exists {
			return nil, nil, fmt.Errorf("secret '%s' not found in project secrets", serviceSecret.Source)
		}

		if projectSecret.External {
			return nil, nil, fmt.Errorf("external secrets are not supported: %s", serviceSecret.Source)
		}

		spec = api.SecretSpec{
			ConfigSpec: api.ConfigSpec{
				Name:    serviceSecret.Source,
				Content: []byte(projectSecret.Content),
			},
		}

		// If File is specified, read the file contents
		if projectSecret.File != "" {
			secretPath := projectSecret.File
			// TODO: handle this in a separate function?
			if !filepath.IsAbs(secretPath) {
				secretPath = filepath.Join(workingDir, secretPath)
			}

			fileContent, err := os.ReadFile(secretPath)
			if err != nil {
				return nil, nil, fmt.Errorf("read secret from file '%s': %w", projectSecret.File, err)
			}
			spec.Content = fileContent
		}

		secretSpecs = append(secretSpecs, spec)

		// Create secret mount
		target := serviceSecret.Target
		if target == "" {
			target = "/run/secrets/" + serviceSecret.Source // Default mount path
		}

		mount := api.SecretMount{
			ConfigMount: api.ConfigMount{
				ConfigName:    spec.Name,
				ContainerPath: target,
				Uid:           serviceSecret.UID,
				Gid:           serviceSecret.GID,
			},
		}

		if serviceSecret.Mode != nil {
			mode := os.FileMode(*serviceSecret.Mode)
			mount.Mode = &mode
		}

		secretMounts = append(secretMounts, mount)
	}

	return secretSpecs, secretMounts, nil
}

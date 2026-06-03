package api

import (
	"fmt"
	"path/filepath"
	"sort"
)

// SecretSpec defines a secret object that can be mounted into containers
type SecretSpec struct {
	ConfigSpec
}

func (s *SecretSpec) Validate() error {
	if s.Name == "" {
		return fmt.Errorf("secret name is required")
	}
	return nil
}

// SecretMount defines how a secret is mounted into a container.
type SecretMount struct {
	ConfigMount
}

func (s *SecretMount) Validate() error {
	if s.ConfigName == "" {
		return fmt.Errorf("secret mount source is required")
	}
	if _, err := s.GetNumericUid(); err != nil {
		return err
	}
	if _, err := s.GetNumericGid(); err != nil {
		return err
	}
	if s.ContainerPath != "" && !filepath.IsAbs(s.ContainerPath) {
		return fmt.Errorf("secret container path must be absolute")
	}
	return nil
}

// sortSecretMounts sorts a slice of SecretMount instances.
func sortSecretMounts(mounts []SecretMount) {
	sort.Slice(mounts, func(i, j int) bool {
		return mounts[i].Compare(&mounts[j].ConfigMount) < 0
	})
}

// ValidateSecretsAndMounts takes secret specs and secret mounts and validates that all mounts refer to existing specs
func ValidateSecretsAndMounts(secrets []SecretSpec, mounts []SecretMount) error {
	secretMap := make(map[string]struct{})
	for _, secret := range secrets {
		if err := secret.Validate(); err != nil {
			return fmt.Errorf("invalid secret: %w", err)
		}
		if _, ok := secretMap[secret.Name]; ok {
			return fmt.Errorf("duplicate secret name: '%s'", secret.Name)
		}

		secretMap[secret.Name] = struct{}{}
	}

	for _, mount := range mounts {
		if err := mount.Validate(); err != nil {
			return fmt.Errorf("invalid secret mount: %w", err)
		}
		if _, exists := secretMap[mount.ConfigName]; !exists {
			return fmt.Errorf("secret mount source '%s' does not refer to any defined secret", mount.ConfigName)
		}
	}

	return nil
}

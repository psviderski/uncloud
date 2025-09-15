// Implementation of Config feature from the Compose spec
package api

import (
	"bytes"
	"fmt"
	"os"
)

// ConfigSpec defines a configuration object that can be mounted into containers
type ConfigSpec struct {
	Name string

	// Content of the config when specified inline
	Content []byte `json:",omitempty"`

	// Note: NOT IMPLEMENTED
	// External indicates this config already exists and should not be created
	// External bool `json:",omitempty"`

	// Note: NOT IMPLEMENTED
	// Labels for the config
	// Labels map[string]string `json:",omitempty"`

	// TODO: add support for "environment"
}

func (c *ConfigSpec) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("config name is required")
	}
	return nil
}

// Equals compares two ConfigSpec instances
func (c *ConfigSpec) Equals(other ConfigSpec) bool {
	return c.Name == other.Name &&
		bytes.Equal(c.Content, other.Content)
}

// ConfigMount defines how a config is mounted into a container
type ConfigMount struct {
	// ConfigName references a config defined in ServiceSpec.Configs by its Name field
	ConfigName string
	// ContainerPath is the absolute path where the config is mounted in the container
	ContainerPath string `json:",omitempty"`
	// UID for the mounted config file
	UID string `json:",omitempty"`
	// GID for the mounted config file
	GID string `json:",omitempty"`
	// Mode (file permissions) for the mounted config file
	Mode *os.FileMode `json:",omitempty"`
}

func (c *ConfigMount) Validate() error {
	if c.ConfigName == "" {
		return fmt.Errorf("config mount source is required")
	}
	return nil
}

// ValidateConfigsAndMounts takes config specs and config mounts and validates that all mounts refer to existing specs
func ValidateConfigsAndMounts(configs []ConfigSpec, mounts []ConfigMount) error {
	configMap := make(map[string]struct{})
	for _, cfg := range configs {
		if err := cfg.Validate(); err != nil {
			return fmt.Errorf("invalid config: %w", err)
		}
		if _, ok := configMap[cfg.Name]; ok {
			return fmt.Errorf("duplicate config name: '%s'", cfg.Name)
		}

		configMap[cfg.Name] = struct{}{}
	}

	for _, mount := range mounts {
		if err := mount.Validate(); err != nil {
			return fmt.Errorf("invalid config mount: %w", err)
		}
		if _, exists := configMap[mount.ConfigName]; !exists {
			return fmt.Errorf("config mount source '%s' does not refer to any defined config", mount.ConfigName)
		}
	}

	return nil
}

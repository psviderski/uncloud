// Implementation of Config feature from the Compose spec
package api

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
	// Uid for the mounted config file
	Uid string `json:",omitempty"`
	// Gid for the mounted config file
	Gid string `json:",omitempty"`
	// Mode (file permissions) for the mounted config file
	Mode *os.FileMode `json:",omitempty"`
}

func (c *ConfigMount) GetNumericUid() (*uint64, error) {
	if c.Uid == "" {
		return nil, nil
	}
	uid, err := strconv.ParseUint(c.Uid, 10, 64)
	if err != nil || int(uid) < 0 {
		return nil, fmt.Errorf("invalid Uid '%s': %w", c.Uid, err)
	}
	return &uid, nil
}

func (c *ConfigMount) GetNumericGid() (*uint64, error) {
	if c.Gid == "" {
		return nil, nil
	}
	gid, err := strconv.ParseUint(c.Gid, 10, 64)
	if err != nil || int(gid) < 0 {
		return nil, fmt.Errorf("invalid Gid '%s': %w", c.Gid, err)
	}
	return &gid, nil
}

func (c *ConfigMount) Validate() error {
	if c.ConfigName == "" {
		return fmt.Errorf("config mount source is required")
	}
	if _, err := c.GetNumericUid(); err != nil {
		return err
	}
	if _, err := c.GetNumericGid(); err != nil {
		return err
	}
	if c.ContainerPath != "" && !filepath.IsAbs(c.ContainerPath) {
		return fmt.Errorf("container path must be absolute")
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

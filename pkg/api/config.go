// Implementation of Config feature from the Compose spec
package api

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	if err != nil {
		return nil, fmt.Errorf("invalid Uid '%s': %w", c.Uid, err)
	}
	if int(uid) < 0 {
		return nil, fmt.Errorf("invalid Uid '%s': value too high", c.Uid)
	}
	return &uid, nil
}

func (c *ConfigMount) GetNumericGid() (*uint64, error) {
	if c.Gid == "" {
		return nil, nil
	}
	gid, err := strconv.ParseUint(c.Gid, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid Gid '%s': %w", c.Gid, err)
	}
	if int(gid) < 0 {
		return nil, fmt.Errorf("invalid Gid '%s': value too high", c.Gid)
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

// Compare compares this ConfigMount with another.
// Returns:
//
//	-1 if c < other
//	 0 if c == other
//	+1 if c > other
func (c *ConfigMount) Compare(other *ConfigMount) int {
	if c.ConfigName != other.ConfigName {
		if c.ConfigName < other.ConfigName {
			return -1
		}
		return 1
	}
	if c.ContainerPath != other.ContainerPath {
		if c.ContainerPath < other.ContainerPath {
			return -1
		}
		return 1
	}
	if c.Uid != other.Uid {
		if c.Uid < other.Uid {
			return -1
		}
		return 1
	}
	if c.Gid != other.Gid {
		if c.Gid < other.Gid {
			return -1
		}
		return 1
	}
	// Compare Mode (handle nil cases)
	if c.Mode == nil && other.Mode != nil {
		return -1
	}
	if c.Mode != nil && other.Mode == nil {
		return 1
	}
	if c.Mode != nil && other.Mode != nil {
		if *c.Mode < *other.Mode {
			return -1
		}
		if *c.Mode > *other.Mode {
			return 1
		}
	}
	return 0
}

// Equals compares two ConfigMount instances for equality.
func (c *ConfigMount) Equals(other *ConfigMount) bool {
	return c.Compare(other) == 0
}

func (c *ConfigMount) Clone() ConfigMount {
	clone := *c
	if c.Mode != nil {
		mode := *c.Mode
		clone.Mode = &mode
	}
	return clone
}

// sortConfigMounts sorts a slice of ConfigMount instances.
func sortConfigMounts(mounts []ConfigMount) {
	sort.Slice(mounts, func(i, j int) bool {
		return mounts[i].Compare(&mounts[j]) < 0
	})
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

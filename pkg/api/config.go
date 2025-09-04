// Implementation of Config feature from the Compose spec
package api

// ConfigSpec defines a configuration object that can be mounted into containers
type ConfigSpec struct {
	// Do we need this, if external is not supported yet?
	Name string `json:"name"`

	// File path (when External is false)
	File string `json:"file,omitempty"`

	// Content of the config when specified inline
	Content string `json:"content,omitempty"`

	// Note: NOT IMPLEMENTED
	// External indicates this config already exists and should not be created
	// External bool `json:"external,omitempty"`

	// Note: NOT IMPLEMENTED
	// Labels for the config
	// Labels map[string]string `json:"labels,omitempty"`

	// TODO: add support for "environment"
}

// ConfigMount defines how a config is mounted into a container
type ConfigMount struct {
	// Source is the name of the config
	Source string `json:"source"`

	// Target path inside the container (defaults to /<source> if not specified)
	Target string `json:"target,omitempty"`

	// UID for the mounted config file
	UID string `json:"uid,omitempty"`

	// GID for the mounted config file
	GID string `json:"gid,omitempty"`

	// Mode (file permissions) for the mounted config file
	Mode *uint32 `json:"mode,omitempty"`
}

// Equals compares two ConfigSpec instances
func (c ConfigSpec) Equals(other ConfigSpec) bool {
	return c.Name == other.Name &&
		c.File == other.File &&
		c.Content == other.Content
	// c.External == other.External &&
	// mapsEqual(c.Labels, other.Labels)
}

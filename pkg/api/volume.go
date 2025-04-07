package api

import (
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/mount"
)

const (
	// VolumeTypeBind is the type for mounting a host path.
	VolumeTypeBind = "bind"
	// VolumeTypeVolume is the type for mounting a managed volume.
	VolumeTypeVolume = "volume"
	// VolumeTypeTmpfs is the type for mounting a temporary file system stored in the host memory.
	VolumeTypeTmpfs = "tmpfs"

	// SELinuxShared share the volume content.
	SELinuxShared = "z"
	// SELinuxUnshared label content as private unshared.
	SELinuxUnshared = "Z"
)

// VolumeSpec defines a volume mount specification.
type VolumeSpec struct {
	// Name is the volume name used to reference this volume in container mounts.
	Name          string
	Type          string
	BindOptions   *BindOptions        `json:",omitempty"`
	TmpfsOptions  *mount.TmpfsOptions `json:",omitempty"`
	VolumeOptions *VolumeOptions      `json:",omitempty"`
}

// BindOptions represents options for a bind volume.
type BindOptions struct {
	// HostPath is the absolute path on the host filesystem.
	HostPath string
	// AutoCreate indicates whether the host path should be created if it doesn't exist.
	// If false, deployment will fail if the path doesn't exist.
	AutoCreate  bool              `json:",omitempty"`
	Propagation mount.Propagation `json:",omitempty"`
	SELinux     string            `json:",omitempty"`
}

// VolumeOptions represents options for a managed volume.
type VolumeOptions struct {
	// AutoCreate indicates whether the volume should be created if it doesn't exist.
	// If false, deployment will fail if the volume doesn't exist.
	AutoCreate bool `json:",omitempty"`
	// Driver specifies the volume driver and its options for volume creation (AutoCreate is true).
	Driver *mount.Driver `json:",omitempty"`
	// Labels are key-value metadata to apply to the volume if creating a new volume.
	Labels map[string]string `json:",omitempty"`
	// Name of the managed volume to use. If not specified, defaults to the VolumeSpec.Name.
	Name string `json:",omitempty"`
	// NoCopy prevents automatic copying of data from the container mount path to the volume.
	NoCopy bool `json:",omitempty"`
	// SubPath is the path within the volume to mount instead of its root.
	SubPath string `json:",omitempty"`
}

func (v *VolumeSpec) Validate() error {
	if v.Name == "" {
		return fmt.Errorf("volume name must not be empty")
	}

	switch v.Type {
	case VolumeTypeBind, VolumeTypeVolume, VolumeTypeTmpfs:
	default:
		return fmt.Errorf("invalid volume type: '%s', must be one of '%s', '%s', '%s')",
			v.Type, VolumeTypeBind, VolumeTypeVolume, VolumeTypeTmpfs)
	}

	return nil
}

// VolumeMount defines how a volume is mounted into a container.
type VolumeMount struct {
	// VolumeName references a volume defined in ServiceSpec.Volumes by its Name field.
	VolumeName string
	// ContainerPath is the absolute path where the volume is mounted in the container.
	ContainerPath string
	// ReadOnly indicates whether the volume should be mounted read-only.
	// If false (default), the volume is mounted read-write.
	ReadOnly bool `json:",omitempty"`
}

func (m *VolumeMount) Validate() error {
	if m.VolumeName == "" {
		return fmt.Errorf("volume name must not be empty")
	}

	if !strings.HasPrefix(m.ContainerPath, "/") {
		return fmt.Errorf("invalid container path: '%s', must be an absolute path in the container", m.ContainerPath)
	}

	return nil
}

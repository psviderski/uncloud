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

type VolumeSpec struct {
	Type     string
	Source   string
	Target   string
	ReadOnly bool
	Bind     *VolumeBind
	// TODO: add options for tmpfs.
}

type VolumeBind struct {
	CreateHostPath bool
	Propagation    mount.Propagation
	SELinux        string
}

func (v *VolumeSpec) Validate() error {
	switch v.Type {
	case VolumeTypeBind, VolumeTypeVolume:
	default:
		return fmt.Errorf("invalid volume type: '%s'", v.Type)
	}

	if !strings.HasPrefix(v.Target, "/") {
		return fmt.Errorf("invalid volume target: '%s', must be an absolute path in the container", v.Target)
	}

	return nil
}

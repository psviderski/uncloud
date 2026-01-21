package api

import (
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"

	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/volume"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

const (
	// VolumeTypeBind is the type for mounting a host path.
	VolumeTypeBind = "bind"
	// VolumeTypeVolume is the type for mounting a named Docker volume.
	VolumeTypeVolume = "volume"
	// VolumeTypeTmpfs is the type for mounting a temporary file system stored in the host memory.
	VolumeTypeTmpfs = "tmpfs"

	// VolumeDriverLocal is the default volume driver for local named Docker volumes.
	VolumeDriverLocal = "local"
)

// VolumeSpec defines a volume mount specification. As of April 2025, the volume must be created before deploying
// a service using it.
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
	// CreateHostPath indicates whether the host path should be created if it doesn't exist.
	// If false, deployment will fail if the path doesn't exist.
	CreateHostPath bool              `json:",omitempty"`
	Propagation    mount.Propagation `json:",omitempty"`
	Recursive      string            `json:",omitempty"`
}

// VolumeOptions represents options for a named Docker volume.
type VolumeOptions struct {
	// Driver specifies the volume driver and its options for volume creation.
	// TODO: It seems we don't really need Driver and Labels if we only support externally managed volumes.
	//  However we may need them in the future if we add support for isolated container-scoped volumes.
	Driver *mount.Driver `json:",omitempty"`
	// Labels are key-value metadata to apply to the volume if creating a new volume.
	Labels map[string]string `json:",omitempty"`
	// Name of the named Docker volume to use. If not specified, defaults to the VolumeSpec.Name.
	Name string `json:",omitempty"`
	// NoCopy prevents automatic copying of data from the container mount path to the volume.
	NoCopy bool `json:",omitempty"`
	// SubPath is the path within the volume to mount instead of its root.
	SubPath string `json:",omitempty"`
}

func (v *VolumeSpec) DockerVolumeName() string {
	if v.Type != VolumeTypeVolume {
		return ""
	}
	if v.VolumeOptions != nil && v.VolumeOptions.Name != "" {
		return v.VolumeOptions.Name
	}
	return v.Name
}

func (v *VolumeSpec) SetDefaults() VolumeSpec {
	spec := v.Clone()

	if spec.Type == VolumeTypeVolume {
		if spec.VolumeOptions == nil {
			spec.VolumeOptions = &VolumeOptions{}
		}
		if spec.VolumeOptions.Name == "" {
			spec.VolumeOptions.Name = spec.Name
		}
		if spec.VolumeOptions.Driver != nil && spec.VolumeOptions.Driver.Name == "" {
			spec.VolumeOptions.Driver.Name = VolumeDriverLocal
		}
	}
	// TODO: set explicit default values for Propagation and Recursive for bind mounts?

	return spec
}

func (v *VolumeSpec) Validate() error {
	if v.Name == "" {
		return fmt.Errorf("volume name must not be empty")
	}

	switch v.Type {
	case VolumeTypeBind:
		if v.BindOptions == nil {
			return fmt.Errorf("bind volume must have bind options")
		}
	case VolumeTypeVolume, VolumeTypeTmpfs:
	default:
		return fmt.Errorf("invalid volume type: '%s', must be one of '%s', '%s', '%s')",
			v.Type, VolumeTypeBind, VolumeTypeVolume, VolumeTypeTmpfs)
	}

	return nil
}

func (v *VolumeSpec) Equals(other VolumeSpec) bool {
	vol := v.SetDefaults()
	other = other.SetDefaults()

	return cmp.Equal(vol, other, cmpopts.EquateEmpty())
}

// MatchesDockerVolume checks if this VolumeSpec is compatible with the given named Docker volume.
// In other words, it checks if the spec could be used to mount the volume.
func (v *VolumeSpec) MatchesDockerVolume(vol volume.Volume) bool {
	if v.Type != VolumeTypeVolume {
		return false
	}
	spec := v.SetDefaults()

	if spec.DockerVolumeName() != vol.Name {
		return false
	}

	// The volume spec may not define the driver which means to use the default driver if creating a new volume
	// or accept any driver when mounting an existing volume. If the driver is specified in the spec, the spec's
	// driver and options must match the volume's driver and options.
	if spec.VolumeOptions.Driver != nil {
		volDriver := vol.Driver
		if volDriver == "" {
			volDriver = VolumeDriverLocal
		}

		if spec.VolumeOptions.Driver.Name != volDriver {
			return false
		}

		if !reflect.DeepEqual(spec.VolumeOptions.Driver.Options, vol.Options) {
			return false
		}
	}

	return true
}

func (v *VolumeSpec) Clone() VolumeSpec {
	spec := *v

	if v.BindOptions != nil {
		opts := *v.BindOptions
		spec.BindOptions = &opts
	}

	if v.VolumeOptions != nil {
		opts := *v.VolumeOptions
		if v.VolumeOptions.Driver != nil {
			driver := *v.VolumeOptions.Driver
			if driver.Options != nil {
				driver.Options = make(map[string]string, len(v.VolumeOptions.Driver.Options))
				for k, val := range v.VolumeOptions.Driver.Options {
					driver.Options[k] = val
				}
			}
			opts.Driver = &driver
		}

		if opts.Labels != nil {
			opts.Labels = make(map[string]string, len(v.VolumeOptions.Labels))
			for k, val := range v.VolumeOptions.Labels {
				opts.Labels[k] = val
			}
		}

		spec.VolumeOptions = &opts
	}

	if v.TmpfsOptions != nil {
		opts := *v.TmpfsOptions
		opts.Options = make([][]string, len(v.TmpfsOptions.Options))
		for i, opt := range v.TmpfsOptions.Options {
			opts.Options[i] = make([]string, len(opt))
			copy(opts.Options[i], opt)
		}
		spec.TmpfsOptions = &opts
	}

	return spec
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

func sortVolumeMounts(mounts []VolumeMount) {
	sort.Slice(mounts, func(i, j int) bool {
		if mounts[i].VolumeName != mounts[j].VolumeName {
			return mounts[i].VolumeName < mounts[j].VolumeName
		}
		if mounts[i].ContainerPath != mounts[j].ContainerPath {
			return mounts[i].ContainerPath < mounts[j].ContainerPath
		}
		if mounts[i].ReadOnly != mounts[j].ReadOnly {
			if !mounts[i].ReadOnly {
				return true
			}
		}
		return false
	})
}

// MachineVolume represents a volume on a specific machine.
type MachineVolume struct {
	// MachineID is the ID of the machine where the volume exists.
	MachineID string
	// MachineName is the name of the machine where the volume exists.
	MachineName string
	// Volume is the Docker volume model.
	Volume volume.Volume
}

// VolumeFilter defines criteria to filter volumes in ListVolumes.
type VolumeFilter struct {
	// Driver filters volumes by storage driver name.
	Driver string
	// Labels filters volumes by label key-value pairs. Volumes must match all labels.
	Labels map[string]string
	// Machines filters volumes to those on the specified machines (names or IDs).
	Machines []string
	// Names filters volumes by name. Volumes must match one of the names.
	Names []string
}

// MatchesFilter checks if a volume matches the specified filter criteria.
func (v *MachineVolume) MatchesFilter(filter *VolumeFilter) bool {
	if filter == nil {
		return true
	}
	// Filter by name.
	if len(filter.Names) > 0 && !slices.Contains(filter.Names, v.Volume.Name) {
		return false
	}
	// Filter by driver.
	if filter.Driver != "" && v.Volume.Driver != filter.Driver {
		return false
	}
	// Filter by labels.
	for key, value := range filter.Labels {
		labelValue, exists := v.Volume.Labels[key]
		if !exists || labelValue != value {
			return false
		}
	}
	// Filter by machines.
	if len(filter.Machines) > 0 {
		if !slices.ContainsFunc(filter.Machines, func(nameOrID string) bool {
			return v.MachineName == nameOrID || v.MachineID == nameOrID
		}) {
			return false
		}
	}

	return true
}

package deploy

import (
	"testing"

	"github.com/docker/docker/api/types/mount"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/stretchr/testify/assert"
)

func TestEvalContainerSpecChange_Volumes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		current  api.ServiceSpec
		new      api.ServiceSpec
		expected ContainerSpecStatus
	}{
		// TODO: should all volumes that are defined but not used (no corresponding mounts) be simply ignored?
		{
			name: "identical volumes",
			current: api.ServiceSpec{
				Volumes: []api.VolumeSpec{
					{
						Name: "data",
						Type: api.VolumeTypeVolume,
					},
				},
			},
			new: api.ServiceSpec{
				Volumes: []api.VolumeSpec{
					{
						Name: "data",
						Type: api.VolumeTypeVolume,
					},
				},
			},
			expected: ContainerUpToDate,
		},
		{
			name: "volumes in different order but identical",
			current: api.ServiceSpec{
				Volumes: []api.VolumeSpec{
					{
						Name: "data1",
						Type: api.VolumeTypeVolume,
					},
					{
						Name: "data2",
						Type: api.VolumeTypeVolume,
					},
				},
			},
			new: api.ServiceSpec{
				Volumes: []api.VolumeSpec{
					{
						Name: "data2",
						Type: api.VolumeTypeVolume,
					},
					{
						Name: "data1",
						Type: api.VolumeTypeVolume,
					},
				},
			},
			expected: ContainerUpToDate,
		},
		{
			name: "different number of volumes",
			current: api.ServiceSpec{
				Volumes: []api.VolumeSpec{
					{
						Name: "data",
						Type: api.VolumeTypeVolume,
					},
				},
			},
			new: api.ServiceSpec{
				Volumes: []api.VolumeSpec{
					{
						Name: "data",
						Type: api.VolumeTypeVolume,
					},
					{
						Name: "config",
						Type: api.VolumeTypeVolume,
					},
				},
			},
			expected: ContainerNeedsRecreate,
		},
		{
			name: "no volumes to one volume",
			current: api.ServiceSpec{
				Volumes: []api.VolumeSpec{},
			},
			new: api.ServiceSpec{
				Volumes: []api.VolumeSpec{
					{
						Name: "data",
						Type: api.VolumeTypeVolume,
					},
				},
			},
			expected: ContainerNeedsRecreate,
		},
		{
			name: "one volume to no volumes",
			current: api.ServiceSpec{
				Volumes: []api.VolumeSpec{
					{
						Name: "data",
						Type: api.VolumeTypeVolume,
					},
				},
			},
			new: api.ServiceSpec{
				Volumes: []api.VolumeSpec{},
			},
			expected: ContainerNeedsRecreate,
		},
		{
			name: "change volume type",
			current: api.ServiceSpec{
				Volumes: []api.VolumeSpec{
					{
						Name: "data",
						Type: api.VolumeTypeVolume,
					},
				},
			},
			new: api.ServiceSpec{
				Volumes: []api.VolumeSpec{
					{
						Name: "data",
						Type: api.VolumeTypeBind,
						BindOptions: &api.BindOptions{
							HostPath: "/host/path",
						},
					},
				},
			},
			expected: ContainerNeedsRecreate,
		},
		{
			name: "change volume options",
			current: api.ServiceSpec{
				Volumes: []api.VolumeSpec{
					{
						Name: "data",
						Type: api.VolumeTypeVolume,
						VolumeOptions: &api.VolumeOptions{
							NoCopy: false,
						},
					},
				},
			},
			new: api.ServiceSpec{
				Volumes: []api.VolumeSpec{
					{
						Name: "data",
						Type: api.VolumeTypeVolume,
						VolumeOptions: &api.VolumeOptions{
							NoCopy: true,
						},
					},
				},
			},
			expected: ContainerNeedsRecreate,
		},
		{
			name: "change volume options to defaults",
			current: api.ServiceSpec{
				Volumes: []api.VolumeSpec{
					{
						Name: "data",
						Type: api.VolumeTypeVolume,
					},
				},
			},
			new: api.ServiceSpec{
				Volumes: []api.VolumeSpec{
					{
						Name: "data",
						Type: api.VolumeTypeVolume,
						VolumeOptions: &api.VolumeOptions{
							Name: "data",
						},
					},
				},
			},
			expected: ContainerUpToDate,
		},
		{
			name: "change volume driver to local",
			current: api.ServiceSpec{
				Volumes: []api.VolumeSpec{
					{
						Name: "data",
						Type: api.VolumeTypeVolume,
					},
				},
			},
			new: api.ServiceSpec{
				Volumes: []api.VolumeSpec{
					{
						Name: "data",
						Type: api.VolumeTypeVolume,
						VolumeOptions: &api.VolumeOptions{
							Name: "data",
							Driver: &mount.Driver{
								Name: api.VolumeDriverLocal,
							},
						},
					},
				},
			},
			// TODO: this doesn't really require a recreate, only a spec update would be sufficient.
			expected: ContainerNeedsRecreate,
		},
		{
			name: "change volume driver to custom",
			current: api.ServiceSpec{
				Volumes: []api.VolumeSpec{
					{
						Name: "data",
						Type: api.VolumeTypeVolume,
					},
				},
			},
			new: api.ServiceSpec{
				Volumes: []api.VolumeSpec{
					{
						Name: "data",
						Type: api.VolumeTypeVolume,
						VolumeOptions: &api.VolumeOptions{
							Name: "data",
							Driver: &mount.Driver{
								Name: "custom",
							},
						},
					},
				},
			},
			// TODO: this doesn't really require a recreate, only a spec update would be sufficient.
			expected: ContainerNeedsRecreate,
		},
		{
			name: "change bind option CreateHostPath",
			current: api.ServiceSpec{
				Volumes: []api.VolumeSpec{
					{
						Name: "config",
						Type: api.VolumeTypeBind,
						BindOptions: &api.BindOptions{
							HostPath: "/host/path",
						},
					},
				},
			},
			new: api.ServiceSpec{
				Volumes: []api.VolumeSpec{
					{
						Name: "config",
						Type: api.VolumeTypeBind,
						BindOptions: &api.BindOptions{
							HostPath:       "/host/path",
							CreateHostPath: true,
						},
					},
				},
			},
			// TODO: this doesn't really require a recreate, only a spec update would be sufficient.
			expected: ContainerNeedsRecreate,
		},
		{
			name: "change bind option propagation",
			current: api.ServiceSpec{
				Volumes: []api.VolumeSpec{
					{
						Name: "config",
						Type: api.VolumeTypeBind,
						BindOptions: &api.BindOptions{
							HostPath: "/host/path",
						},
					},
				},
			},
			new: api.ServiceSpec{
				Volumes: []api.VolumeSpec{
					{
						Name: "config",
						Type: api.VolumeTypeBind,
						BindOptions: &api.BindOptions{
							HostPath:    "/host/path",
							Propagation: mount.PropagationRPrivate,
						},
					},
				},
			},
			// TODO: should we handle rprivate the same as empty propagation, hence up-to-date?
			expected: ContainerNeedsRecreate,
		},
		{
			name: "change tmpfs options",
			current: api.ServiceSpec{
				Volumes: []api.VolumeSpec{
					{
						Name: "temp",
						Type: api.VolumeTypeTmpfs,
						TmpfsOptions: &mount.TmpfsOptions{
							SizeBytes: 1024 * 1024,
						},
					},
				},
			},
			new: api.ServiceSpec{
				Volumes: []api.VolumeSpec{
					{
						Name: "temp",
						Type: api.VolumeTypeTmpfs,
						TmpfsOptions: &mount.TmpfsOptions{
							SizeBytes: 2 * 1024 * 1024,
						},
					},
				},
			},
			expected: ContainerNeedsRecreate,
		},
		{
			name: "changing volume name",
			current: api.ServiceSpec{
				Volumes: []api.VolumeSpec{
					{
						Name: "data",
						Type: api.VolumeTypeVolume,
					},
				},
			},
			new: api.ServiceSpec{
				Volumes: []api.VolumeSpec{
					{
						Name: "data-new",
						Type: api.VolumeTypeVolume,
					},
				},
			},
			expected: ContainerNeedsRecreate,
		},
		// Tests for volume mounts
		{
			name: "identical volume mounts",
			current: api.ServiceSpec{
				Volumes: []api.VolumeSpec{
					{
						Name: "data",
						Type: api.VolumeTypeVolume,
					},
				},
				Container: api.ContainerSpec{
					VolumeMounts: []api.VolumeMount{
						{
							VolumeName:    "data",
							ContainerPath: "/data",
						},
					},
				},
			},
			new: api.ServiceSpec{
				Volumes: []api.VolumeSpec{
					{
						Name: "data",
						Type: api.VolumeTypeVolume,
					},
				},
				Container: api.ContainerSpec{
					VolumeMounts: []api.VolumeMount{
						{
							VolumeName:    "data",
							ContainerPath: "/data",
						},
					},
				},
			},
			expected: ContainerUpToDate,
		},
		{
			name: "volume mounts in different order but identical",
			current: api.ServiceSpec{
				Volumes: []api.VolumeSpec{
					{
						Name: "data1",
						Type: api.VolumeTypeVolume,
					},
					{
						Name: "data2",
						Type: api.VolumeTypeVolume,
					},
				},
				Container: api.ContainerSpec{
					VolumeMounts: []api.VolumeMount{
						{
							VolumeName:    "data1",
							ContainerPath: "/data1",
						},
						{
							VolumeName:    "data2",
							ContainerPath: "/data2",
						},
					},
				},
			},
			new: api.ServiceSpec{
				Volumes: []api.VolumeSpec{
					{
						Name: "data1",
						Type: api.VolumeTypeVolume,
					},
					{
						Name: "data2",
						Type: api.VolumeTypeVolume,
					},
				},
				Container: api.ContainerSpec{
					VolumeMounts: []api.VolumeMount{
						{
							VolumeName:    "data2",
							ContainerPath: "/data2",
						},
						{
							VolumeName:    "data1",
							ContainerPath: "/data1",
						},
					},
				},
			},
			expected: ContainerUpToDate,
		},
		{
			name: "volumes and volume mounts in different order but identical",
			current: api.ServiceSpec{
				Volumes: []api.VolumeSpec{
					{
						Name: "data1",
						Type: api.VolumeTypeVolume,
					},
					{
						Name: "data2",
						Type: api.VolumeTypeVolume,
					},
				},
				Container: api.ContainerSpec{
					VolumeMounts: []api.VolumeMount{
						{
							VolumeName:    "data1",
							ContainerPath: "/data1",
						},
						{
							VolumeName:    "data2",
							ContainerPath: "/data2",
						},
					},
				},
			},
			new: api.ServiceSpec{
				Volumes: []api.VolumeSpec{
					{
						Name: "data2",
						Type: api.VolumeTypeVolume,
					},
					{
						Name: "data1",
						Type: api.VolumeTypeVolume,
					},
				},
				Container: api.ContainerSpec{
					VolumeMounts: []api.VolumeMount{
						{
							VolumeName:    "data2",
							ContainerPath: "/data2",
						},
						{
							VolumeName:    "data1",
							ContainerPath: "/data1",
						},
					},
				},
			},
			expected: ContainerUpToDate,
		},
		{
			name: "added volume mount",
			current: api.ServiceSpec{
				Volumes: []api.VolumeSpec{
					{
						Name: "data",
						Type: api.VolumeTypeVolume,
					},
					{
						Name: "config",
						Type: api.VolumeTypeVolume,
					},
				},
				Container: api.ContainerSpec{
					VolumeMounts: []api.VolumeMount{
						{
							VolumeName:    "data",
							ContainerPath: "/data",
						},
					},
				},
			},
			new: api.ServiceSpec{
				Volumes: []api.VolumeSpec{
					{
						Name: "data",
						Type: api.VolumeTypeVolume,
					},
					{
						Name: "config",
						Type: api.VolumeTypeVolume,
					},
				},
				Container: api.ContainerSpec{
					VolumeMounts: []api.VolumeMount{
						{
							VolumeName:    "data",
							ContainerPath: "/data",
						},
						{
							VolumeName:    "config",
							ContainerPath: "/config",
						},
					},
				},
			},
			expected: ContainerNeedsRecreate,
		},
		{
			name: "removed volume mount",
			current: api.ServiceSpec{
				Volumes: []api.VolumeSpec{
					{
						Name: "data",
						Type: api.VolumeTypeVolume,
					},
					{
						Name: "config",
						Type: api.VolumeTypeVolume,
					},
				},
				Container: api.ContainerSpec{
					VolumeMounts: []api.VolumeMount{
						{
							VolumeName:    "data",
							ContainerPath: "/data",
						},
						{
							VolumeName:    "config",
							ContainerPath: "/config",
						},
					},
				},
			},
			new: api.ServiceSpec{
				Volumes: []api.VolumeSpec{
					{
						Name: "data",
						Type: api.VolumeTypeVolume,
					},
					{
						Name: "config",
						Type: api.VolumeTypeVolume,
					},
				},
				Container: api.ContainerSpec{
					VolumeMounts: []api.VolumeMount{
						{
							VolumeName:    "data",
							ContainerPath: "/data",
						},
					},
				},
			},
			expected: ContainerNeedsRecreate,
		},
		{
			name: "changed volume mount container path",
			current: api.ServiceSpec{
				Volumes: []api.VolumeSpec{
					{
						Name: "data",
						Type: api.VolumeTypeVolume,
					},
				},
				Container: api.ContainerSpec{
					VolumeMounts: []api.VolumeMount{
						{
							VolumeName:    "data",
							ContainerPath: "/data",
						},
					},
				},
			},
			new: api.ServiceSpec{
				Volumes: []api.VolumeSpec{
					{
						Name: "data",
						Type: api.VolumeTypeVolume,
					},
				},
				Container: api.ContainerSpec{
					VolumeMounts: []api.VolumeMount{
						{
							VolumeName:    "data",
							ContainerPath: "/new/data/path",
						},
					},
				},
			},
			expected: ContainerNeedsRecreate,
		},
		{
			name: "changed volume mount read-only flag",
			current: api.ServiceSpec{
				Volumes: []api.VolumeSpec{
					{
						Name: "data",
						Type: api.VolumeTypeVolume,
					},
				},
				Container: api.ContainerSpec{
					VolumeMounts: []api.VolumeMount{
						{
							VolumeName:    "data",
							ContainerPath: "/data",
							ReadOnly:      false,
						},
					},
				},
			},
			new: api.ServiceSpec{
				Volumes: []api.VolumeSpec{
					{
						Name: "data",
						Type: api.VolumeTypeVolume,
					},
				},
				Container: api.ContainerSpec{
					VolumeMounts: []api.VolumeMount{
						{
							VolumeName:    "data",
							ContainerPath: "/data",
							ReadOnly:      true,
						},
					},
				},
			},
			expected: ContainerNeedsRecreate,
		},
		{
			name: "changed mounted volume type",
			current: api.ServiceSpec{
				Volumes: []api.VolumeSpec{
					{
						Name: "data",
						Type: api.VolumeTypeVolume,
					},
				},
				Container: api.ContainerSpec{
					VolumeMounts: []api.VolumeMount{
						{
							VolumeName:    "data",
							ContainerPath: "/data",
						},
					},
				},
			},
			new: api.ServiceSpec{
				Volumes: []api.VolumeSpec{
					{
						Name: "data",
						Type: api.VolumeTypeBind,
						BindOptions: &api.BindOptions{
							HostPath: "/host/path",
						},
					},
				},
				Container: api.ContainerSpec{
					VolumeMounts: []api.VolumeMount{
						{
							VolumeName:    "data",
							ContainerPath: "/data",
						},
					},
				},
			},
			expected: ContainerNeedsRecreate,
		},
		{
			name: "changed reference name in spec but preserved original volume name",
			current: api.ServiceSpec{
				Volumes: []api.VolumeSpec{
					{
						Name: "data",
						Type: api.VolumeTypeVolume,
					},
				},
				Container: api.ContainerSpec{
					VolumeMounts: []api.VolumeMount{
						{
							VolumeName:    "data",
							ContainerPath: "/data",
						},
					},
				},
			},
			new: api.ServiceSpec{
				Volumes: []api.VolumeSpec{
					{
						Name: "new_name",
						Type: api.VolumeTypeVolume,
						VolumeOptions: &api.VolumeOptions{
							Name: "data",
						},
					},
				},
				Container: api.ContainerSpec{
					VolumeMounts: []api.VolumeMount{
						{
							VolumeName:    "new_name",
							ContainerPath: "/data",
						},
					},
				},
			},
			// TODO: this doesn't really require a recreate, only a spec update would be sufficient.
			expected: ContainerNeedsRecreate,
		},
		{
			name: "changed volumes, mounts and paths",
			current: api.ServiceSpec{
				Volumes: []api.VolumeSpec{
					{
						Name: "data",
						Type: api.VolumeTypeVolume,
					},
					{
						Name: "config",
						Type: api.VolumeTypeBind,
						BindOptions: &api.BindOptions{
							HostPath: "/etc/app/config",
						},
					},
				},
				Container: api.ContainerSpec{
					VolumeMounts: []api.VolumeMount{
						{
							VolumeName:    "data",
							ContainerPath: "/data",
						},
						{
							VolumeName:    "config",
							ContainerPath: "/app/config",
							ReadOnly:      true,
						},
					},
				},
			},
			new: api.ServiceSpec{
				Volumes: []api.VolumeSpec{
					{
						Name: "data",
						Type: api.VolumeTypeVolume,
						VolumeOptions: &api.VolumeOptions{
							NoCopy: true,
						},
					},
					{
						Name: "config",
						Type: api.VolumeTypeBind,
						BindOptions: &api.BindOptions{
							HostPath:       "/etc/app/new-config",
							CreateHostPath: true,
						},
					},
					{
						Name: "logs",
						Type: api.VolumeTypeVolume,
					},
				},
				Container: api.ContainerSpec{
					VolumeMounts: []api.VolumeMount{
						{
							VolumeName:    "data",
							ContainerPath: "/var/lib/app/data",
						},
						{
							VolumeName:    "config",
							ContainerPath: "/app/config",
							ReadOnly:      true,
						},
						{
							VolumeName:    "logs",
							ContainerPath: "/var/log/app",
						},
					},
				},
			},
			expected: ContainerNeedsRecreate,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EvalContainerSpecChange(tt.current, tt.new)
			assert.Equal(t, tt.expected, result)
		})
	}
}

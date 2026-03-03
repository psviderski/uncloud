package deploy

import (
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/stretchr/testify/assert"
)

func TestEvalContainerSpecChange_ContainerCapAdd(t *testing.T) {
	t.Parallel()

	currentSpec := api.ServiceSpec{
		Container: api.ContainerSpec{
			Image: "nginx:latest",
		},
	}
	newSpec := api.ServiceSpec{
		Container: api.ContainerSpec{
			Image:  "nginx:latest",
			CapAdd: []string{"NET_ADMIN"},
		},
	}

	assert.Equal(t, ContainerNeedsRecreate, EvalContainerSpecChange(currentSpec, newSpec))
	assert.Equal(t, ContainerNeedsRecreate, EvalContainerSpecChange(newSpec, currentSpec))
}

func TestEvalContainerSpecChange_ContainerCapDrop(t *testing.T) {
	t.Parallel()

	currentSpec := api.ServiceSpec{
		Container: api.ContainerSpec{
			Image: "nginx:latest",
		},
	}
	newSpec := api.ServiceSpec{
		Container: api.ContainerSpec{
			Image:   "nginx:latest",
			CapDrop: []string{"ALL"},
		},
	}

	assert.Equal(t, ContainerNeedsRecreate, EvalContainerSpecChange(currentSpec, newSpec))
	assert.Equal(t, ContainerNeedsRecreate, EvalContainerSpecChange(newSpec, currentSpec))
}

func TestEvalContainerSpecChange_ContainerResources(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		current api.ContainerResources
		new     api.ContainerResources
		want    ContainerSpecStatus
	}{
		{
			name:    "empty",
			current: api.ContainerResources{},
			new:     api.ContainerResources{},
			want:    ContainerUpToDate,
		},

		// CPU
		{
			name:    "set CPU",
			current: api.ContainerResources{},
			new: api.ContainerResources{
				CPU: 1000000000,
			},
			want: ContainerNeedsUpdate,
		},
		{
			name: "change CPU",
			current: api.ContainerResources{
				CPU: 1000000000,
			},
			new: api.ContainerResources{
				CPU: 2000000000,
			},
			want: ContainerNeedsUpdate,
		},
		{
			name: "unset CPU",
			current: api.ContainerResources{
				CPU: 1000000000,
			},
			new:  api.ContainerResources{},
			want: ContainerNeedsUpdate,
		},

		// Memory
		{
			name:    "equal memory",
			current: api.ContainerResources{Memory: 100 * 1024 * 1024},
			new:     api.ContainerResources{Memory: 100 * 1024 * 1024},
			want:    ContainerUpToDate,
		},
		{
			name:    "equal reservation",
			current: api.ContainerResources{MemoryReservation: 50 * 1024 * 1024},
			new:     api.ContainerResources{MemoryReservation: 50 * 1024 * 1024},
			want:    ContainerUpToDate,
		},
		{
			name:    "equal memory and reservation",
			current: api.ContainerResources{Memory: 100 * 1024 * 1024, MemoryReservation: 50 * 1024 * 1024},
			new:     api.ContainerResources{Memory: 100 * 1024 * 1024, MemoryReservation: 50 * 1024 * 1024},
			want:    ContainerUpToDate,
		},
		{
			name:    "set memory",
			current: api.ContainerResources{},
			new:     api.ContainerResources{Memory: 100 * 1024 * 1024},
			want:    ContainerNeedsUpdate,
		},
		{
			name:    "set reservation",
			current: api.ContainerResources{},
			new:     api.ContainerResources{MemoryReservation: 50 * 1024 * 1024},
			want:    ContainerNeedsUpdate,
		},
		{
			name:    "set memory and reservation",
			current: api.ContainerResources{},
			new:     api.ContainerResources{Memory: 100 * 1024 * 1024, MemoryReservation: 50 * 1024 * 1024},
			want:    ContainerNeedsUpdate,
		},
		{
			name:    "change memory",
			current: api.ContainerResources{Memory: 100 * 1024 * 1024},
			new:     api.ContainerResources{Memory: 200 * 1024 * 1024},
			want:    ContainerNeedsUpdate,
		},
		{
			name:    "change reservation",
			current: api.ContainerResources{MemoryReservation: 50 * 1024 * 1024},
			new:     api.ContainerResources{MemoryReservation: 100 * 1024 * 1024},
			want:    ContainerNeedsUpdate,
		},
		{
			name:    "change memory and reservation",
			current: api.ContainerResources{Memory: 100 * 1024 * 1024, MemoryReservation: 50 * 1024 * 1024},
			new:     api.ContainerResources{Memory: 200 * 1024 * 1024, MemoryReservation: 100 * 1024 * 1024},
			want:    ContainerNeedsUpdate,
		},
		{
			name:    "unset memory",
			current: api.ContainerResources{Memory: 100 * 1024 * 1024, MemoryReservation: 50 * 1024 * 1024},
			new:     api.ContainerResources{MemoryReservation: 50 * 1024 * 1024},
			want:    ContainerNeedsUpdate,
		},
		{
			name:    "unset reservation",
			current: api.ContainerResources{Memory: 100 * 1024 * 1024, MemoryReservation: 50 * 1024 * 1024},
			new:     api.ContainerResources{Memory: 100 * 1024 * 1024},
			want:    ContainerNeedsUpdate,
		},
		{
			name:    "unset memory and reservation",
			current: api.ContainerResources{Memory: 100 * 1024 * 1024, MemoryReservation: 50 * 1024 * 1024},
			new:     api.ContainerResources{},
			want:    ContainerNeedsUpdate,
		},

		// Memory and CPU
		{
			name:    "set CPU and memory",
			current: api.ContainerResources{},
			new: api.ContainerResources{
				CPU:    1000000000,
				Memory: 100 * 1024 * 1024,
			},
			want: ContainerNeedsUpdate,
		},
		{
			name: "update CPU and memory",
			current: api.ContainerResources{
				CPU:    1000000000,
				Memory: 100 * 1024 * 1024,
			},
			new: api.ContainerResources{
				CPU:               2000000000,
				Memory:            200 * 1024 * 1024,
				MemoryReservation: 100 * 1024 * 1024,
			},
			want: ContainerNeedsUpdate,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			currentSpec := api.ServiceSpec{
				Container: api.ContainerSpec{
					Image:     "nginx:latest",
					Resources: tt.current,
				},
			}
			newSpec := api.ServiceSpec{
				Container: api.ContainerSpec{
					Image:     "nginx:latest",
					Resources: tt.new,
				},
			}

			result := EvalContainerSpecChange(currentSpec, newSpec)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestEvalContainerSpecChange_ContainerImage(t *testing.T) {
	t.Parallel()

	currentSpec := api.ServiceSpec{
		Container: api.ContainerSpec{
			Image: "nginx:latest",
		},
	}
	newSpec := api.ServiceSpec{
		Container: api.ContainerSpec{
			Image: "nginx:latest",
		},
	}
	assert.Equal(t, ContainerUpToDate, EvalContainerSpecChange(currentSpec, newSpec))

	newSpec.Container.Image = "nginx:1.19"
	assert.Equal(t, ContainerNeedsRecreate, EvalContainerSpecChange(currentSpec, newSpec))
}

func TestEvalContainerSpecChange_ContainerLogDriver(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		current *api.LogDriver
		new     *api.LogDriver
		want    ContainerSpecStatus
	}{
		{
			name:    "set driver",
			current: nil,
			new: &api.LogDriver{
				Name: "json-file",
			},
			want: ContainerNeedsRecreate,
		},
		{
			name: "change driver",
			current: &api.LogDriver{
				Name: "json-file",
			},
			new: &api.LogDriver{
				Name: "syslog",
			},
			want: ContainerNeedsRecreate,
		},
		{
			name: "set empty options",
			current: &api.LogDriver{
				Name: "json-file",
			},
			new: &api.LogDriver{
				Name:    "json-file",
				Options: map[string]string{},
			},
			want: ContainerUpToDate,
		},
		{
			name: "set options",
			current: &api.LogDriver{
				Name: "json-file",
			},
			new: &api.LogDriver{
				Name: "json-file",
				Options: map[string]string{
					"max-size": "10m",
				},
			},
			want: ContainerNeedsRecreate,
		},
		{
			name: "change options",
			current: &api.LogDriver{
				Name: "json-file",
				Options: map[string]string{
					"max-size": "10m",
				},
			},
			new: &api.LogDriver{
				Name: "json-file",
				Options: map[string]string{
					"max-size": "20m",
				},
			},
			want: ContainerNeedsRecreate,
		},
		{
			name: "unset options",
			current: &api.LogDriver{
				Name: "json-file",
				Options: map[string]string{
					"max-size": "10m",
				},
			},
			new: &api.LogDriver{
				Name: "json-file",
			},
			want: ContainerNeedsRecreate,
		},
		{
			name: "unset driver",
			current: &api.LogDriver{
				Name: "json-file",
				Options: map[string]string{
					"max-size": "10m",
				},
			},
			new:  nil,
			want: ContainerNeedsRecreate,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			currentSpec := api.ServiceSpec{
				Container: api.ContainerSpec{
					Image:     "nginx:latest",
					LogDriver: tt.current,
				},
			}
			newSpec := api.ServiceSpec{
				Container: api.ContainerSpec{
					Image:     "nginx:latest",
					LogDriver: tt.new,
				},
			}

			result := EvalContainerSpecChange(currentSpec, newSpec)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestEvalContainerSpecChange_ContainerPrivileged(t *testing.T) {
	t.Parallel()

	currentSpec := api.ServiceSpec{
		Container: api.ContainerSpec{
			Image: "nginx:latest",
		},
	}
	newSpec := api.ServiceSpec{
		Container: api.ContainerSpec{
			Image:      "nginx:latest",
			Privileged: true,
		},
	}

	assert.Equal(t, ContainerNeedsRecreate, EvalContainerSpecChange(currentSpec, newSpec))
	assert.Equal(t, ContainerNeedsRecreate, EvalContainerSpecChange(newSpec, currentSpec))
}

func TestEvalContainerSpecChange_ContainerSysctls(t *testing.T) {
	t.Parallel()

	currentSpec := api.ServiceSpec{
		Container: api.ContainerSpec{
			Image: "nginx:latest",
		},
	}
	newSpec := api.ServiceSpec{
		Container: api.ContainerSpec{
			Image: "nginx:latest",
			Sysctls: map[string]string{
				"net.ipv4.ip_forward": "1",
			},
		},
	}

	assert.Equal(t, ContainerNeedsRecreate, EvalContainerSpecChange(currentSpec, newSpec))
	assert.Equal(t, ContainerNeedsRecreate, EvalContainerSpecChange(newSpec, currentSpec))
}

func TestEvalContainerSpecChange_PullPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		current string
		new     string
		want    ContainerSpecStatus
	}{
		{
			name:    "non-always to always",
			current: api.PullPolicyMissing,
			new:     api.PullPolicyAlways,
			want:    ContainerNeedsRecreate,
		},
		{
			name:    "always to always",
			current: api.PullPolicyAlways,
			new:     api.PullPolicyAlways,
			want:    ContainerNeedsRecreate,
		},
		{
			name:    "always to non-always",
			current: api.PullPolicyAlways,
			new:     api.PullPolicyMissing,
			want:    ContainerUpToDate,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			currentSpec := api.ServiceSpec{
				Container: api.ContainerSpec{
					PullPolicy: tt.current,
				},
			}
			newSpec := api.ServiceSpec{
				Container: api.ContainerSpec{
					PullPolicy: tt.new,
				},
			}

			result := EvalContainerSpecChange(currentSpec, newSpec)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestEvalContainerSpecChange_ContainerUser(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		current string
		new     string
		want    ContainerSpecStatus
	}{
		{
			name:    "empty",
			current: "",
			new:     "",
			want:    ContainerUpToDate,
		},
		{
			name:    "equal user",
			current: "user",
			new:     "user",
			want:    ContainerUpToDate,
		},
		{
			name:    "equal UID",
			current: "1000",
			new:     "1000",
			want:    ContainerUpToDate,
		},
		{
			name:    "equal user group",
			current: "user:group",
			new:     "user:group",
			want:    ContainerUpToDate,
		},
		{
			name:    "equal UID GID",
			current: "1000:1000",
			new:     "1000:1000",
			want:    ContainerUpToDate,
		},
		{
			name:    "set user",
			current: "",
			new:     "user",
			want:    ContainerNeedsRecreate,
		},
		{
			name:    "set UID",
			current: "",
			new:     "1000",
			want:    ContainerNeedsRecreate,
		},
		{
			name:    "set user group",
			current: "",
			new:     "user:group",
			want:    ContainerNeedsRecreate,
		},
		{
			name:    "set UID GID",
			current: "",
			new:     "1000:1000",
			want:    ContainerNeedsRecreate,
		},
		{
			name:    "change user",
			current: "user",
			new:     "another_user",
			want:    ContainerNeedsRecreate,
		},
		{
			name:    "change group",
			current: "user:group",
			new:     "user:another_group",
			want:    ContainerNeedsRecreate,
		},
		{
			name:    "change user group",
			current: "user:group",
			new:     "another_user:another_group",
			want:    ContainerNeedsRecreate,
		},
		{
			name:    "unser user",
			current: "user",
			new:     "",
			want:    ContainerNeedsRecreate,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			currentSpec := api.ServiceSpec{
				Container: api.ContainerSpec{
					Image: "nginx:latest",
					User:  tt.current,
				},
			}
			newSpec := api.ServiceSpec{
				Container: api.ContainerSpec{
					Image: "nginx:latest",
					User:  tt.new,
				},
			}

			result := EvalContainerSpecChange(currentSpec, newSpec)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestEvalContainerSpecChange_Placement(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		current api.Placement
		new     api.Placement
		want    ContainerSpecStatus
	}{
		{
			name:    "empty",
			current: api.Placement{},
			new:     api.Placement{},
			want:    ContainerUpToDate,
		},
		{
			name:    "empty machines",
			current: api.Placement{Machines: nil},
			new:     api.Placement{Machines: []string{}},
			want:    ContainerUpToDate,
		},
		{
			name:    "set machines",
			current: api.Placement{},
			new:     api.Placement{Machines: []string{"machine1"}},
			want:    ContainerNeedsRecreate,
		},
		{
			name:    "unset",
			current: api.Placement{Machines: []string{"machine1"}},
			new:     api.Placement{},
			want:    ContainerNeedsRecreate,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			currentSpec := api.ServiceSpec{
				Container: api.ContainerSpec{
					Image: "nginx:latest",
				},
				Placement: tt.current,
			}
			newSpec := api.ServiceSpec{
				Container: api.ContainerSpec{
					Image: "nginx:latest",
				},
				Placement: tt.new,
			}

			result := EvalContainerSpecChange(currentSpec, newSpec)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestEvalContainerSpecChange_Volumes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		current api.ServiceSpec
		new     api.ServiceSpec
		want    ContainerSpecStatus
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
			want: ContainerUpToDate,
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
			want: ContainerUpToDate,
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
			want: ContainerNeedsRecreate,
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
			want: ContainerNeedsRecreate,
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
			want: ContainerNeedsRecreate,
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
			want: ContainerNeedsRecreate,
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
			want: ContainerNeedsRecreate,
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
			want: ContainerUpToDate,
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
			want: ContainerNeedsRecreate,
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
			want: ContainerNeedsRecreate,
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
			want: ContainerNeedsRecreate,
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
			want: ContainerNeedsRecreate,
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
			want: ContainerNeedsRecreate,
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
			want: ContainerNeedsRecreate,
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
			want: ContainerUpToDate,
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
			want: ContainerUpToDate,
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
			want: ContainerUpToDate,
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
			want: ContainerNeedsRecreate,
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
			want: ContainerNeedsRecreate,
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
			want: ContainerNeedsRecreate,
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
			want: ContainerNeedsRecreate,
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
			want: ContainerNeedsRecreate,
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
			want: ContainerNeedsRecreate,
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
			want: ContainerNeedsRecreate,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EvalContainerSpecChange(tt.current, tt.new)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestEvalContainerSpecChange_Devices(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		current api.ContainerResources
		new     api.ContainerResources
		want    ContainerSpecStatus
	}{
		{
			name:    "empty",
			current: api.ContainerResources{},
			new:     api.ContainerResources{},
			want:    ContainerUpToDate,
		},
		{
			name:    "identical mapping",
			current: api.ContainerResources{Devices: []api.DeviceMapping{{HostPath: "/dev/foo", ContainerPath: "/dev/foo", CgroupPermissions: "rwm"}}},
			new:     api.ContainerResources{Devices: []api.DeviceMapping{{HostPath: "/dev/foo", ContainerPath: "/dev/foo", CgroupPermissions: "rwm"}}},
			want:    ContainerUpToDate,
		},
		{
			name:    "add mapping",
			current: api.ContainerResources{},
			new:     api.ContainerResources{Devices: []api.DeviceMapping{{HostPath: "/dev/foo", ContainerPath: "/dev/foo", CgroupPermissions: "rwm"}}},
			want:    ContainerNeedsRecreate,
		},
		{
			name:    "remove mapping",
			current: api.ContainerResources{Devices: []api.DeviceMapping{{HostPath: "/dev/foo", ContainerPath: "/dev/foo", CgroupPermissions: "rwm"}}},
			new:     api.ContainerResources{},
			want:    ContainerNeedsRecreate,
		},
		{
			name:    "change mapping path",
			current: api.ContainerResources{Devices: []api.DeviceMapping{{HostPath: "/dev/foo", ContainerPath: "/dev/foo", CgroupPermissions: "rwm"}}},
			new:     api.ContainerResources{Devices: []api.DeviceMapping{{HostPath: "/dev/foo", ContainerPath: "/dev/bar", CgroupPermissions: "rwm"}}},
			want:    ContainerNeedsRecreate,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			currentSpec := api.ServiceSpec{
				Container: api.ContainerSpec{
					Image:     "nginx:latest",
					Resources: tt.current,
				},
			}
			newSpec := api.ServiceSpec{
				Container: api.ContainerSpec{
					Image:     "nginx:latest",
					Resources: tt.new,
				},
			}

			result := EvalContainerSpecChange(currentSpec, newSpec)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestEvalContainerSpecChange_DeviceReservations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		current api.ContainerResources
		new     api.ContainerResources
		want    ContainerSpecStatus
	}{
		{
			name:    "empty",
			current: api.ContainerResources{},
			new:     api.ContainerResources{},
			want:    ContainerUpToDate,
		},
		{
			name:    "both nil",
			current: api.ContainerResources{DeviceReservations: nil},
			new:     api.ContainerResources{DeviceReservations: nil},
			want:    ContainerUpToDate,
		},
		{
			name:    "both empty",
			current: api.ContainerResources{DeviceReservations: []container.DeviceRequest{}},
			new:     api.ContainerResources{DeviceReservations: []container.DeviceRequest{}},
			want:    ContainerUpToDate,
		},
		{
			name:    "set GPU request",
			current: api.ContainerResources{},
			new: api.ContainerResources{
				DeviceReservations: []container.DeviceRequest{
					{
						Count:        -1, // all
						Capabilities: [][]string{{"gpu"}},
					},
				},
			},
			want: ContainerNeedsRecreate,
		},
		{
			name: "unset GPU request",
			current: api.ContainerResources{
				DeviceReservations: []container.DeviceRequest{
					{
						Count:        -1,
						Capabilities: [][]string{{"gpu"}},
					},
				},
			},
			new:  api.ContainerResources{},
			want: ContainerNeedsRecreate,
		},
		{
			name: "identical GPU request",
			current: api.ContainerResources{
				DeviceReservations: []container.DeviceRequest{
					{
						Driver:       "nvidia",
						Count:        2,
						Capabilities: [][]string{{"gpu", "compute"}},
					},
				},
			},
			new: api.ContainerResources{
				DeviceReservations: []container.DeviceRequest{
					{
						Driver:       "nvidia",
						Count:        2,
						Capabilities: [][]string{{"gpu", "compute"}},
					},
				},
			},
			want: ContainerUpToDate,
		},
		{
			name: "change GPU count",
			current: api.ContainerResources{
				DeviceReservations: []container.DeviceRequest{
					{
						Count:        1,
						Capabilities: [][]string{{"gpu"}},
					},
				},
			},
			new: api.ContainerResources{
				DeviceReservations: []container.DeviceRequest{
					{
						Count:        2,
						Capabilities: [][]string{{"gpu"}},
					},
				},
			},
			want: ContainerNeedsRecreate,
		},
		{
			name: "change GPU driver",
			current: api.ContainerResources{
				DeviceReservations: []container.DeviceRequest{
					{
						Driver:       "nvidia",
						Count:        1,
						Capabilities: [][]string{{"gpu"}},
					},
				},
			},
			new: api.ContainerResources{
				DeviceReservations: []container.DeviceRequest{
					{
						Driver:       "amd",
						Count:        1,
						Capabilities: [][]string{{"gpu"}},
					},
				},
			},
			want: ContainerNeedsRecreate,
		},
		{
			name: "change device IDs",
			current: api.ContainerResources{
				DeviceReservations: []container.DeviceRequest{
					{
						DeviceIDs:    []string{"0"},
						Capabilities: [][]string{{"gpu"}},
					},
				},
			},
			new: api.ContainerResources{
				DeviceReservations: []container.DeviceRequest{
					{
						DeviceIDs:    []string{"0", "1"},
						Capabilities: [][]string{{"gpu"}},
					},
				},
			},
			want: ContainerNeedsRecreate,
		},
		{
			name: "change capabilities",
			current: api.ContainerResources{
				DeviceReservations: []container.DeviceRequest{
					{
						Count:        1,
						Capabilities: [][]string{{"gpu"}},
					},
				},
			},
			new: api.ContainerResources{
				DeviceReservations: []container.DeviceRequest{
					{
						Count:        1,
						Capabilities: [][]string{{"gpu", "compute"}},
					},
				},
			},
			want: ContainerNeedsRecreate,
		},
		{
			name: "set options",
			current: api.ContainerResources{
				DeviceReservations: []container.DeviceRequest{
					{
						Count:        1,
						Capabilities: [][]string{{"gpu"}},
					},
				},
			},
			new: api.ContainerResources{
				DeviceReservations: []container.DeviceRequest{
					{
						Count:        1,
						Capabilities: [][]string{{"gpu"}},
						Options: map[string]string{
							"key": "value",
						},
					},
				},
			},
			want: ContainerNeedsRecreate,
		},
		{
			name: "change options",
			current: api.ContainerResources{
				DeviceReservations: []container.DeviceRequest{
					{
						Count:        1,
						Capabilities: [][]string{{"gpu"}},
						Options: map[string]string{
							"key": "value1",
						},
					},
				},
			},
			new: api.ContainerResources{
				DeviceReservations: []container.DeviceRequest{
					{
						Count:        1,
						Capabilities: [][]string{{"gpu"}},
						Options: map[string]string{
							"key": "value2",
						},
					},
				},
			},
			want: ContainerNeedsRecreate,
		},
		{
			name: "unset options",
			current: api.ContainerResources{
				DeviceReservations: []container.DeviceRequest{
					{
						Count:        1,
						Capabilities: [][]string{{"gpu"}},
						Options: map[string]string{
							"key": "value",
						},
					},
				},
			},
			new: api.ContainerResources{
				DeviceReservations: []container.DeviceRequest{
					{
						Count:        1,
						Capabilities: [][]string{{"gpu"}},
					},
				},
			},
			want: ContainerNeedsRecreate,
		},
		{
			name: "add device request",
			current: api.ContainerResources{
				DeviceReservations: []container.DeviceRequest{
					{
						Count:        1,
						Capabilities: [][]string{{"gpu"}},
					},
				},
			},
			new: api.ContainerResources{
				DeviceReservations: []container.DeviceRequest{
					{
						Count:        1,
						Capabilities: [][]string{{"gpu"}},
					},
					{
						Count:        1,
						Capabilities: [][]string{{"tpu"}},
					},
				},
			},
			want: ContainerNeedsRecreate,
		},
		{
			name: "remove device request",
			current: api.ContainerResources{
				DeviceReservations: []container.DeviceRequest{
					{
						Count:        1,
						Capabilities: [][]string{{"gpu"}},
					},
					{
						Count:        1,
						Capabilities: [][]string{{"tpu"}},
					},
				},
			},
			new: api.ContainerResources{
				DeviceReservations: []container.DeviceRequest{
					{
						Count:        1,
						Capabilities: [][]string{{"gpu"}},
					},
				},
			},
			want: ContainerNeedsRecreate,
		},
		{
			name: "multiple identical device requests",
			current: api.ContainerResources{
				DeviceReservations: []container.DeviceRequest{
					{
						Driver:       "nvidia",
						Count:        2,
						Capabilities: [][]string{{"gpu", "compute"}},
					},
					{
						Count:        1,
						Capabilities: [][]string{{"tpu"}},
					},
				},
			},
			new: api.ContainerResources{
				DeviceReservations: []container.DeviceRequest{
					{
						Driver:       "nvidia",
						Count:        2,
						Capabilities: [][]string{{"gpu", "compute"}},
					},
					{
						Count:        1,
						Capabilities: [][]string{{"tpu"}},
					},
				},
			},
			want: ContainerUpToDate,
		},
		{
			name: "reordered device requests",
			current: api.ContainerResources{
				DeviceReservations: []container.DeviceRequest{
					{
						Driver:       "nvidia",
						Count:        2,
						Capabilities: [][]string{{"gpu"}},
					},
					{
						Count:        1,
						Capabilities: [][]string{{"tpu"}},
					},
				},
			},
			new: api.ContainerResources{
				DeviceReservations: []container.DeviceRequest{
					{
						Count:        1,
						Capabilities: [][]string{{"tpu"}},
					},
					{
						Driver:       "nvidia",
						Count:        2,
						Capabilities: [][]string{{"gpu"}},
					},
				},
			},
			want: ContainerNeedsRecreate,
		},
		{
			name: "complex GPU configuration identical",
			current: api.ContainerResources{
				DeviceReservations: []container.DeviceRequest{
					{
						Driver:       "nvidia",
						Count:        2,
						DeviceIDs:    []string{"0", "1"},
						Capabilities: [][]string{{"gpu", "compute", "utility"}},
						Options: map[string]string{
							"runtime": "nvidia",
							"compute": "exclusive",
						},
					},
				},
			},
			new: api.ContainerResources{
				DeviceReservations: []container.DeviceRequest{
					{
						Driver:       "nvidia",
						Count:        2,
						DeviceIDs:    []string{"0", "1"},
						Capabilities: [][]string{{"gpu", "compute", "utility"}},
						Options: map[string]string{
							"runtime": "nvidia",
							"compute": "exclusive",
						},
					},
				},
			},
			want: ContainerUpToDate,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			currentSpec := api.ServiceSpec{
				Container: api.ContainerSpec{
					Image:     "nvidia/cuda:latest",
					Resources: tt.current,
				},
			}
			newSpec := api.ServiceSpec{
				Container: api.ContainerSpec{
					Image:     "nvidia/cuda:latest",
					Resources: tt.new,
				},
			}

			result := EvalContainerSpecChange(currentSpec, newSpec)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestEvalContainerSpecChange_Ulimits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		current api.ContainerResources
		new     api.ContainerResources
		want    ContainerSpecStatus
	}{
		{
			name:    "empty",
			current: api.ContainerResources{},
			new:     api.ContainerResources{},
			want:    ContainerUpToDate,
		},
		{
			name:    "identical single ulimit",
			current: api.ContainerResources{Ulimits: map[string]api.Ulimit{"nofile": {Soft: 20000, Hard: 40000}}},
			new:     api.ContainerResources{Ulimits: map[string]api.Ulimit{"nofile": {Soft: 20000, Hard: 40000}}},
			want:    ContainerUpToDate,
		},
		{
			name:    "set ulimit",
			current: api.ContainerResources{},
			new:     api.ContainerResources{Ulimits: map[string]api.Ulimit{"nofile": {Soft: 20000, Hard: 40000}}},
			want:    ContainerNeedsRecreate,
		},
		{
			name:    "remove ulimit",
			current: api.ContainerResources{Ulimits: map[string]api.Ulimit{"nofile": {Soft: 20000, Hard: 40000}}},
			new:     api.ContainerResources{},
			want:    ContainerNeedsRecreate,
		},
		{
			name:    "change ulimit soft value",
			current: api.ContainerResources{Ulimits: map[string]api.Ulimit{"nofile": {Soft: 20000, Hard: 40000}}},
			new:     api.ContainerResources{Ulimits: map[string]api.Ulimit{"nofile": {Soft: 30000, Hard: 40000}}},
			want:    ContainerNeedsRecreate,
		},
		{
			name:    "change ulimit hard value",
			current: api.ContainerResources{Ulimits: map[string]api.Ulimit{"nofile": {Soft: 20000, Hard: 40000}}},
			new:     api.ContainerResources{Ulimits: map[string]api.Ulimit{"nofile": {Soft: 20000, Hard: 80000}}},
			want:    ContainerNeedsRecreate,
		},
		{
			name: "add ulimit",
			current: api.ContainerResources{Ulimits: map[string]api.Ulimit{
				"nofile": {Soft: 20000, Hard: 40000},
			}},
			new: api.ContainerResources{Ulimits: map[string]api.Ulimit{
				"nofile": {Soft: 20000, Hard: 40000},
				"nproc":  {Soft: 65535, Hard: 65535},
			}},
			want: ContainerNeedsRecreate,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			currentSpec := api.ServiceSpec{
				Container: api.ContainerSpec{
					Image:     "nginx:latest",
					Resources: tt.current,
				},
			}
			newSpec := api.ServiceSpec{
				Container: api.ContainerSpec{
					Image:     "nginx:latest",
					Resources: tt.new,
				},
			}

			result := EvalContainerSpecChange(currentSpec, newSpec)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestEvalContainerSpecChange_Mixed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		current api.ServiceSpec
		new     api.ServiceSpec
		want    ContainerSpecStatus
	}{
		{
			name: "mutable changes",
			current: api.ServiceSpec{
				Container: api.ContainerSpec{
					Image: "nginx:latest",
					Resources: api.ContainerResources{
						Memory: 100 * 1024 * 1024,
					},
				},
			},
			new: api.ServiceSpec{
				Container: api.ContainerSpec{
					Image: "nginx:latest",
					Resources: api.ContainerResources{
						CPU:               1000000000,
						Memory:            200 * 1024 * 1024,
						MemoryReservation: 100 * 1024 * 1024,
					},
				},
			},
			want: ContainerNeedsUpdate,
		},
		{
			name: "mutable and immutable changes",
			current: api.ServiceSpec{
				Container: api.ContainerSpec{
					Image: "nginx:latest",
				},
			},
			new: api.ServiceSpec{
				Container: api.ContainerSpec{
					Image: "nginx:latest",
					Resources: api.ContainerResources{
						CPU: 1000000000,
					},
					User: "root",
				},
			},
			want: ContainerNeedsRecreate,
		},
		{
			name: "mutable memory change with immutable device reservation change",
			current: api.ServiceSpec{
				Container: api.ContainerSpec{
					Image: "nvidia/cuda:latest",
					Resources: api.ContainerResources{
						Memory: 100 * 1024 * 1024,
					},
				},
			},
			new: api.ServiceSpec{
				Container: api.ContainerSpec{
					Image: "nvidia/cuda:latest",
					Resources: api.ContainerResources{
						Memory: 200 * 1024 * 1024,
						DeviceReservations: []container.DeviceRequest{
							{
								Count:        1,
								Capabilities: [][]string{{"gpu"}},
							},
						},
					},
				},
			},
			want: ContainerNeedsRecreate,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EvalContainerSpecChange(tt.current, tt.new)
			assert.Equal(t, tt.want, result)
		})
	}
}

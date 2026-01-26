package scheduler

import (
	"testing"

	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/volume"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/stretchr/testify/assert"
)

func TestResourceConstraint_Evaluate(t *testing.T) {
	t.Parallel()

	const (
		gb   = 1024 * 1024 * 1024
		core = int64(1e9)
	)

	tests := []struct {
		name       string
		constraint *ResourceConstraint
		machine    *Machine
		want       bool
	}{
		{
			name: "no resource requirements - always passes",
			constraint: &ResourceConstraint{
				RequiredCPU:    0,
				RequiredMemory: 0,
			},
			machine: &Machine{
				Info: &pb.MachineInfo{
					TotalCpuNanos:    4 * core,
					TotalMemoryBytes: 8 * gb,
				},
			},
			want: true,
		},
		{
			name: "no resource requirements - passes even with zero resources",
			constraint: &ResourceConstraint{
				RequiredCPU:    0,
				RequiredMemory: 0,
			},
			machine: &Machine{
				Info: &pb.MachineInfo{
					TotalCpuNanos:    0,
					TotalMemoryBytes: 0,
				},
			},
			want: true,
		},
		{
			name: "CPU only requirement - sufficient resources",
			constraint: &ResourceConstraint{
				RequiredCPU:    2 * core,
				RequiredMemory: 0,
			},
			machine: &Machine{
				Info: &pb.MachineInfo{
					TotalCpuNanos:    4 * core,
					TotalMemoryBytes: 8 * gb,
				},
			},
			want: true,
		},
		{
			name: "CPU only requirement - insufficient resources",
			constraint: &ResourceConstraint{
				RequiredCPU:    4 * core,
				RequiredMemory: 0,
			},
			machine: &Machine{
				Info: &pb.MachineInfo{
					TotalCpuNanos:    2 * core,
					TotalMemoryBytes: 8 * gb,
				},
			},
			want: false,
		},
		{
			name: "memory only requirement - sufficient resources",
			constraint: &ResourceConstraint{
				RequiredCPU:    0,
				RequiredMemory: 4 * gb,
			},
			machine: &Machine{
				Info: &pb.MachineInfo{
					TotalCpuNanos:    4 * core,
					TotalMemoryBytes: 8 * gb,
				},
			},
			want: true,
		},
		{
			name: "memory only requirement - insufficient resources",
			constraint: &ResourceConstraint{
				RequiredCPU:    0,
				RequiredMemory: 16 * gb,
			},
			machine: &Machine{
				Info: &pb.MachineInfo{
					TotalCpuNanos:    4 * core,
					TotalMemoryBytes: 8 * gb,
				},
			},
			want: false,
		},
		{
			name: "both CPU and memory required - both sufficient",
			constraint: &ResourceConstraint{
				RequiredCPU:    2 * core,
				RequiredMemory: 4 * gb,
			},
			machine: &Machine{
				Info: &pb.MachineInfo{
					TotalCpuNanos:    4 * core,
					TotalMemoryBytes: 8 * gb,
				},
			},
			want: true,
		},
		{
			name: "both required - insufficient CPU",
			constraint: &ResourceConstraint{
				RequiredCPU:    8 * core,
				RequiredMemory: 4 * gb,
			},
			machine: &Machine{
				Info: &pb.MachineInfo{
					TotalCpuNanos:    4 * core,
					TotalMemoryBytes: 8 * gb,
				},
			},
			want: false,
		},
		{
			name: "both required - insufficient memory",
			constraint: &ResourceConstraint{
				RequiredCPU:    2 * core,
				RequiredMemory: 16 * gb,
			},
			machine: &Machine{
				Info: &pb.MachineInfo{
					TotalCpuNanos:    4 * core,
					TotalMemoryBytes: 8 * gb,
				},
			},
			want: false,
		},
		{
			name: "exactly matching resources - passes",
			constraint: &ResourceConstraint{
				RequiredCPU:    4 * core,
				RequiredMemory: 8 * gb,
			},
			machine: &Machine{
				Info: &pb.MachineInfo{
					TotalCpuNanos:    4 * core,
					TotalMemoryBytes: 8 * gb,
				},
			},
			want: true,
		},
		{
			name: "accounts for scheduled CPU resources",
			constraint: &ResourceConstraint{
				RequiredCPU:    2 * core,
				RequiredMemory: 0,
			},
			machine: &Machine{
				Info: &pb.MachineInfo{
					TotalCpuNanos:    4 * core,
					TotalMemoryBytes: 8 * gb,
				},
				ScheduledCPU: 3 * core, // Only 1 core available
			},
			want: false,
		},
		{
			name: "accounts for scheduled memory resources",
			constraint: &ResourceConstraint{
				RequiredCPU:    0,
				RequiredMemory: 4 * gb,
			},
			machine: &Machine{
				Info: &pb.MachineInfo{
					TotalCpuNanos:    4 * core,
					TotalMemoryBytes: 8 * gb,
				},
				ScheduledMemory: 6 * gb, // Only 2 GB available
			},
			want: false,
		},
		{
			name: "accounts for both reserved and scheduled CPU resources",
			constraint: &ResourceConstraint{
				RequiredCPU:    2 * core,
				RequiredMemory: 0,
			},
			machine: &Machine{
				Info: &pb.MachineInfo{
					TotalCpuNanos:    4 * core,
					ReservedCpuNanos: 1 * core, // 1 core reserved
					TotalMemoryBytes: 8 * gb,
				},
				ScheduledCPU: 2 * core, // 2 cores scheduled, only 1 available
			},
			want: false,
		},
		{
			name: "accounts for both reserved and scheduled memory resources",
			constraint: &ResourceConstraint{
				RequiredCPU:    0,
				RequiredMemory: 4 * gb,
			},
			machine: &Machine{
				Info: &pb.MachineInfo{
					TotalCpuNanos:       4 * core,
					TotalMemoryBytes:    8 * gb,
					ReservedMemoryBytes: 2 * gb, // 2 GB reserved
				},
				ScheduledMemory: 4 * gb, // 4 GB scheduled, only 2 GB available
			},
			want: false,
		},
		{
			name: "passes with reserved resources when enough available",
			constraint: &ResourceConstraint{
				RequiredCPU:    1 * core,
				RequiredMemory: 2 * gb,
			},
			machine: &Machine{
				Info: &pb.MachineInfo{
					TotalCpuNanos:       4 * core,
					ReservedCpuNanos:    1 * core,
					TotalMemoryBytes:    8 * gb,
					ReservedMemoryBytes: 2 * gb,
				},
				ScheduledCPU:    1 * core,
				ScheduledMemory: 2 * gb,
			},
			want: true, // 1 core and 2 GB still available
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.constraint.Evaluate(tt.machine)
			assert.Equal(t, tt.want, result.Satisfied)
			if !tt.want {
				assert.NotEmpty(t, result.Reason, "failing constraint should have a reason")
			}
		})
	}
}

func TestResourceConstraint_Description(t *testing.T) {
	t.Parallel()

	const (
		mb   = 1024 * 1024
		core = int64(1e9)
	)

	tests := []struct {
		name       string
		constraint *ResourceConstraint
		want       string
	}{
		{
			name: "no requirements",
			constraint: &ResourceConstraint{
				RequiredCPU:    0,
				RequiredMemory: 0,
			},
			want: "No resource constraint",
		},
		{
			name: "CPU only",
			constraint: &ResourceConstraint{
				RequiredCPU:    2 * core,
				RequiredMemory: 0,
			},
			want: "Resource reservation: CPU: 2.00 cores",
		},
		{
			name: "memory only",
			constraint: &ResourceConstraint{
				RequiredCPU:    0,
				RequiredMemory: 512 * mb,
			},
			want: "Resource reservation: Memory: 512 MB",
		},
		{
			name: "both CPU and memory",
			constraint: &ResourceConstraint{
				RequiredCPU:    1500000000, // 1.5 cores
				RequiredMemory: 1024 * mb,
			},
			want: "Resource reservation: CPU: 1.50 cores, Memory: 1024 MB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.constraint.Description()
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestPlacementConstraint_Evaluate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		constraint *PlacementConstraint
		machine    *Machine
		want       bool
	}{
		{
			name: "machine matches by ID",
			constraint: &PlacementConstraint{
				Machines: []string{"machine-1"},
			},
			machine: &Machine{
				Info: &pb.MachineInfo{
					Id:   "machine-1",
					Name: "node1",
				},
			},
			want: true,
		},
		{
			name: "machine matches by name",
			constraint: &PlacementConstraint{
				Machines: []string{"node1"},
			},
			machine: &Machine{
				Info: &pb.MachineInfo{
					Id:   "machine-1",
					Name: "node1",
				},
			},
			want: true,
		},
		{
			name: "machine not in list",
			constraint: &PlacementConstraint{
				Machines: []string{"machine-2", "machine-3"},
			},
			machine: &Machine{
				Info: &pb.MachineInfo{
					Id:   "machine-1",
					Name: "node1",
				},
			},
			want: false,
		},
		{
			name: "multiple machines - matches one",
			constraint: &PlacementConstraint{
				Machines: []string{"machine-1", "machine-2"},
			},
			machine: &Machine{
				Info: &pb.MachineInfo{
					Id:   "machine-2",
					Name: "node2",
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.constraint.Evaluate(tt.machine)
			assert.Equal(t, tt.want, result.Satisfied)
			if !tt.want {
				assert.NotEmpty(t, result.Reason, "failing constraint should have a reason")
				assert.Contains(t, result.Reason, "not in allowed list")
			}
		})
	}
}

func TestVolumesConstraint_Evaluate(t *testing.T) {
	t.Parallel()

	volumeSpec := api.VolumeSpec{
		Name:          "data",
		Type:          api.VolumeTypeVolume,
		VolumeOptions: &api.VolumeOptions{Driver: &mount.Driver{Name: api.VolumeDriverLocal}},
	}

	tests := []struct {
		name    string
		machine *Machine
		want    bool
	}{
		{
			name: "passes when volume exists on machine",
			machine: &Machine{
				Info: &pb.MachineInfo{Id: "m1"},
				Volumes: []volume.Volume{{
					Name:   volumeSpec.DockerVolumeName(),
					Driver: api.VolumeDriverLocal,
				}},
			},
			want: true,
		},
		{
			name: "passes when volume is scheduled on machine",
			machine: &Machine{
				Info:             &pb.MachineInfo{Id: "m1"},
				ScheduledVolumes: []api.VolumeSpec{volumeSpec},
			},
			want: true,
		},
		{
			name:    "fails when volume missing",
			machine: &Machine{Info: &pb.MachineInfo{Id: "m1"}},
			want:    false,
		},
		{
			name: "fails when scheduled volume driver mismatches",
			machine: &Machine{
				Info: &pb.MachineInfo{Id: "m1"},
				ScheduledVolumes: []api.VolumeSpec{{
					Name:          "data",
					Type:          api.VolumeTypeVolume,
					VolumeOptions: &api.VolumeOptions{Driver: &mount.Driver{Name: "nfs"}},
				}},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &VolumesConstraint{Volumes: []api.VolumeSpec{volumeSpec}}
			result := c.Evaluate(tt.machine)
			assert.Equal(t, tt.want, result.Satisfied)
			if !tt.want {
				assert.NotEmpty(t, result.Reason, "failing constraint should have a reason")
				assert.Contains(t, result.Reason, "not found on machine")
			}
		})
	}
}

func TestConstraintsFromSpec(t *testing.T) {
	t.Parallel()

	const (
		core = int64(1e9)
		mb   = int64(1024 * 1024)
	)

	tests := []struct {
		name                   string
		spec                   api.ServiceSpec
		wantPlacement          bool
		wantVolumes            bool
		wantResources          bool
		wantResourceConstraint *ResourceConstraint
	}{
		{
			name:          "empty spec - no constraints",
			spec:          api.ServiceSpec{},
			wantPlacement: false,
			wantVolumes:   false,
			wantResources: false,
		},
		{
			name: "placement machines set",
			spec: api.ServiceSpec{
				Placement: api.Placement{
					Machines: []string{"machine-1"},
				},
			},
			wantPlacement: true,
			wantVolumes:   false,
			wantResources: false,
		},
		{
			name: "volume mounts with VolumeTypeVolume",
			spec: api.ServiceSpec{
				Container: api.ContainerSpec{
					VolumeMounts: []api.VolumeMount{
						{VolumeName: "data", ContainerPath: "/data"},
					},
				},
				Volumes: []api.VolumeSpec{
					{Name: "data", Type: api.VolumeTypeVolume},
				},
			},
			wantPlacement: false,
			wantVolumes:   true,
			wantResources: false,
		},
		{
			name: "CPU reservation only",
			spec: api.ServiceSpec{
				Container: api.ContainerSpec{
					Resources: api.ContainerResources{
						CPUReservation: 2 * core,
					},
				},
			},
			wantPlacement: false,
			wantVolumes:   false,
			wantResources: true,
			wantResourceConstraint: &ResourceConstraint{
				RequiredCPU:    2 * core,
				RequiredMemory: 0,
			},
		},
		{
			name: "memory reservation only",
			spec: api.ServiceSpec{
				Container: api.ContainerSpec{
					Resources: api.ContainerResources{
						MemoryReservation: 512 * mb,
					},
				},
			},
			wantPlacement: false,
			wantVolumes:   false,
			wantResources: true,
			wantResourceConstraint: &ResourceConstraint{
				RequiredCPU:    0,
				RequiredMemory: 512 * mb,
			},
		},
		{
			name: "both CPU and memory reservations",
			spec: api.ServiceSpec{
				Container: api.ContainerSpec{
					Resources: api.ContainerResources{
						CPUReservation:    2 * core,
						MemoryReservation: 512 * mb,
					},
				},
			},
			wantPlacement: false,
			wantVolumes:   false,
			wantResources: true,
			wantResourceConstraint: &ResourceConstraint{
				RequiredCPU:    2 * core,
				RequiredMemory: 512 * mb,
			},
		},
		{
			name: "combined placement + volumes + resources",
			spec: api.ServiceSpec{
				Placement: api.Placement{
					Machines: []string{"machine-1"},
				},
				Container: api.ContainerSpec{
					VolumeMounts: []api.VolumeMount{
						{VolumeName: "data", ContainerPath: "/data"},
					},
					Resources: api.ContainerResources{
						CPUReservation:    1 * core,
						MemoryReservation: 256 * mb,
					},
				},
				Volumes: []api.VolumeSpec{
					{Name: "data", Type: api.VolumeTypeVolume},
				},
			},
			wantPlacement: true,
			wantVolumes:   true,
			wantResources: true,
			wantResourceConstraint: &ResourceConstraint{
				RequiredCPU:    1 * core,
				RequiredMemory: 256 * mb,
			},
		},
		{
			name: "bind volume does not create volumes constraint",
			spec: api.ServiceSpec{
				Container: api.ContainerSpec{
					VolumeMounts: []api.VolumeMount{
						{VolumeName: "config", ContainerPath: "/config"},
					},
				},
				Volumes: []api.VolumeSpec{
					{Name: "config", Type: api.VolumeTypeBind, BindOptions: &api.BindOptions{HostPath: "/host/config"}},
				},
			},
			wantPlacement: false,
			wantVolumes:   false,
			wantResources: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			constraints := constraintsFromSpec(tt.spec)

			var foundPlacement, foundVolumes, foundResources bool
			var resourceConstraint *ResourceConstraint

			for _, c := range constraints {
				switch v := c.(type) {
				case *PlacementConstraint:
					foundPlacement = true
				case *VolumesConstraint:
					foundVolumes = true
				case *ResourceConstraint:
					foundResources = true
					resourceConstraint = v
				}
			}

			assert.Equal(t, tt.wantPlacement, foundPlacement, "PlacementConstraint")
			assert.Equal(t, tt.wantVolumes, foundVolumes, "VolumesConstraint")
			assert.Equal(t, tt.wantResources, foundResources, "ResourceConstraint")

			if tt.wantResourceConstraint != nil {
				assert.Equal(t, tt.wantResourceConstraint, resourceConstraint)
			}
		})
	}
}

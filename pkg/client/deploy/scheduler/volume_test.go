package scheduler

import (
	"testing"

	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/volume"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	core = int64(1e9)
	gb   = int64(1e9)
)

func TestVolumeScheduler_Schedule(t *testing.T) {
	tests := []struct {
		name         string
		machines     []*Machine
		serviceSpecs []api.ServiceSpec
		want         map[string][]api.VolumeSpec
		wantErr      string
	}{
		{
			name: "single service with missing volume",
			machines: []*Machine{
				{
					Info: &pb.MachineInfo{
						Id: "machine1",
					},
				},
			},
			serviceSpecs: []api.ServiceSpec{
				{
					Name: "service1",
					Container: api.ContainerSpec{
						Image: "portainer/pause:latest",
						VolumeMounts: []api.VolumeMount{
							{
								VolumeName:    "vol1",
								ContainerPath: "/data",
							},
						},
					},
					Volumes: []api.VolumeSpec{
						{
							Name: "vol1",
							Type: api.VolumeTypeVolume,
						},
					},
				},
			},
			want: map[string][]api.VolumeSpec{
				"machine1": {
					{
						Name: "vol1",
						Type: api.VolumeTypeVolume,
					},
				},
			},
		},
		{
			name: "multiple services sharing a missing volume",
			machines: []*Machine{
				{
					Info: &pb.MachineInfo{
						Id: "machine1",
					},
					Volumes: []volume.Volume{},
				},
				{
					Info: &pb.MachineInfo{
						Id: "machine2",
					},
					Volumes: []volume.Volume{},
				},
			},
			serviceSpecs: []api.ServiceSpec{
				{
					Name: "service1",
					Container: api.ContainerSpec{
						Image: "portainer/pause:latest",
						VolumeMounts: []api.VolumeMount{
							{
								VolumeName:    "vol1",
								ContainerPath: "/data",
							},
						},
					},
					Volumes: []api.VolumeSpec{
						{
							Name: "vol1",
							Type: api.VolumeTypeVolume,
						},
					},
				},
				{
					Name: "service2",
					Container: api.ContainerSpec{
						Image: "portainer/pause:latest",
						VolumeMounts: []api.VolumeMount{
							{
								VolumeName:    "vol1",
								ContainerPath: "/data",
							},
						},
					},
					Volumes: []api.VolumeSpec{
						{
							Name: "vol1",
							Type: api.VolumeTypeVolume,
						},
					},
				},
			},
			want: map[string][]api.VolumeSpec{
				"machine1": {
					{
						Name: "vol1",
						Type: api.VolumeTypeVolume,
					},
				},
			},
		},
		{
			name: "service with existing volume",
			machines: []*Machine{
				{
					Info: &pb.MachineInfo{
						Id: "machine1",
					},
				},
				{
					Info: &pb.MachineInfo{
						Id: "machine2",
					},
					Volumes: []volume.Volume{
						{
							Name: "vol1",
						},
					},
				},
			},
			serviceSpecs: []api.ServiceSpec{
				{
					Name: "service1",
					Container: api.ContainerSpec{
						Image: "portainer/pause:latest",
						VolumeMounts: []api.VolumeMount{
							{
								VolumeName:    "vol1",
								ContainerPath: "/data",
							},
						},
					},
					Volumes: []api.VolumeSpec{
						{
							Name: "vol1",
							Type: api.VolumeTypeVolume,
						},
					},
				},
			},
			want: map[string][]api.VolumeSpec{},
		},
		{
			name: "service with placement constraint and missing volume",
			machines: []*Machine{
				{
					Info: &pb.MachineInfo{
						Id: "machine1",
					},
					Volumes: []volume.Volume{},
				},
				{
					Info: &pb.MachineInfo{
						Id: "machine2",
					},
					Volumes: []volume.Volume{},
				},
			},
			serviceSpecs: []api.ServiceSpec{
				{
					Name: "service1",
					Placement: api.Placement{
						Machines: []string{"machine2"},
					},
					Container: api.ContainerSpec{
						Image: "portainer/pause:latest",
						VolumeMounts: []api.VolumeMount{
							{
								VolumeName:    "vol1",
								ContainerPath: "/data",
							},
						},
					},
					Volumes: []api.VolumeSpec{
						{
							Name: "vol1",
							Type: api.VolumeTypeVolume,
						},
					},
				},
			},
			want: map[string][]api.VolumeSpec{
				"machine2": {
					{
						Name: "vol1",
						Type: api.VolumeTypeVolume,
					},
				},
			},
		},
		{
			name: "services with conflicting placement constraints",
			machines: []*Machine{
				{
					Info: &pb.MachineInfo{
						Id: "machine1",
					},
					Volumes: []volume.Volume{},
				},
				{
					Info: &pb.MachineInfo{
						Id: "machine2",
					},
					Volumes: []volume.Volume{},
				},
			},
			serviceSpecs: []api.ServiceSpec{
				{
					Name: "service1",
					Placement: api.Placement{
						Machines: []string{"machine1"},
					},
					Container: api.ContainerSpec{
						Image: "portainer/pause:latest",
						VolumeMounts: []api.VolumeMount{
							{
								VolumeName:    "vol1",
								ContainerPath: "/data",
							},
						},
					},
					Volumes: []api.VolumeSpec{
						{
							Name: "vol1",
							Type: api.VolumeTypeVolume,
						},
					},
				},
				{
					Name: "service2",
					Placement: api.Placement{
						Machines: []string{"machine2"},
					},
					Container: api.ContainerSpec{
						Image: "portainer/pause:latest",
						VolumeMounts: []api.VolumeMount{
							{
								VolumeName:    "vol1",
								ContainerPath: "/data",
							},
						},
					},
					Volumes: []api.VolumeSpec{
						{
							Name: "vol1",
							Type: api.VolumeTypeVolume,
						},
					},
				},
			},
			wantErr: "unable to find a machine that satisfies placement constraints for services " +
				"'service1', 'service2' that must be placed together to share volume 'vol1'",
		},
		{
			name: "service with existing volume on wrong machine",
			machines: []*Machine{
				{
					Info: &pb.MachineInfo{
						Id: "machine1",
					},
					Volumes: []volume.Volume{
						{
							Name: "vol1",
						},
					},
				},
				{
					Info: &pb.MachineInfo{
						Id: "machine2",
					},
					Volumes: []volume.Volume{},
				},
			},
			serviceSpecs: []api.ServiceSpec{
				{
					Name: "service1",
					Placement: api.Placement{
						Machines: []string{"machine2"},
					},
					Container: api.ContainerSpec{
						Image: "portainer/pause:latest",
						VolumeMounts: []api.VolumeMount{
							{
								VolumeName:    "vol1",
								ContainerPath: "/data",
							},
						},
					},
					Volumes: []api.VolumeSpec{
						{
							Name: "vol1",
							Type: api.VolumeTypeVolume,
						},
					},
				},
			},
			wantErr: "unable to find a machine that satisfies service 'service1' placement constraints " +
				"and has all required volumes: 'vol1'",
		},
		{
			name: "multiple services with multiple volumes, some shared",
			machines: []*Machine{
				{
					Info: &pb.MachineInfo{
						Id: "machine1",
					},
					Volumes: []volume.Volume{},
				},
				{
					Info: &pb.MachineInfo{
						Id: "machine2",
					},
					Volumes: []volume.Volume{},
				},
			},
			serviceSpecs: []api.ServiceSpec{
				{
					Name: "service1",
					Container: api.ContainerSpec{
						Image: "portainer/pause:latest",
						VolumeMounts: []api.VolumeMount{
							{
								VolumeName:    "vol1",
								ContainerPath: "/data1",
							},
							{
								VolumeName:    "vol2",
								ContainerPath: "/data2",
							},
						},
					},
					Volumes: []api.VolumeSpec{
						{
							Name: "vol1",
							Type: api.VolumeTypeVolume,
						},
						{
							Name: "vol2",
							Type: api.VolumeTypeVolume,
						},
					},
				},
				{
					Name: "service2",
					Container: api.ContainerSpec{
						Image: "portainer/pause:latest",
						VolumeMounts: []api.VolumeMount{
							{
								VolumeName:    "vol2",
								ContainerPath: "/data2",
							},
							{
								VolumeName:    "vol3",
								ContainerPath: "/data3",
							},
						},
					},
					Volumes: []api.VolumeSpec{
						{
							Name: "vol2",
							Type: api.VolumeTypeVolume,
						},
						{
							Name: "vol3",
							Type: api.VolumeTypeVolume,
						},
					},
				},
				{
					Name: "service3",
					Container: api.ContainerSpec{
						Image: "portainer/pause:latest",
						VolumeMounts: []api.VolumeMount{
							{
								VolumeName:    "vol3",
								ContainerPath: "/data3",
							},
							{
								VolumeName:    "vol4",
								ContainerPath: "/data4",
							},
						},
					},
					Volumes: []api.VolumeSpec{
						{
							Name: "vol3",
							Type: api.VolumeTypeVolume,
						},
						{
							Name: "vol4",
							Type: api.VolumeTypeVolume,
						},
					},
				},
			},
			want: map[string][]api.VolumeSpec{
				"machine1": {
					{
						Name: "vol1",
						Type: api.VolumeTypeVolume,
					},
					{
						Name: "vol2",
						Type: api.VolumeTypeVolume,
					},
					{
						Name: "vol3",
						Type: api.VolumeTypeVolume,
					},
					{
						Name: "vol4",
						Type: api.VolumeTypeVolume,
					},
				},
			},
		},
		{
			name: "multiple services with multiple volumes, some shared and existing",
			machines: []*Machine{
				{
					Info: &pb.MachineInfo{
						Id: "machine1",
					},
				},
				{
					Info: &pb.MachineInfo{
						Id: "machine2",
					},
					Volumes: []volume.Volume{
						{
							Name: "vol2",
						},
					},
				},
				{
					Info: &pb.MachineInfo{
						Id: "machine3",
					},
					Volumes: []volume.Volume{
						{
							Name: "vol1",
						},
						{
							Name: "vol2",
						},
					},
				},
			},
			serviceSpecs: []api.ServiceSpec{
				{
					Name: "service1",
					Container: api.ContainerSpec{
						Image: "portainer/pause:latest",
						VolumeMounts: []api.VolumeMount{
							{
								VolumeName:    "vol3",
								ContainerPath: "/data3",
							},
							{
								VolumeName:    "vol4",
								ContainerPath: "/data4",
							},
						},
					},
					Volumes: []api.VolumeSpec{
						{
							Name: "vol3",
							Type: api.VolumeTypeVolume,
						},
						{
							Name: "vol4",
							Type: api.VolumeTypeVolume,
						},
					},
				},
				{
					Name: "service2",
					Container: api.ContainerSpec{
						Image: "portainer/pause:latest",
						VolumeMounts: []api.VolumeMount{
							{
								VolumeName:    "vol1",
								ContainerPath: "/data1",
							},
							{
								VolumeName:    "vol2",
								ContainerPath: "/data2",
							},
							{
								VolumeName:    "vol3",
								ContainerPath: "/data3",
							},
						},
					},
					Volumes: []api.VolumeSpec{
						{
							Name: "vol1",
							Type: api.VolumeTypeVolume,
						},
						{
							Name: "vol2",
							Type: api.VolumeTypeVolume,
							VolumeOptions: &api.VolumeOptions{
								Driver: &mount.Driver{
									Name: api.VolumeDriverLocal,
								},
							},
						},
						{
							Name: "vol3",
							Type: api.VolumeTypeVolume,
						},
					},
				},
				{
					Name: "service3",
					Container: api.ContainerSpec{
						Image: "portainer/pause:latest",
						VolumeMounts: []api.VolumeMount{
							{
								VolumeName:    "vol2",
								ContainerPath: "/data2",
							},
							{
								VolumeName:    "vol4-alias",
								ContainerPath: "/data4",
							},
						},
					},
					Volumes: []api.VolumeSpec{
						{
							Name: "vol2",
							Type: api.VolumeTypeVolume,
							VolumeOptions: &api.VolumeOptions{
								Driver: &mount.Driver{
									Name: api.VolumeDriverLocal,
								},
							},
						},
						{
							Name: "vol4-alias",
							Type: api.VolumeTypeVolume,
							VolumeOptions: &api.VolumeOptions{
								Name: "vol4",
							},
						},
					},
				},
				{
					Name: "service4",
					Container: api.ContainerSpec{
						Image: "portainer/pause:latest",
						VolumeMounts: []api.VolumeMount{
							{
								VolumeName:    "vol2-alias",
								ContainerPath: "/data2",
							},
							{
								VolumeName:    "vol5-with-driver",
								ContainerPath: "/data5",
							},
						},
					},
					Volumes: []api.VolumeSpec{
						{
							Name: "vol2-alias",
							Type: api.VolumeTypeVolume,
							VolumeOptions: &api.VolumeOptions{
								Name: "vol2",
								Driver: &mount.Driver{
									Name: api.VolumeDriverLocal,
								},
							},
						},
						{
							Name: "vol5-with-driver",
							Type: api.VolumeTypeVolume,
							VolumeOptions: &api.VolumeOptions{
								Name: "vol5",
								Driver: &mount.Driver{
									Name: api.VolumeDriverLocal,
								},
							},
						},
					},
				},
			},
			want: map[string][]api.VolumeSpec{
				"machine2": {
					{
						Name: "vol5",
						Type: api.VolumeTypeVolume,
						VolumeOptions: &api.VolumeOptions{
							Driver: &mount.Driver{
								Name: api.VolumeDriverLocal,
							},
						},
					},
				},
				"machine3": {
					{
						Name: "vol3",
						Type: api.VolumeTypeVolume,
					},
					{
						Name: "vol4",
						Type: api.VolumeTypeVolume,
					},
				},
			},
		},
		{
			name: "multiple services with multiple volumes, some shared, with conflicting placement constraints",
			machines: []*Machine{
				{
					Info: &pb.MachineInfo{
						Id: "machine1",
					},
					Volumes: []volume.Volume{},
				},
				{
					Info: &pb.MachineInfo{
						Id: "machine2",
					},
					Volumes: []volume.Volume{},
				},
			},
			serviceSpecs: []api.ServiceSpec{
				{
					Name: "service1",
					Placement: api.Placement{
						Machines: []string{"machine1"},
					},
					Container: api.ContainerSpec{
						Image: "portainer/pause:latest",
						VolumeMounts: []api.VolumeMount{
							{
								VolumeName:    "vol1",
								ContainerPath: "/data1",
							},
							{
								VolumeName:    "vol2",
								ContainerPath: "/data2",
							},
						},
					},
					Volumes: []api.VolumeSpec{
						{
							Name: "vol1",
							Type: api.VolumeTypeVolume,
						},
						{
							Name: "vol2",
							Type: api.VolumeTypeVolume,
						},
					},
				},
				{
					Name: "service2",
					Placement: api.Placement{
						Machines: []string{"machine2"},
					},
					Container: api.ContainerSpec{
						Image: "portainer/pause:latest",
						VolumeMounts: []api.VolumeMount{
							{
								VolumeName:    "vol2",
								ContainerPath: "/data2",
							},
							{
								VolumeName:    "vol3",
								ContainerPath: "/data3",
							},
						},
					},
					Volumes: []api.VolumeSpec{
						{
							Name: "vol2",
							Type: api.VolumeTypeVolume,
						},
						{
							Name: "vol3",
							Type: api.VolumeTypeVolume,
						},
					},
				},
			},
			wantErr: "unable to find a machine that satisfies placement constraints for services " +
				"'service1', 'service2' that must be placed together to share volume 'vol2'",
		},
		{
			name: "multiple services with multiple volumes, no shared",
			machines: []*Machine{
				{
					Info: &pb.MachineInfo{
						Id: "machine1",
					},
					Volumes: []volume.Volume{
						{
							Name: "vol1",
						},
					},
				},
				{
					Info: &pb.MachineInfo{
						Id: "machine2",
					},
					Volumes: []volume.Volume{
						{
							Name: "vol3",
						},
					},
				},
			},
			serviceSpecs: []api.ServiceSpec{
				{
					Name: "service1",
					Container: api.ContainerSpec{
						Image: "portainer/pause:latest",
						VolumeMounts: []api.VolumeMount{
							{
								VolumeName:    "vol1",
								ContainerPath: "/data1",
							},
							{
								VolumeName:    "vol2",
								ContainerPath: "/data2",
							},
						},
					},
					Volumes: []api.VolumeSpec{
						{
							Name: "vol1",
							Type: api.VolumeTypeVolume,
						},
						{
							Name: "vol2",
							Type: api.VolumeTypeVolume,
						},
					},
				},
				{
					Name: "service2",
					Container: api.ContainerSpec{
						Image: "portainer/pause:latest",
						VolumeMounts: []api.VolumeMount{
							{
								VolumeName:    "vol3",
								ContainerPath: "/data3",
							},
							{
								VolumeName:    "vol4",
								ContainerPath: "/data4",
							},
						},
					},
					Volumes: []api.VolumeSpec{
						{
							Name: "vol3",
							Type: api.VolumeTypeVolume,
						},
						{
							Name: "vol4",
							Type: api.VolumeTypeVolume,
						},
					},
				},
			},
			want: map[string][]api.VolumeSpec{
				"machine1": {
					{
						Name: "vol2",
						Type: api.VolumeTypeVolume,
					},
				},
				"machine2": {
					{
						Name: "vol4",
						Type: api.VolumeTypeVolume,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &ClusterState{
				Machines: tt.machines,
			}
			scheduler, err := NewVolumeScheduler(state, tt.serviceSpecs)
			require.NoError(t, err)
			result, err := scheduler.Schedule()

			if tt.wantErr != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				assert.NoError(t, err)

				assert.Len(t, result, len(tt.want), "Number of machines with volumes to create should match")
				for machineID, expectedVolumes := range tt.want {
					// Transform the expected volumes to the canonical form with defaults set.
					for i := range expectedVolumes {
						expectedVolumes[i] = expectedVolumes[i].SetDefaults()
					}

					actualVolumes, ok := result[machineID]
					assert.True(t, ok, "Machine %s should be in the result", machineID)
					assert.ElementsMatch(t, expectedVolumes, actualVolumes,
						"Volumes for machine %s should match", machineID)
				}
			}
		})
	}
}

func TestVolumeScheduler_ResourceBudget_CombinedExceedsCapacity(t *testing.T) {
	// Machine has 3 cores, but services sharing volume need 4 cores combined (2+2).
	// BUG: Current code assigns volume to machine, then service scheduling fails later.
	// FIX: VolumeScheduler should fail with clear error about insufficient resources.
	machines := []*Machine{
		{
			Info: &pb.MachineInfo{
				Id:               "machine1",
				Name:             "node1",
				TotalCpuNanos:    3 * core, // Only 3 cores available
				TotalMemoryBytes: 8 * gb,
			},
		},
	}
	serviceSpecs := []api.ServiceSpec{
		{
			Name: "service1",
			Container: api.ContainerSpec{
				Image: "test:latest",
				Resources: api.ContainerResources{
					CPUReservation: 2 * core, // Needs 2 cores
				},
				VolumeMounts: []api.VolumeMount{
					{VolumeName: "shared-vol", ContainerPath: "/data"},
				},
			},
			Volumes: []api.VolumeSpec{
				{Name: "shared-vol", Type: api.VolumeTypeVolume},
			},
		},
		{
			Name: "service2",
			Container: api.ContainerSpec{
				Image: "test:latest",
				Resources: api.ContainerResources{
					CPUReservation: 2 * core, // Needs 2 cores
				},
				VolumeMounts: []api.VolumeMount{
					{VolumeName: "shared-vol", ContainerPath: "/data"},
				},
			},
			Volumes: []api.VolumeSpec{
				{Name: "shared-vol", Type: api.VolumeTypeVolume},
			},
		},
	}

	state := &ClusterState{Machines: machines}
	scheduler, err := NewVolumeScheduler(state, serviceSpecs)
	require.NoError(t, err)

	_, err = scheduler.Schedule()
	// Should fail because combined need (4 cores) > machine capacity (3 cores)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "insufficient")
	assert.Contains(t, err.Error(), "shared-vol")
}

func TestVolumeScheduler_ResourceBudget_ReplicasMultiply(t *testing.T) {
	// Service has 3 replicas × 2 cores = 6 cores needed on the machine.
	// Machine only has 4 cores.
	// BUG: Current code checks 1 replica fits, assigns volume.
	// FIX: Should check all replicas fit.
	machines := []*Machine{
		{
			Info: &pb.MachineInfo{
				Id:               "machine1",
				Name:             "node1",
				TotalCpuNanos:    4 * core, // Only 4 cores
				TotalMemoryBytes: 8 * gb,
			},
		},
	}
	serviceSpecs := []api.ServiceSpec{
		{
			Name:     "replicated-svc",
			Mode:     api.ServiceModeReplicated,
			Replicas: 3, // 3 replicas × 2 cores = 6 cores needed
			Container: api.ContainerSpec{
				Image: "test:latest",
				Resources: api.ContainerResources{
					CPUReservation: 2 * core,
				},
				VolumeMounts: []api.VolumeMount{
					{VolumeName: "data-vol", ContainerPath: "/data"},
				},
			},
			Volumes: []api.VolumeSpec{
				{Name: "data-vol", Type: api.VolumeTypeVolume},
			},
		},
	}

	state := &ClusterState{Machines: machines}
	scheduler, err := NewVolumeScheduler(state, serviceSpecs)
	require.NoError(t, err)

	_, err = scheduler.Schedule()
	// Should fail because 3 × 2 = 6 cores needed > 4 cores available
	require.Error(t, err)
	assert.Contains(t, err.Error(), "insufficient")
}

func TestVolumeScheduler_ResourceBudget_SelectsCapableMachine(t *testing.T) {
	// Machine A (sorted first): 3 cores - not enough for combined need of 4 cores
	// Machine B: 6 cores - sufficient
	// BUG: Current code may pick Machine A (first in sorted order).
	// FIX: Should pick Machine B (only one with sufficient capacity).
	machines := []*Machine{
		{
			Info: &pb.MachineInfo{
				Id:               "aaa-machine", // Sorted first alphabetically
				Name:             "small-node",
				TotalCpuNanos:    3 * core, // Insufficient for combined need
				TotalMemoryBytes: 8 * gb,
			},
		},
		{
			Info: &pb.MachineInfo{
				Id:               "bbb-machine", // Sorted second
				Name:             "large-node",
				TotalCpuNanos:    6 * core, // Sufficient for combined need
				TotalMemoryBytes: 8 * gb,
			},
		},
	}
	serviceSpecs := []api.ServiceSpec{
		{
			Name: "service1",
			Container: api.ContainerSpec{
				Image: "test:latest",
				Resources: api.ContainerResources{
					CPUReservation: 2 * core,
				},
				VolumeMounts: []api.VolumeMount{
					{VolumeName: "shared-vol", ContainerPath: "/data"},
				},
			},
			Volumes: []api.VolumeSpec{
				{Name: "shared-vol", Type: api.VolumeTypeVolume},
			},
		},
		{
			Name: "service2",
			Container: api.ContainerSpec{
				Image: "test:latest",
				Resources: api.ContainerResources{
					CPUReservation: 2 * core,
				},
				VolumeMounts: []api.VolumeMount{
					{VolumeName: "shared-vol", ContainerPath: "/data"},
				},
			},
			Volumes: []api.VolumeSpec{
				{Name: "shared-vol", Type: api.VolumeTypeVolume},
			},
		},
	}

	state := &ClusterState{Machines: machines}
	scheduler, err := NewVolumeScheduler(state, serviceSpecs)
	require.NoError(t, err)

	result, err := scheduler.Schedule()
	require.NoError(t, err)

	// Volume should be placed on bbb-machine (the only one with 6 cores)
	_, hasBBB := result["bbb-machine"]
	assert.True(t, hasBBB, "volume should be placed on machine with sufficient capacity")
	_, hasAAA := result["aaa-machine"]
	assert.False(t, hasAAA, "volume should NOT be placed on machine with insufficient capacity")
}

func TestVolumeScheduler_IndependentVolumesShouldNotOvercommit(t *testing.T) {
	// Machine A: 4 cores, Machine B: 1 core
	// Small service: 1 core with "aaa-vol" (alphabetically first)
	// Big service: 4 cores with "zzz-vol" (alphabetically last)
	//
	// Without resource-based sorting, alphabetical order would schedule aaa-vol first,
	// taking machine-a, then zzz-vol can't fit anywhere (needs 4 cores, A has 0 left, B has 1).
	//
	// With resource-based sorting, zzz-vol (4 cores) schedules first on A,
	// then aaa-vol (1 core) goes to B. Both fit.
	machines := []*Machine{
		{Info: &pb.MachineInfo{Id: "machine-a", TotalCpuNanos: 4 * core, TotalMemoryBytes: 8 * gb}},
		{Info: &pb.MachineInfo{Id: "machine-b", TotalCpuNanos: 1 * core, TotalMemoryBytes: 8 * gb}},
	}
	serviceSpecs := []api.ServiceSpec{
		{
			Name: "small-svc",
			Container: api.ContainerSpec{
				Image:        "test:latest",
				Resources:    api.ContainerResources{CPUReservation: 1 * core},
				VolumeMounts: []api.VolumeMount{{VolumeName: "aaa-vol", ContainerPath: "/data"}},
			},
			Volumes: []api.VolumeSpec{{Name: "aaa-vol", Type: api.VolumeTypeVolume}},
		},
		{
			Name: "big-svc",
			Container: api.ContainerSpec{
				Image:        "test:latest",
				Resources:    api.ContainerResources{CPUReservation: 4 * core},
				VolumeMounts: []api.VolumeMount{{VolumeName: "zzz-vol", ContainerPath: "/data"}},
			},
			Volumes: []api.VolumeSpec{{Name: "zzz-vol", Type: api.VolumeTypeVolume}},
		},
	}

	state := &ClusterState{Machines: machines}
	scheduler, err := NewVolumeScheduler(state, serviceSpecs)
	require.NoError(t, err)

	result, err := scheduler.Schedule()
	require.NoError(t, err)

	// zzz-vol (big) must go to A (only machine with 4 cores).
	// aaa-vol (small) should go to B (so total fits).
	assert.Contains(t, result, "machine-a", "zzz-vol should be on machine-a")
	assert.Contains(t, result, "machine-b", "aaa-vol should be on machine-b")
	assert.Len(t, result["machine-a"], 1, "machine-a should have exactly 1 volume")
	assert.Len(t, result["machine-b"], 1, "machine-b should have exactly 1 volume")
}

func TestVolumeScheduler_SpreadsVolumesAcrossMachines(t *testing.T) {
	// Three independent volumes should spread across three machines.
	machines := []*Machine{
		{Info: &pb.MachineInfo{Id: "machine1", TotalCpuNanos: 4 * core, TotalMemoryBytes: 8 * gb}},
		{Info: &pb.MachineInfo{Id: "machine2", TotalCpuNanos: 4 * core, TotalMemoryBytes: 8 * gb}},
		{Info: &pb.MachineInfo{Id: "machine3", TotalCpuNanos: 4 * core, TotalMemoryBytes: 8 * gb}},
	}
	serviceSpecs := []api.ServiceSpec{
		{
			Name:      "svc1",
			Container: api.ContainerSpec{Image: "test:latest", VolumeMounts: []api.VolumeMount{{VolumeName: "vol1", ContainerPath: "/data"}}},
			Volumes:   []api.VolumeSpec{{Name: "vol1", Type: api.VolumeTypeVolume}},
		},
		{
			Name:      "svc2",
			Container: api.ContainerSpec{Image: "test:latest", VolumeMounts: []api.VolumeMount{{VolumeName: "vol2", ContainerPath: "/data"}}},
			Volumes:   []api.VolumeSpec{{Name: "vol2", Type: api.VolumeTypeVolume}},
		},
		{
			Name:      "svc3",
			Container: api.ContainerSpec{Image: "test:latest", VolumeMounts: []api.VolumeMount{{VolumeName: "vol3", ContainerPath: "/data"}}},
			Volumes:   []api.VolumeSpec{{Name: "vol3", Type: api.VolumeTypeVolume}},
		},
	}

	state := &ClusterState{Machines: machines}
	scheduler, err := NewVolumeScheduler(state, serviceSpecs)
	require.NoError(t, err)

	result, err := scheduler.Schedule()
	require.NoError(t, err)

	// Each volume should be on a different machine (spreading).
	assert.Len(t, result, 3, "volumes should spread across all 3 machines")
	for machineID, vols := range result {
		assert.Len(t, vols, 1, "machine %s should have exactly 1 volume", machineID)
	}
}

func TestVolumeScheduler_ResourceBudget_GlobalServiceSingleReplica(t *testing.T) {
	// Global service runs 1 container per machine.
	// For volume placement, only 1 replica runs on the volume's machine.
	// Should count as 2 cores, not 2 × num_machines.
	machines := []*Machine{
		{
			Info: &pb.MachineInfo{
				Id:               "machine1",
				TotalCpuNanos:    4 * core, // Fits 1 replica (2 cores) + service2 (1 core)
				TotalMemoryBytes: 8 * gb,
			},
		},
		{
			Info: &pb.MachineInfo{
				Id:               "machine2",
				TotalCpuNanos:    4 * core,
				TotalMemoryBytes: 8 * gb,
			},
		},
	}
	serviceSpecs := []api.ServiceSpec{
		{
			Name: "global-svc",
			Mode: api.ServiceModeGlobal, // Runs on all machines
			Container: api.ContainerSpec{
				Image: "test:latest",
				Resources: api.ContainerResources{
					CPUReservation: 2 * core, // 2 cores per machine
				},
				VolumeMounts: []api.VolumeMount{
					{VolumeName: "shared-vol", ContainerPath: "/data"},
				},
			},
			Volumes: []api.VolumeSpec{
				{Name: "shared-vol", Type: api.VolumeTypeVolume},
			},
		},
		{
			Name: "service2",
			Container: api.ContainerSpec{
				Image: "test:latest",
				Resources: api.ContainerResources{
					CPUReservation: 1 * core,
				},
				VolumeMounts: []api.VolumeMount{
					{VolumeName: "shared-vol", ContainerPath: "/data"},
				},
			},
			Volumes: []api.VolumeSpec{
				{Name: "shared-vol", Type: api.VolumeTypeVolume},
			},
		},
	}

	state := &ClusterState{Machines: machines}
	scheduler, err := NewVolumeScheduler(state, serviceSpecs)
	require.NoError(t, err)

	// Combined need: 2 cores (global, 1 replica on volume machine) + 1 core = 3 cores
	// Both machines have 4 cores, so this should succeed.
	result, err := scheduler.Schedule()
	require.NoError(t, err)
	assert.Len(t, result, 1, "volume should be created on exactly one machine")
}

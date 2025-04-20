package scheduler

import (
	"testing"

	"github.com/docker/docker/api/types/volume"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/stretchr/testify/assert"
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
						VolumeMounts: []api.VolumeMount{
							{
								VolumeName:    "vol2",
								ContainerPath: "/data2",
							},
							{
								VolumeName:    "vol4",
								ContainerPath: "/data4",
							},
						},
					},
					Volumes: []api.VolumeSpec{
						{
							Name: "vol2",
							Type: api.VolumeTypeVolume,
						},
						{
							// TODO: use vol4-alias name and Docker name in VolumeOptions
							Name: "vol4",
							Type: api.VolumeTypeVolume,
						},
					},
				},
				{
					Name: "service4",
					Container: api.ContainerSpec{
						VolumeMounts: []api.VolumeMount{
							{
								VolumeName:    "vol2",
								ContainerPath: "/data2",
							},
							{
								VolumeName:    "vol5",
								ContainerPath: "/data5",
							},
						},
					},
					Volumes: []api.VolumeSpec{
						{
							Name: "vol2",
							Type: api.VolumeTypeVolume,
						},
						{
							Name: "vol5",
							Type: api.VolumeTypeVolume,
						},
					},
				},
			},
			want: map[string][]api.VolumeSpec{
				"machine2": {
					{
						Name: "vol5",
						Type: api.VolumeTypeVolume,
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
			scheduler, err := NewVolumeSchedulerWithMachines(tt.machines, tt.serviceSpecs)
			assert.NoError(t, err)
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

func TestVolumeScheduler_getAllVolumesAndSpecs(t *testing.T) {
	serviceSpecs := map[string]api.ServiceSpec{
		"service1": {
			Name: "service1",
			Container: api.ContainerSpec{
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
		"service2": {
			Name: "service2",
			Container: api.ContainerSpec{
				VolumeMounts: []api.VolumeMount{
					{
						VolumeName:    "vol2",
						ContainerPath: "/data",
					},
				},
			},
			Volumes: []api.VolumeSpec{
				{
					Name: "vol2",
					Type: api.VolumeTypeVolume,
				},
			},
		},
	}

	// Convert map to slice
	specsList := make([]api.ServiceSpec, 0, len(serviceSpecs))
	for _, spec := range serviceSpecs {
		specsList = append(specsList, spec)
	}
	scheduler, err := NewVolumeSchedulerWithMachines(nil, specsList)
	assert.NoError(t, err)
	volumes, specs := scheduler.getAllVolumesAndSpecs()

	assert.Len(t, volumes, 2)
	assert.Contains(t, volumes, "vol1")
	assert.Contains(t, volumes, "vol2")

	assert.Len(t, specs, 2)
	assert.Equal(t, api.VolumeSpec{Name: "vol1", Type: api.VolumeTypeVolume}, specs["vol1"])
	assert.Equal(t, api.VolumeSpec{Name: "vol2", Type: api.VolumeTypeVolume}, specs["vol2"])
}

func TestVolumeScheduler_getVolumeLocations(t *testing.T) {
	machines := []*Machine{
		{
			Info: &pb.MachineInfo{
				Id:   "machine1",
				Name: "machine1",
			},
			Volumes: []volume.Volume{
				{
					Name: "vol1",
				},
			},
		},
		{
			Info: &pb.MachineInfo{
				Id:   "machine2",
				Name: "machine2",
			},
			Volumes: []volume.Volume{
				{
					Name: "vol2",
				},
			},
		},
	}

	allVolumes := map[string]struct{}{
		"vol1": {},
		"vol2": {},
		"vol3": {},
	}

	scheduler, err := NewVolumeSchedulerWithMachines(machines, nil)
	assert.NoError(t, err)
	locations := scheduler.getVolumeLocations(allVolumes)

	assert.Len(t, locations, 3)
	assert.Equal(t, []string{"machine1"}, locations["vol1"])
	assert.Equal(t, []string{"machine2"}, locations["vol2"])
	assert.Empty(t, locations["vol3"])
}

func TestVolumeScheduler_getVolumeServices(t *testing.T) {
	serviceSpecs := map[string]api.ServiceSpec{
		"service1": {
			Name: "service1",
			Container: api.ContainerSpec{
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
		"service2": {
			Name: "service2",
			Container: api.ContainerSpec{
				VolumeMounts: []api.VolumeMount{
					{
						VolumeName:    "vol1",
						ContainerPath: "/data",
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
	}

	allVolumes := map[string]struct{}{
		"vol1": {},
		"vol2": {},
	}

	// Convert map to slice
	specsList := make([]api.ServiceSpec, 0, len(serviceSpecs))
	for _, spec := range serviceSpecs {
		specsList = append(specsList, spec)
	}
	scheduler, err := NewVolumeSchedulerWithMachines(nil, specsList)
	assert.NoError(t, err)
	services := scheduler.getVolumeServices(allVolumes)

	assert.Len(t, services, 2)
	assert.ElementsMatch(t, []string{"service1", "service2"}, services["vol1"])
	assert.ElementsMatch(t, []string{"service2"}, services["vol2"])
}

func TestVolumeScheduler_intersectMachines(t *testing.T) {
	scheduler := &VolumeScheduler{}

	a := []string{"machine1", "machine2", "machine3"}
	b := []string{"machine2", "machine3", "machine4"}

	result := scheduler.intersectMachines(a, b)
	assert.ElementsMatch(t, []string{"machine2", "machine3"}, result)

	// Empty intersection
	c := []string{"machine5", "machine6"}
	result = scheduler.intersectMachines(a, c)
	assert.Empty(t, result)

	// One empty slice
	result = scheduler.intersectMachines(a, []string{})
	assert.Empty(t, result)
	result = scheduler.intersectMachines([]string{}, b)
	assert.Empty(t, result)
}

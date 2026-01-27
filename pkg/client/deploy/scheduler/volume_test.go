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
		{
			name: "global service with missing volume schedules on all machines",
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
				},
				{
					Info: &pb.MachineInfo{
						Id: "machine3",
					},
				},
			},
			serviceSpecs: []api.ServiceSpec{
				{
					Name: "global-service",
					Mode: api.ServiceModeGlobal,
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
				"machine2": {
					{
						Name: "vol1",
						Type: api.VolumeTypeVolume,
					},
				},
				"machine3": {
					{
						Name: "vol1",
						Type: api.VolumeTypeVolume,
					},
				},
			},
		},
		{
			name: "global service with volume on some machines schedules remaining",
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
				},
				{
					Info: &pb.MachineInfo{
						Id: "machine3",
					},
				},
			},
			serviceSpecs: []api.ServiceSpec{
				{
					Name: "global-service",
					Mode: api.ServiceModeGlobal,
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
				"machine3": {
					{
						Name: "vol1",
						Type: api.VolumeTypeVolume,
					},
				},
			},
		},
		{
			name: "global service with placement constraint",
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
				},
				{
					Info: &pb.MachineInfo{
						Id: "machine3",
					},
				},
			},
			serviceSpecs: []api.ServiceSpec{
				{
					Name: "global-service",
					Mode: api.ServiceModeGlobal,
					Placement: api.Placement{
						Machines: []string{"machine1", "machine3"},
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
				"machine1": {
					{
						Name: "vol1",
						Type: api.VolumeTypeVolume,
					},
				},
				"machine3": {
					{
						Name: "vol1",
						Type: api.VolumeTypeVolume,
					},
				},
			},
		},
		{
			name: "volume shared between global and replicated fails",
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
				},
			},
			serviceSpecs: []api.ServiceSpec{
				{
					Name: "global-service",
					Mode: api.ServiceModeGlobal,
					Container: api.ContainerSpec{
						Image: "portainer/pause:latest",
						VolumeMounts: []api.VolumeMount{
							{
								VolumeName:    "shared-vol",
								ContainerPath: "/data",
							},
						},
					},
					Volumes: []api.VolumeSpec{
						{
							Name: "shared-vol",
							Type: api.VolumeTypeVolume,
						},
					},
				},
				{
					Name: "replicated-service",
					Mode: api.ServiceModeReplicated,
					Container: api.ContainerSpec{
						Image: "portainer/pause:latest",
						VolumeMounts: []api.VolumeMount{
							{
								VolumeName:    "shared-vol",
								ContainerPath: "/data",
							},
						},
					},
					Volumes: []api.VolumeSpec{
						{
							Name: "shared-vol",
							Type: api.VolumeTypeVolume,
						},
					},
				},
			},
			wantErr: "volume 'shared-vol' cannot be shared between global and replicated services",
		},
		{
			name: "multiple global services sharing same volume",
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
				},
			},
			serviceSpecs: []api.ServiceSpec{
				{
					Name: "global-service-1",
					Mode: api.ServiceModeGlobal,
					Container: api.ContainerSpec{
						Image: "portainer/pause:latest",
						VolumeMounts: []api.VolumeMount{
							{
								VolumeName:    "shared-vol",
								ContainerPath: "/data1",
							},
						},
					},
					Volumes: []api.VolumeSpec{
						{
							Name: "shared-vol",
							Type: api.VolumeTypeVolume,
						},
					},
				},
				{
					Name: "global-service-2",
					Mode: api.ServiceModeGlobal,
					Container: api.ContainerSpec{
						Image: "portainer/pause:latest",
						VolumeMounts: []api.VolumeMount{
							{
								VolumeName:    "shared-vol",
								ContainerPath: "/data2",
							},
						},
					},
					Volumes: []api.VolumeSpec{
						{
							Name: "shared-vol",
							Type: api.VolumeTypeVolume,
						},
					},
				},
			},
			want: map[string][]api.VolumeSpec{
				"machine1": {
					{
						Name: "shared-vol",
						Type: api.VolumeTypeVolume,
					},
				},
				"machine2": {
					{
						Name: "shared-vol",
						Type: api.VolumeTypeVolume,
					},
				},
			},
		},
		{
			name: "multiple global services sharing same volume with different placement constraints",
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
				},
				{
					Info: &pb.MachineInfo{
						Id: "machine3",
					},
				},
			},
			serviceSpecs: []api.ServiceSpec{
				{
					Name: "global-service-1",
					Mode: api.ServiceModeGlobal,
					Placement: api.Placement{
						Machines: []string{"machine1", "machine2"},
					},
					Container: api.ContainerSpec{
						Image: "portainer/pause:latest",
						VolumeMounts: []api.VolumeMount{
							{
								VolumeName:    "shared-vol",
								ContainerPath: "/data1",
							},
						},
					},
					Volumes: []api.VolumeSpec{
						{
							Name: "shared-vol",
							Type: api.VolumeTypeVolume,
						},
					},
				},
				{
					Name: "global-service-2",
					Mode: api.ServiceModeGlobal,
					Placement: api.Placement{
						Machines: []string{"machine2", "machine3"},
					},
					Container: api.ContainerSpec{
						Image: "portainer/pause:latest",
						VolumeMounts: []api.VolumeMount{
							{
								VolumeName:    "shared-vol",
								ContainerPath: "/data2",
							},
						},
					},
					Volumes: []api.VolumeSpec{
						{
							Name: "shared-vol",
							Type: api.VolumeTypeVolume,
						},
					},
				},
			},
			want: map[string][]api.VolumeSpec{
				"machine1": {
					{
						Name: "shared-vol",
						Type: api.VolumeTypeVolume,
					},
				},
				"machine2": {
					{
						Name: "shared-vol",
						Type: api.VolumeTypeVolume,
					},
				},
				"machine3": {
					{
						Name: "shared-vol",
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

package deploy

import (
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/stretchr/testify/assert"
)

func TestDetermineUpdateOrder(t *testing.T) {
	tests := []struct {
		name         string
		oldContainer api.ServiceContainer
		spec         api.ServiceSpec
		expected     string
	}{
		{
			name: "explicit stop-first order",
			oldContainer: api.ServiceContainer{
				Container: api.Container{
					InspectResponse: container.InspectResponse{
						Config: &container.Config{Labels: map[string]string{}},
					},
				},
			},
			spec: api.ServiceSpec{
				UpdateConfig: api.UpdateConfig{
					Order: api.UpdateOrderStopFirst,
				},
			},
			expected: api.UpdateOrderStopFirst,
		},
		{
			name: "explicit start-first order",
			oldContainer: api.ServiceContainer{
				Container: api.Container{
					InspectResponse: container.InspectResponse{
						Config: &container.Config{Labels: map[string]string{}},
					},
				},
			},
			spec: api.ServiceSpec{
				UpdateConfig: api.UpdateConfig{
					Order: api.UpdateOrderStartFirst,
				},
			},
			expected: api.UpdateOrderStartFirst,
		},
		{
			name: "explicit start-first overrides volume default",
			oldContainer: api.ServiceContainer{
				Container: api.Container{
					InspectResponse: container.InspectResponse{
						Config: &container.Config{Labels: map[string]string{}},
					},
				},
			},
			spec: api.ServiceSpec{
				UpdateConfig: api.UpdateConfig{
					Order: api.UpdateOrderStartFirst,
				},
				Volumes: []api.VolumeSpec{
					{
						Name: "data",
						Type: api.VolumeTypeVolume,
						VolumeOptions: &api.VolumeOptions{
							Name: "data",
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
			expected: api.UpdateOrderStartFirst,
		},
		{
			name: "single-replica service with volume defaults to stop-first",
			oldContainer: api.ServiceContainer{
				Container: api.Container{
					InspectResponse: container.InspectResponse{
						Config: &container.Config{Labels: map[string]string{}},
					},
				},
			},
			spec: api.ServiceSpec{
				Replicas: 1,
				Volumes: []api.VolumeSpec{
					{
						Name: "db-data",
						Type: api.VolumeTypeVolume,
						VolumeOptions: &api.VolumeOptions{
							Name: "db-data",
						},
					},
				},
				Container: api.ContainerSpec{
					VolumeMounts: []api.VolumeMount{
						{
							VolumeName:    "db-data",
							ContainerPath: "/var/lib/postgresql/data",
						},
					},
				},
			},
			expected: api.UpdateOrderStopFirst,
		},
		{
			name: "multi-replica service with volume defaults to start-first",
			oldContainer: api.ServiceContainer{
				Container: api.Container{
					InspectResponse: container.InspectResponse{
						Config: &container.Config{Labels: map[string]string{}},
					},
				},
			},
			spec: api.ServiceSpec{
				Replicas: 3,
				Volumes: []api.VolumeSpec{
					{
						Name: "app-data",
						Type: api.VolumeTypeVolume,
						VolumeOptions: &api.VolumeOptions{
							Name: "app-data",
						},
					},
				},
				Container: api.ContainerSpec{
					VolumeMounts: []api.VolumeMount{
						{
							VolumeName:    "app-data",
							ContainerPath: "/data",
						},
					},
				},
			},
			expected: api.UpdateOrderStartFirst,
		},
		{
			name: "service with bind mount defaults to start-first",
			oldContainer: api.ServiceContainer{
				Container: api.Container{
					InspectResponse: container.InspectResponse{
						Config: &container.Config{Labels: map[string]string{}},
					},
				},
			},
			spec: api.ServiceSpec{
				Volumes: []api.VolumeSpec{
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
							VolumeName:    "config",
							ContainerPath: "/config",
						},
					},
				},
			},
			expected: api.UpdateOrderStartFirst,
		},
		{
			name: "service with tmpfs mount defaults to start-first",
			oldContainer: api.ServiceContainer{
				Container: api.Container{
					InspectResponse: container.InspectResponse{
						Config: &container.Config{Labels: map[string]string{}},
					},
				},
			},
			spec: api.ServiceSpec{
				Volumes: []api.VolumeSpec{
					{
						Name: "tmp",
						Type: api.VolumeTypeTmpfs,
					},
				},
				Container: api.ContainerSpec{
					VolumeMounts: []api.VolumeMount{
						{
							VolumeName:    "tmp",
							ContainerPath: "/tmp",
						},
					},
				},
			},
			expected: api.UpdateOrderStartFirst,
		},
		{
			name: "stateless service defaults to start-first",
			oldContainer: api.ServiceContainer{
				Container: api.Container{
					InspectResponse: container.InspectResponse{
						Config: &container.Config{Labels: map[string]string{}},
					},
				},
			},
			spec: api.ServiceSpec{
				Container: api.ContainerSpec{
					Image: "nginx:latest",
				},
			},
			expected: api.UpdateOrderStartFirst,
		},
		{
			name: "port conflict forces stop-first",
			oldContainer: api.ServiceContainer{
				Container: api.Container{
					InspectResponse: container.InspectResponse{
						Config: &container.Config{
							Labels: map[string]string{
								api.LabelServicePorts: `[{"container_port":8080,"published_port":8080,"protocol":"tcp","mode":"host"}]`,
							},
						},
					},
				},
			},
			spec: api.ServiceSpec{
				Ports: []api.PortSpec{
					{
						ContainerPort: 8080,
						PublishedPort: 8080,
						Protocol:      "tcp",
						Mode:          api.PortModeHost,
					},
				},
			},
			expected: api.UpdateOrderStopFirst,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := determineUpdateOrder(tt.oldContainer, tt.spec)
			assert.Equal(t, tt.expected, result)
		})
	}
}

package deploy

import (
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client/deploy/operation"
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

func TestReconcileGlobalContainer(t *testing.T) {
	container1 := api.ServiceContainer{
		Container: api.Container{
			InspectResponse: container.InspectResponse{
				ContainerJSONBase: &container.ContainerJSONBase{
					ID:    "container-1",
					State: &container.State{Running: true},
				},
				Config: &container.Config{
					Labels: map[string]string{
						api.LabelServicePorts: "8080:8080/tcp@host",
					},
				},
			},
		},
	}
	container2WithPort9090 := api.ServiceContainer{
		Container: api.Container{
			InspectResponse: container.InspectResponse{
				ContainerJSONBase: &container.ContainerJSONBase{
					ID:    "container-2",
					State: &container.State{Running: true},
				},
				Config: &container.Config{
					Labels: map[string]string{
						api.LabelServicePorts: "9090:9090/tcp@host",
					},
				},
			},
		},
	}
	container2WithPort3000 := api.ServiceContainer{
		Container: api.Container{
			InspectResponse: container.InspectResponse{
				ContainerJSONBase: &container.ContainerJSONBase{
					ID:    "container-2",
					State: &container.State{Running: true},
				},
				Config: &container.Config{
					Labels: map[string]string{
						api.LabelServicePorts: "3000:3000/tcp@host",
					},
				},
			},
		},
	}

	tests := []struct {
		name          string
		containers    []api.MachineServiceContainer
		spec          api.ServiceSpec
		forceRecreate bool
		expectedOps   []operation.Operation
	}{
		{
			name:       "no containers creates new",
			containers: nil,
			spec: api.ServiceSpec{
				Container: api.ContainerSpec{Image: "nginx:latest"},
			},
			expectedOps: []operation.Operation{
				&operation.RunContainerOperation{
					ServiceID: "service-1",
					MachineID: "machine-1",
				},
			},
		},
		{
			name: "single running container with port conflict uses replace",
			containers: []api.MachineServiceContainer{
				{MachineID: "machine-1", Container: container1},
			},
			spec: api.ServiceSpec{
				Container: api.ContainerSpec{Image: "nginx:latest"},
				Ports: []api.PortSpec{
					{ContainerPort: 8080, PublishedPort: 8080, Protocol: "tcp", Mode: api.PortModeHost},
				},
			},
			expectedOps: []operation.Operation{
				&operation.ReplaceContainerOperation{
					ServiceID:    "service-1",
					MachineID:    "machine-1",
					OldContainer: container1,
					Order:        api.UpdateOrderStopFirst,
				},
			},
		},
		{
			name: "multiple running containers with different conflicting ports stops extras before replace",
			containers: []api.MachineServiceContainer{
				{MachineID: "machine-1", Container: container1},
				{MachineID: "machine-1", Container: container2WithPort9090},
			},
			spec: api.ServiceSpec{
				Container: api.ContainerSpec{Image: "nginx:latest"},
				Ports: []api.PortSpec{
					{ContainerPort: 8080, PublishedPort: 8080, Protocol: "tcp", Mode: api.PortModeHost},
					{ContainerPort: 9090, PublishedPort: 9090, Protocol: "tcp", Mode: api.PortModeHost},
				},
			},
			expectedOps: []operation.Operation{
				&operation.StopContainerOperation{
					ServiceID:   "service-1",
					ContainerID: "container-2",
					MachineID:   "machine-1",
				},
				&operation.ReplaceContainerOperation{
					ServiceID:    "service-1",
					MachineID:    "machine-1",
					OldContainer: container1,
					Order:        api.UpdateOrderStopFirst,
				},
				&operation.RemoveContainerOperation{
					MachineID: "machine-1",
					Container: container2WithPort9090,
				},
			},
		},
		{
			name: "multiple running containers but only one has port conflict",
			containers: []api.MachineServiceContainer{
				{MachineID: "machine-1", Container: container1},
				{MachineID: "machine-1", Container: container2WithPort3000},
			},
			spec: api.ServiceSpec{
				Container: api.ContainerSpec{Image: "nginx:latest"},
				Ports: []api.PortSpec{
					{ContainerPort: 8080, PublishedPort: 8080, Protocol: "tcp", Mode: api.PortModeHost},
				},
			},
			// Container-2 has no conflicting ports, so no StopContainerOperation for it.
			expectedOps: []operation.Operation{
				&operation.ReplaceContainerOperation{
					ServiceID:    "service-1",
					MachineID:    "machine-1",
					OldContainer: container1,
					Order:        api.UpdateOrderStopFirst,
				},
				&operation.RemoveContainerOperation{
					MachineID: "machine-1",
					Container: container2WithPort3000,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ops, err := reconcileGlobalContainer(tt.containers, tt.spec, "service-1", "machine-1", tt.forceRecreate)
			assert.NoError(t, err)
			assertOperationsEqual(t, tt.expectedOps, ops)
		})
	}
}

// assertOperationsEqual compares expected and actual operations, ignoring the Spec field
// which is passed separately to the function and not the focus of these tests.
func assertOperationsEqual(t *testing.T, expected, actual []operation.Operation) {
	t.Helper()
	opts := cmp.Options{
		cmpopts.IgnoreFields(operation.RunContainerOperation{}, "Spec"),
		cmpopts.IgnoreFields(operation.ReplaceContainerOperation{}, "Spec"),
		cmpopts.IgnoreUnexported(api.Container{}),
	}
	if diff := cmp.Diff(expected, actual, opts); diff != "" {
		t.Errorf("operations mismatch (-expected +actual):\n%s", diff)
	}
}

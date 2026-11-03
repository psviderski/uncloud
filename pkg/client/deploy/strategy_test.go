package deploy

import (
	"slices"
	"strings"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/psviderski/uncloud/api/pb"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client/deploy/scheduler"
	"github.com/psviderski/uncloud/pkg/client/operation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
					ServiceID:   "service-1",
					MachineID:   "machine-1",
					MachineName: "machine-1",
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
					MachineName:  "machine-1",
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
					ServiceName: container2WithPort9090.ServiceSpec.Name,
					ContainerID: "container-2",
					MachineID:   "machine-1",
					MachineName: "machine-1",
				},
				&operation.ReplaceContainerOperation{
					ServiceID:    "service-1",
					MachineID:    "machine-1",
					MachineName:  "machine-1",
					OldContainer: container1,
					Order:        api.UpdateOrderStopFirst,
				},
				&operation.RemoveContainerOperation{
					MachineID:   "machine-1",
					MachineName: "machine-1",
					Container:   container2WithPort9090,
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
					MachineName:  "machine-1",
					OldContainer: container1,
					Order:        api.UpdateOrderStopFirst,
				},
				&operation.RemoveContainerOperation{
					MachineID:   "machine-1",
					MachineName: "machine-1",
					Container:   container2WithPort3000,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ops, err := reconcileGlobalContainer(
				tt.containers,
				tt.spec,
				"service-1",
				&pb.MachineInfo{Id: "machine-1", Name: "machine-1"},
				tt.forceRecreate,
				false,
			)
			assert.NoError(t, err)
			assertOperationsEqual(t, tt.expectedOps, ops)
		})
	}
}

func TestRollingStrategy_Plan_UpdateInPlace(t *testing.T) {
	t.Parallel()

	state := &scheduler.ClusterState{
		Machines: []*scheduler.Machine{
			{Info: &pb.MachineInfo{Id: "m-1", Name: "machine-1"}},
			{Info: &pb.MachineInfo{Id: "m-2", Name: "machine-2"}},
		},
	}
	newContainer := func(id string, spec api.ServiceSpec) api.ServiceContainer {
		ctr := newServiceContainer(id, container.State{Running: true, Status: "running"})
		ctr.ServiceSpec = spec
		return ctr
	}

	tests := []struct {
		name   string
		change func(spec *api.ServiceSpec)
		// options are the expected in-place update options. Nil means the containers must be recreated.
		options *api.UpdateContainerOptions
	}{
		{
			name:    "service labels",
			change:  func(spec *api.ServiceSpec) { spec.Labels = map[string]string{"tier": "backend"} },
			options: &api.UpdateContainerOptions{ServiceLabels: &map[string]string{"tier": "backend"}},
		},
		{
			name: "CPU and memory limits",
			change: func(spec *api.ServiceSpec) {
				spec.Container.Resources.CPU = 2 * api.Core
				spec.Container.Resources.Memory = 512 * 1024 * 1024
			},
			options: &api.UpdateContainerOptions{
				CPU:    new(int64(2 * api.Core)),
				Memory: new(int64(512 * 1024 * 1024)),
			},
		},
		{
			name:   "remove memory limit",
			change: func(spec *api.ServiceSpec) { spec.Container.Resources.Memory = 0 },
		},
	}

	for _, tt := range tests {
		for _, mode := range []string{api.ServiceModeReplicated, api.ServiceModeGlobal} {
			t.Run(tt.name+"/"+mode, func(t *testing.T) {
				currentSpec := api.ServiceSpec{
					Name: "app",
					Mode: mode,
					Container: api.ContainerSpec{
						Image: "nginx:latest",
						Resources: api.ContainerResources{
							CPU:    api.Core,
							Memory: 256 * 1024 * 1024,
						},
					},
					Labels:    map[string]string{"tier": "frontend"},
					PreDeploy: &api.PreDeployHook{Command: []string{"db", "migrate"}},
				}
				if mode == api.ServiceModeReplicated {
					currentSpec.Replicas = 2
				}
				newSpec := currentSpec.Clone()
				tt.change(&newSpec)

				ctr1 := newContainer("container-1", currentSpec)
				ctr2 := newContainer("container-2", currentSpec)
				svc := &api.Service{
					ID:   "svc-1",
					Name: "app",
					Mode: mode,
					Containers: []api.MachineServiceContainer{
						{MachineID: "m-1", MachineName: "machine-1", Container: ctr1},
						{MachineID: "m-2", MachineName: "machine-2", Container: ctr2},
					},
				}

				plan, err := (&RollingStrategy{}).Plan(state, svc, newSpec)
				require.NoError(t, err)

				if tt.options == nil {
					require.NotEmpty(t, plan.Operations)
					for _, op := range plan.Operations {
						_, isUpdate := op.(*operation.UpdateContainerOperation)
						assert.False(t, isUpdate, "Expected containers to be recreated, got %s", op)
					}
					return
				}
				// The replicated strategy shuffles machines so sort the operations to compare them deterministically.
				ops := slices.Clone(plan.Operations)
				slices.SortFunc(ops, func(a, b operation.Operation) int {
					return strings.Compare(a.String(), b.String())
				})
				// The containers are updated in place and the pre-deploy hook is not run.
				assertOperationsEqual(t, []operation.Operation{
					&operation.UpdateContainerOperation{
						ServiceID:   "svc-1",
						MachineID:   "m-1",
						MachineName: "machine-1",
						Container:   ctr1,
						Options:     *tt.options,
					},
					&operation.UpdateContainerOperation{
						ServiceID:   "svc-1",
						MachineID:   "m-2",
						MachineName: "machine-2",
						Container:   ctr2,
						Options:     *tt.options,
					},
				}, ops)
			})
		}
	}
}

func TestPreDeployOperations(t *testing.T) {
	hook := &api.PreDeployHook{
		Command: []string{"db", "migrate"},
	}

	strategy := &RollingStrategy{
		state: &scheduler.ClusterState{
			Machines: []*scheduler.Machine{
				{Info: &pb.MachineInfo{Id: "m-1", Name: "machine-1"}},
				{Info: &pb.MachineInfo{Id: "m-2", Name: "machine-2"}},
				{Info: &pb.MachineInfo{Id: "m-3", Name: "machine-3"}},
			},
		},
	}

	runningHook1 := newServiceContainer("running-hook-1", container.State{Running: true, Status: "running"})
	runningHook2 := newServiceContainer("running-hook-2", container.State{Running: true, Status: "running"})

	tests := []struct {
		name     string
		plan     ServicePlan
		svc      *api.Service
		expected []operation.Operation
	}{
		{
			name: "no pre-deploy hook in spec",
			plan: ServicePlan{
				ServiceID:   "svc-1",
				ServiceName: "app",
				Spec:        api.ServiceSpec{},
				SequenceOperation: operation.SequenceOperation{
					Operations: []operation.Operation{
						&operation.RunContainerOperation{MachineID: "m-1", MachineName: "machine-1"},
					},
				},
			},
			expected: nil,
		},
		{
			name: "plan has RunContainerOperation",
			plan: ServicePlan{
				ServiceID:   "svc-1",
				ServiceName: "app",
				Spec:        api.ServiceSpec{PreDeploy: hook},
				SequenceOperation: operation.SequenceOperation{
					Operations: []operation.Operation{
						&operation.RunContainerOperation{MachineID: "m-1", MachineName: "machine-1"},
					},
				},
			},
			expected: []operation.Operation{
				&operation.RunPreDeployOperation{
					ServiceID:   "svc-1",
					MachineID:   "m-1",
					MachineName: "machine-1",
				},
			},
		},
		{
			name: "plan has ReplaceContainerOperation",
			plan: ServicePlan{
				ServiceID:   "svc-1",
				ServiceName: "app",
				Spec:        api.ServiceSpec{PreDeploy: hook},
				SequenceOperation: operation.SequenceOperation{
					Operations: []operation.Operation{
						&operation.ReplaceContainerOperation{MachineID: "m-2", MachineName: "machine-2"},
					},
				},
			},
			expected: []operation.Operation{
				&operation.RunPreDeployOperation{
					ServiceID:   "svc-1",
					MachineID:   "m-2",
					MachineName: "machine-2",
				},
			},
		},
		{
			name: "plan has only UpdateContainerOperation",
			plan: ServicePlan{
				ServiceID:   "svc-1",
				ServiceName: "app",
				Spec:        api.ServiceSpec{PreDeploy: hook},
				SequenceOperation: operation.SequenceOperation{
					Operations: []operation.Operation{
						&operation.UpdateContainerOperation{MachineID: "m-1", MachineName: "machine-1"},
					},
				},
			},
			expected: nil,
		},
		{
			name: "plan has only RemoveContainerOperation",
			plan: ServicePlan{
				ServiceID:   "svc-1",
				ServiceName: "app",
				Spec:        api.ServiceSpec{PreDeploy: hook},
				SequenceOperation: operation.SequenceOperation{
					Operations: []operation.Operation{
						&operation.RemoveContainerOperation{MachineID: "m-1", MachineName: "machine-1"},
					},
				},
			},
			expected: nil,
		},
		{
			name: "plan has only StopContainerOperation",
			plan: ServicePlan{
				ServiceID:   "svc-1",
				ServiceName: "app",
				Spec:        api.ServiceSpec{PreDeploy: hook},
				SequenceOperation: operation.SequenceOperation{
					Operations: []operation.Operation{
						&operation.StopContainerOperation{MachineID: "m-1", MachineName: "machine-1"},
					},
				},
			},
			expected: nil,
		},
		{
			name: "empty plan with no operations",
			plan: ServicePlan{
				ServiceID:   "svc-1",
				ServiceName: "app",
				Spec:        api.ServiceSpec{PreDeploy: hook},
			},
			expected: nil,
		},
		{
			name: "stopped hook containers are collected for cleanup",
			plan: ServicePlan{
				ServiceID:   "svc-1",
				ServiceName: "app",
				Spec:        api.ServiceSpec{PreDeploy: hook},
				SequenceOperation: operation.SequenceOperation{
					Operations: []operation.Operation{
						&operation.RunContainerOperation{MachineID: "m-1", MachineName: "machine-1"},
					},
				},
			},
			svc: &api.Service{
				HookContainers: []api.MachineServiceContainer{
					{MachineID: "m-1", Container: newServiceContainer("old-hook-1", container.State{Status: "exited"})},
				},
			},
			expected: []operation.Operation{
				&operation.RunPreDeployOperation{
					ServiceID:       "svc-1",
					MachineID:       "m-1",
					MachineName:     "machine-1",
					OldContainerIDs: []string{"old-hook-1"},
				},
			},
		},
		{
			name: "running hook containers are stopped before run",
			plan: ServicePlan{
				ServiceID:   "svc-1",
				ServiceName: "app",
				Spec:        api.ServiceSpec{PreDeploy: hook},
				SequenceOperation: operation.SequenceOperation{
					Operations: []operation.Operation{
						&operation.RunContainerOperation{MachineID: "m-1", MachineName: "machine-1"},
					},
				},
			},
			svc: &api.Service{
				HookContainers: []api.MachineServiceContainer{
					{MachineID: "m-2", Container: runningHook1},
				},
			},
			expected: []operation.Operation{
				&operation.StopPreDeployOperation{
					MachineID:   "m-2",
					MachineName: "machine-2",
					Container:   runningHook1,
				},
				&operation.RunPreDeployOperation{
					ServiceID:       "svc-1",
					MachineID:       "m-1",
					MachineName:     "machine-1",
					OldContainerIDs: []string{"running-hook-1"},
				},
			},
		},
		{
			name: "mixed stopped and running hook containers and mixed container operations",
			plan: ServicePlan{
				ServiceID:   "svc-1",
				ServiceName: "app",
				Spec:        api.ServiceSpec{PreDeploy: hook},
				SequenceOperation: operation.SequenceOperation{
					Operations: []operation.Operation{
						&operation.StopContainerOperation{MachineID: "m-1", MachineName: "machine-1"},
						&operation.RemoveContainerOperation{MachineID: "m-1", MachineName: "machine-1"},
						&operation.ReplaceContainerOperation{MachineID: "m-2", MachineName: "machine-2"},
						&operation.RunContainerOperation{MachineID: "m-3", MachineName: "machine-3"},
						&operation.RemoveContainerOperation{MachineID: "m-3", MachineName: "machine-3"},
					},
				},
			},
			svc: &api.Service{
				HookContainers: []api.MachineServiceContainer{
					{MachineID: "m-1", Container: newServiceContainer("stopped-hook",
						container.State{Status: "exited"})},
					{MachineID: "m-1", Container: runningHook1},
					{MachineID: "m-3", Container: runningHook2},
				},
			},
			expected: []operation.Operation{
				&operation.StopPreDeployOperation{
					MachineID:   "m-1",
					MachineName: "machine-1",
					Container:   runningHook1,
				},
				&operation.StopPreDeployOperation{
					MachineID:   "m-3",
					MachineName: "machine-3",
					Container:   runningHook2,
				},
				&operation.RunPreDeployOperation{
					ServiceID:       "svc-1",
					MachineID:       "m-2",
					MachineName:     "machine-2",
					OldContainerIDs: []string{"stopped-hook", "running-hook-1", "running-hook-2"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := strategy.preDeployOperations(tt.svc, tt.plan)
			if tt.expected == nil {
				assert.Nil(t, result)
				return
			}
			assertOperationsEqual(t, tt.expected, result)
		})
	}
}

// newServiceContainer creates an api.ServiceContainer with the given ID and state.
func newServiceContainer(id string, state container.State) api.ServiceContainer {
	return api.ServiceContainer{Container: api.Container{
		InspectResponse: container.InspectResponse{
			ContainerJSONBase: &container.ContainerJSONBase{
				ID:    id,
				State: &state,
			},
			Config: &container.Config{},
		},
	}}
}

// assertOperationsEqual compares expected and actual operations, ignoring the Spec field
// which is passed separately to the function and not the focus of these tests.
func assertOperationsEqual(t *testing.T, expected, actual []operation.Operation) {
	t.Helper()
	opts := cmp.Options{
		cmpopts.IgnoreFields(operation.RunContainerOperation{}, "Spec"),
		cmpopts.IgnoreFields(operation.ReplaceContainerOperation{}, "Spec"),
		cmpopts.IgnoreFields(operation.RunPreDeployOperation{}, "Spec"),
		cmpopts.IgnoreUnexported(api.Container{}),
	}
	if diff := cmp.Diff(expected, actual, opts); diff != "" {
		t.Errorf("operations mismatch (-expected +actual):\n%s", diff)
	}
}

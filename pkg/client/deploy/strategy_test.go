package deploy

import (
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client/deploy/scheduler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper functions to create test data.

func newMachine(id, name string) *scheduler.Machine {
	return &scheduler.Machine{
		Info: &pb.MachineInfo{
			Id:   id,
			Name: name,
		},
	}
}

func newClusterState(machines ...*scheduler.Machine) *scheduler.ClusterState {
	return &scheduler.ClusterState{
		Machines: machines,
	}
}

func newRunningContainer(id string, spec api.ServiceSpec) api.ServiceContainer {
	return api.ServiceContainer{
		Container: api.Container{
			InspectResponse: container.InspectResponse{
				ContainerJSONBase: &container.ContainerJSONBase{
					ID: id,
					State: &container.State{
						Running: true,
					},
				},
				Config: &container.Config{
					Labels: map[string]string{},
				},
			},
		},
		ServiceSpec: spec,
	}
}

func newStoppedContainer(id string, spec api.ServiceSpec) api.ServiceContainer {
	return api.ServiceContainer{
		Container: api.Container{
			InspectResponse: container.InspectResponse{
				ContainerJSONBase: &container.ContainerJSONBase{
					ID: id,
					State: &container.State{
						Running: false,
					},
				},
				Config: &container.Config{
					Labels: map[string]string{},
				},
			},
		},
		ServiceSpec: spec,
	}
}

func newService(id, name string, containers []api.MachineServiceContainer) *api.Service {
	return &api.Service{
		ID:         id,
		Name:       name,
		Containers: containers,
	}
}

func baseSpec(name string) api.ServiceSpec {
	return api.ServiceSpec{
		Name: name,
		Mode: api.ServiceModeReplicated,
		Container: api.ContainerSpec{
			Image: "nginx:latest",
		},
		Replicas: 1,
	}
}

func globalSpec(name string) api.ServiceSpec {
	return api.ServiceSpec{
		Name: name,
		Mode: api.ServiceModeGlobal,
		Container: api.ContainerSpec{
			Image: "nginx:latest",
		},
	}
}

func globalSpecWithDeployLabels(name string, labels map[string]string) api.ServiceSpec {
	s := globalSpec(name)
	s.DeployLabels = labels
	return s
}

func globalSpecWithLabels(name string, labels map[string]string) api.ServiceSpec {
	s := globalSpec(name)
	s.Labels = labels
	return s
}

// getOperationsOfType returns all operations of a specific type.
func getOperationsOfType[T Operation](ops []Operation) []T {
	var result []T
	for _, op := range ops {
		if typed, ok := op.(T); ok {
			result = append(result, typed)
		}
	}
	return result
}

// TestRollingStrategy_Replicated_UpToDate tests that no operations are generated
// when containers are already up-to-date.
func TestRollingStrategy_Replicated_UpToDate(t *testing.T) {
	t.Parallel()

	machine := newMachine("m1", "machine-1")
	state := newClusterState(machine)

	spec := baseSpec("test-service")
	spec.Replicas = 1

	svc := newService("svc-1", "test-service", []api.MachineServiceContainer{
		{
			MachineID: "m1",
			Container: newRunningContainer("c1", spec),
		},
	})

	strategy := &RollingStrategy{}
	plan, err := strategy.Plan(state, svc, spec)

	require.NoError(t, err)
	assert.Empty(t, plan.Operations, "expected no operations when container is up-to-date")
}

// TestRollingStrategy_Replicated_NeedsSpecUpdate tests that only UpdateSpecOperation
// is generated when deploy labels change (no container recreation).
func TestRollingStrategy_Replicated_NeedsSpecUpdate(t *testing.T) {
	t.Parallel()

	machine := newMachine("m1", "machine-1")
	state := newClusterState(machine)

	currentSpec := baseSpec("test-service")
	currentSpec.DeployLabels = map[string]string{"version": "v1"}

	newSpec := baseSpec("test-service")
	newSpec.DeployLabels = map[string]string{"version": "v2"}

	svc := newService("svc-1", "test-service", []api.MachineServiceContainer{
		{
			MachineID: "m1",
			Container: newRunningContainer("c1", currentSpec),
		},
	})

	strategy := &RollingStrategy{}
	plan, err := strategy.Plan(state, svc, newSpec)

	require.NoError(t, err)
	require.Len(t, plan.Operations, 1, "expected exactly one operation")

	updateOp, ok := plan.Operations[0].(*UpdateSpecOperation)
	require.True(t, ok, "expected UpdateSpecOperation, got %T", plan.Operations[0])
	assert.Equal(t, "m1", updateOp.MachineID)
	assert.Equal(t, "c1", updateOp.ContainerID)
	assert.Equal(t, newSpec, updateOp.NewSpec)
}

// TestRollingStrategy_Replicated_NeedsRecreate tests that containers are recreated
// when immutable fields change (like Labels).
func TestRollingStrategy_Replicated_NeedsRecreate(t *testing.T) {
	t.Parallel()

	machine := newMachine("m1", "machine-1")
	state := newClusterState(machine)

	currentSpec := baseSpec("test-service")
	currentSpec.Labels = map[string]string{"app": "old"}

	newSpec := baseSpec("test-service")
	newSpec.Labels = map[string]string{"app": "new"}

	svc := newService("svc-1", "test-service", []api.MachineServiceContainer{
		{
			MachineID: "m1",
			Container: newRunningContainer("c1", currentSpec),
		},
	})

	strategy := &RollingStrategy{}
	plan, err := strategy.Plan(state, svc, newSpec)

	require.NoError(t, err)
	assert.Len(t, plan.Operations, 2, "expected run + remove operations")

	// Should have one RunContainerOperation and one RemoveContainerOperation.
	runOps := getOperationsOfType[*RunContainerOperation](plan.Operations)
	removeOps := getOperationsOfType[*RemoveContainerOperation](plan.Operations)

	assert.Len(t, runOps, 1, "expected one RunContainerOperation")
	assert.Len(t, removeOps, 1, "expected one RemoveContainerOperation")
}

// TestRollingStrategy_Replicated_ScaleUp tests scaling up adds new containers.
func TestRollingStrategy_Replicated_ScaleUp(t *testing.T) {
	t.Parallel()

	machines := []*scheduler.Machine{
		newMachine("m1", "machine-1"),
		newMachine("m2", "machine-2"),
	}
	state := newClusterState(machines...)

	spec := baseSpec("test-service")
	spec.Replicas = 1

	newSpec := baseSpec("test-service")
	newSpec.Replicas = 3

	svc := newService("svc-1", "test-service", []api.MachineServiceContainer{
		{
			MachineID: "m1",
			Container: newRunningContainer("c1", spec),
		},
	})

	strategy := &RollingStrategy{}
	plan, err := strategy.Plan(state, svc, newSpec)

	require.NoError(t, err)

	runOps := getOperationsOfType[*RunContainerOperation](plan.Operations)
	assert.Len(t, runOps, 2, "expected 2 new containers to be created")
}

// TestRollingStrategy_Replicated_ScaleDown tests scaling down removes excess containers.
func TestRollingStrategy_Replicated_ScaleDown(t *testing.T) {
	t.Parallel()

	machine := newMachine("m1", "machine-1")
	state := newClusterState(machine)

	spec := baseSpec("test-service")
	spec.Replicas = 3

	newSpec := baseSpec("test-service")
	newSpec.Replicas = 1

	svc := newService("svc-1", "test-service", []api.MachineServiceContainer{
		{MachineID: "m1", Container: newRunningContainer("c1", spec)},
		{MachineID: "m1", Container: newRunningContainer("c2", spec)},
		{MachineID: "m1", Container: newRunningContainer("c3", spec)},
	})

	strategy := &RollingStrategy{}
	plan, err := strategy.Plan(state, svc, newSpec)

	require.NoError(t, err)

	removeOps := getOperationsOfType[*RemoveContainerOperation](plan.Operations)
	assert.Len(t, removeOps, 2, "expected 2 containers to be removed")
}

// TestRollingStrategy_Replicated_RecreatePreferredOverSpecUpdate tests that when both
// Labels and DeployLabels change, recreate takes precedence.
func TestRollingStrategy_Replicated_RecreatePreferredOverSpecUpdate(t *testing.T) {
	t.Parallel()

	machine := newMachine("m1", "machine-1")
	state := newClusterState(machine)

	currentSpec := baseSpec("test-service")
	currentSpec.Labels = map[string]string{"app": "old"}
	currentSpec.DeployLabels = map[string]string{"version": "v1"}

	newSpec := baseSpec("test-service")
	newSpec.Labels = map[string]string{"app": "new"}
	newSpec.DeployLabels = map[string]string{"version": "v2"}

	svc := newService("svc-1", "test-service", []api.MachineServiceContainer{
		{
			MachineID: "m1",
			Container: newRunningContainer("c1", currentSpec),
		},
	})

	strategy := &RollingStrategy{}
	plan, err := strategy.Plan(state, svc, newSpec)

	require.NoError(t, err)

	// Should recreate, not just update spec.
	updateOps := getOperationsOfType[*UpdateSpecOperation](plan.Operations)
	assert.Empty(t, updateOps, "should not have UpdateSpecOperation when Labels change")

	runOps := getOperationsOfType[*RunContainerOperation](plan.Operations)
	removeOps := getOperationsOfType[*RemoveContainerOperation](plan.Operations)
	assert.Len(t, runOps, 1, "expected one RunContainerOperation")
	assert.Len(t, removeOps, 1, "expected one RemoveContainerOperation")
}

// TestRollingStrategy_Replicated_ForceRecreate tests that ForceRecreate flag
// causes recreation even when spec hasn't changed.
func TestRollingStrategy_Replicated_ForceRecreate(t *testing.T) {
	t.Parallel()

	machine := newMachine("m1", "machine-1")
	state := newClusterState(machine)

	spec := baseSpec("test-service")

	svc := newService("svc-1", "test-service", []api.MachineServiceContainer{
		{
			MachineID: "m1",
			Container: newRunningContainer("c1", spec),
		},
	})

	strategy := &RollingStrategy{ForceRecreate: true}
	plan, err := strategy.Plan(state, svc, spec)

	require.NoError(t, err)

	runOps := getOperationsOfType[*RunContainerOperation](plan.Operations)
	removeOps := getOperationsOfType[*RemoveContainerOperation](plan.Operations)
	assert.Len(t, runOps, 1, "expected container to be recreated")
	assert.Len(t, removeOps, 1, "expected old container to be removed")
}

// TestReconcileGlobalContainer tests the reconcileGlobalContainer function directly.
func TestReconcileGlobalContainer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		containers    []api.MachineServiceContainer
		newSpec       api.ServiceSpec
		forceRecreate bool
		wantRunOps    int
		wantRemoveOps int
		wantUpdateOps int
		wantStopOps   int
	}{
		{
			name:          "no containers - create new",
			containers:    nil,
			newSpec:       globalSpec("test"),
			wantRunOps:    1,
			wantRemoveOps: 0,
			wantUpdateOps: 0,
		},
		{
			name: "container up-to-date - no ops",
			containers: []api.MachineServiceContainer{
				{MachineID: "m1", Container: newRunningContainer("c1", globalSpec("test"))},
			},
			newSpec:       globalSpec("test"),
			wantRunOps:    0,
			wantRemoveOps: 0,
			wantUpdateOps: 0,
		},
		{
			name: "container needs spec update only",
			containers: []api.MachineServiceContainer{
				{MachineID: "m1", Container: newRunningContainer("c1", globalSpecWithDeployLabels("test", map[string]string{"version": "v1"}))},
			},
			newSpec:       globalSpecWithDeployLabels("test", map[string]string{"version": "v2"}),
			wantRunOps:    0,
			wantRemoveOps: 0,
			wantUpdateOps: 1,
		},
		{
			name: "container needs recreate - labels changed",
			containers: []api.MachineServiceContainer{
				{MachineID: "m1", Container: newRunningContainer("c1", globalSpecWithLabels("test", map[string]string{"app": "old"}))},
			},
			newSpec:       globalSpecWithLabels("test", map[string]string{"app": "new"}),
			wantRunOps:    1,
			wantRemoveOps: 1,
			wantUpdateOps: 0,
		},
		{
			name: "multiple containers - one up-to-date - remove extras",
			containers: []api.MachineServiceContainer{
				{MachineID: "m1", Container: newRunningContainer("c1", globalSpec("test"))},
				{MachineID: "m1", Container: newRunningContainer("c2", globalSpec("test"))},
				{MachineID: "m1", Container: newRunningContainer("c3", globalSpec("test"))},
			},
			newSpec:       globalSpec("test"),
			wantRunOps:    0,
			wantRemoveOps: 2, // Remove the extras.
			wantUpdateOps: 0,
		},
		{
			name: "multiple containers - one needs spec update - update and remove extras",
			containers: []api.MachineServiceContainer{
				{MachineID: "m1", Container: newRunningContainer("c1", globalSpecWithDeployLabels("test", map[string]string{"version": "v1"}))},
				{MachineID: "m1", Container: newRunningContainer("c2", globalSpecWithDeployLabels("test", map[string]string{"version": "v1"}))},
			},
			newSpec:       globalSpecWithDeployLabels("test", map[string]string{"version": "v2"}),
			wantRunOps:    0,
			wantRemoveOps: 1, // Remove the extra.
			wantUpdateOps: 1, // Update the first one.
		},
		{
			name: "multiple containers - none match - recreate",
			containers: []api.MachineServiceContainer{
				{MachineID: "m1", Container: newRunningContainer("c1", globalSpecWithLabels("test", map[string]string{"app": "old"}))},
				{MachineID: "m1", Container: newRunningContainer("c2", globalSpecWithLabels("test", map[string]string{"app": "old"}))},
			},
			newSpec:       globalSpecWithLabels("test", map[string]string{"app": "new"}),
			wantRunOps:    1,
			wantRemoveOps: 2,
			wantUpdateOps: 0,
		},
		{
			name: "stopped container ignored - create new",
			containers: []api.MachineServiceContainer{
				{MachineID: "m1", Container: newStoppedContainer("c1", globalSpec("test"))},
			},
			newSpec:       globalSpec("test"),
			wantRunOps:    1,
			wantRemoveOps: 1, // Remove the stopped one.
			wantUpdateOps: 0,
		},
		{
			name: "force recreate - recreate even if up-to-date",
			containers: []api.MachineServiceContainer{
				{MachineID: "m1", Container: newRunningContainer("c1", globalSpec("test"))},
			},
			newSpec:       globalSpec("test"),
			forceRecreate: true,
			wantRunOps:    1,
			wantRemoveOps: 1,
			wantUpdateOps: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ops, err := reconcileGlobalContainer(
				tt.containers,
				tt.newSpec,
				"svc-1",
				"m1",
				tt.forceRecreate,
			)

			require.NoError(t, err)

			runOps := getOperationsOfType[*RunContainerOperation](ops)
			removeOps := getOperationsOfType[*RemoveContainerOperation](ops)
			updateOps := getOperationsOfType[*UpdateSpecOperation](ops)
			stopOps := getOperationsOfType[*StopContainerOperation](ops)

			assert.Len(t, runOps, tt.wantRunOps, "RunContainerOperation count mismatch")
			assert.Len(t, removeOps, tt.wantRemoveOps, "RemoveContainerOperation count mismatch")
			assert.Len(t, updateOps, tt.wantUpdateOps, "UpdateSpecOperation count mismatch")
			assert.Len(t, stopOps, tt.wantStopOps, "StopContainerOperation count mismatch")
		})
	}
}

// TestRollingStrategy_Global_NeedsSpecUpdate tests the full Plan flow for global services
// with spec updates.
func TestRollingStrategy_Global_NeedsSpecUpdate(t *testing.T) {
	t.Parallel()

	machines := []*scheduler.Machine{
		newMachine("m1", "machine-1"),
		newMachine("m2", "machine-2"),
	}
	state := newClusterState(machines...)

	currentSpec := globalSpec("test-service")
	currentSpec.DeployLabels = map[string]string{"version": "v1"}

	newSpec := globalSpec("test-service")
	newSpec.DeployLabels = map[string]string{"version": "v2"}

	svc := newService("svc-1", "test-service", []api.MachineServiceContainer{
		{MachineID: "m1", Container: newRunningContainer("c1", currentSpec)},
		{MachineID: "m2", Container: newRunningContainer("c2", currentSpec)},
	})

	strategy := &RollingStrategy{}
	plan, err := strategy.Plan(state, svc, newSpec)

	require.NoError(t, err)

	updateOps := getOperationsOfType[*UpdateSpecOperation](plan.Operations)
	assert.Len(t, updateOps, 2, "expected UpdateSpecOperation for each machine")

	// Verify no recreate operations.
	runOps := getOperationsOfType[*RunContainerOperation](plan.Operations)
	removeOps := getOperationsOfType[*RemoveContainerOperation](plan.Operations)
	assert.Empty(t, runOps, "should not have RunContainerOperation for spec-only update")
	assert.Empty(t, removeOps, "should not have RemoveContainerOperation for spec-only update")
}

// TestRollingStrategy_NewService tests deploying a brand new service.
func TestRollingStrategy_NewService(t *testing.T) {
	t.Parallel()

	machines := []*scheduler.Machine{
		newMachine("m1", "machine-1"),
		newMachine("m2", "machine-2"),
	}
	state := newClusterState(machines...)

	spec := baseSpec("test-service")
	spec.Replicas = 2

	strategy := &RollingStrategy{}
	plan, err := strategy.Plan(state, nil, spec) // nil service = new deployment

	require.NoError(t, err)
	assert.NotEmpty(t, plan.ServiceID, "should generate new service ID")

	runOps := getOperationsOfType[*RunContainerOperation](plan.Operations)
	assert.Len(t, runOps, 2, "expected 2 containers for new service")
}

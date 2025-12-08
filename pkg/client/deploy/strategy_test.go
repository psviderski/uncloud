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

const (
	core = int64(1e9)
	gb   = int64(1024 * 1024 * 1024)
)

// newTestClusterState creates a ClusterState with the given machines for testing.
func newTestClusterState(machines ...*scheduler.Machine) *scheduler.ClusterState {
	return &scheduler.ClusterState{Machines: machines}
}

// newTestMachine creates a Machine with the given parameters for testing.
func newTestMachine(id, name string, cpuCores, memoryGB int64) *scheduler.Machine {
	return &scheduler.Machine{
		Info: &pb.MachineInfo{
			Id:               id,
			Name:             name,
			TotalCpuNanos:    cpuCores * core,
			TotalMemoryBytes: memoryGB * gb,
		},
	}
}

// countOperationsByType counts operations by their type.
func countOperationsByType(ops []Operation) map[string]int {
	counts := make(map[string]int)
	for _, op := range ops {
		switch op.(type) {
		case *RunContainerOperation:
			counts["run"]++
		case *StopContainerOperation:
			counts["stop"]++
		case *RemoveContainerOperation:
			counts["remove"]++
		}
	}
	return counts
}

// getMachineIDsFromRunOps extracts machine IDs from RunContainerOperations.
func getMachineIDsFromRunOps(ops []Operation) []string {
	var machineIDs []string
	for _, op := range ops {
		if runOp, ok := op.(*RunContainerOperation); ok {
			machineIDs = append(machineIDs, runOp.MachineID)
		}
	}
	return machineIDs
}

func TestRollingStrategy_planReplicated_WithResources(t *testing.T) {
	t.Parallel()

	t.Run("schedule replicas across machines with sufficient resources", func(t *testing.T) {
		state := newTestClusterState(
			newTestMachine("m1", "node1", 4, 8),
			newTestMachine("m2", "node2", 4, 8),
		)

		spec := api.ServiceSpec{
			Name:     "test-service",
			Mode:     api.ServiceModeReplicated,
			Replicas: 4,
			Container: api.ContainerSpec{
				Image: "nginx:latest",
				Resources: api.ContainerResources{
					CPUReservation:    1 * core,
					MemoryReservation: 1 * gb,
				},
			},
		}

		strategy := &RollingStrategy{}
		plan, err := strategy.Plan(state, nil, spec)

		require.NoError(t, err)
		assert.NotEmpty(t, plan.ServiceID)
		assert.Equal(t, "test-service", plan.ServiceName)

		// Should have 4 RunContainerOperations
		counts := countOperationsByType(plan.Operations)
		assert.Equal(t, 4, counts["run"])

		// Should spread across machines
		machineIDs := getMachineIDsFromRunOps(plan.Operations)
		m1Count := 0
		m2Count := 0
		for _, mid := range machineIDs {
			if mid == "m1" {
				m1Count++
			} else if mid == "m2" {
				m2Count++
			}
		}
		assert.Equal(t, 2, m1Count, "Should schedule 2 containers on m1")
		assert.Equal(t, 2, m2Count, "Should schedule 2 containers on m2")
	})

	t.Run("error when replica count exceeds cluster capacity", func(t *testing.T) {
		state := newTestClusterState(
			newTestMachine("m1", "node1", 2, 8), // Can only fit 2 containers
			newTestMachine("m2", "node2", 2, 8), // Can only fit 2 containers
		)

		spec := api.ServiceSpec{
			Name:     "test-service",
			Mode:     api.ServiceModeReplicated,
			Replicas: 5, // Need 5 but cluster can only handle 4
			Container: api.ContainerSpec{
				Image: "nginx:latest",
				Resources: api.ContainerResources{
					CPUReservation: 1 * core,
				},
			},
		}

		strategy := &RollingStrategy{}
		_, err := strategy.Plan(state, nil, spec)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot schedule replica")
	})

	t.Run("respects resource constraints during scheduling", func(t *testing.T) {
		state := newTestClusterState(
			newTestMachine("m1", "node1", 2, 8),  // Small machine
			newTestMachine("m2", "node2", 8, 16), // Large machine
		)

		spec := api.ServiceSpec{
			Name:     "test-service",
			Mode:     api.ServiceModeReplicated,
			Replicas: 4,
			Container: api.ContainerSpec{
				Image: "nginx:latest",
				Resources: api.ContainerResources{
					CPUReservation: 2 * core, // Requires 2 cores per container
				},
			},
		}

		strategy := &RollingStrategy{}
		plan, err := strategy.Plan(state, nil, spec)

		require.NoError(t, err)

		// m1 can only fit 1 container (2 cores total, 2 cores per container)
		// m2 can fit 3 containers (8 cores total, 2 cores per container)
		// We need 4 containers, so: m1 gets 1, m2 gets 3
		machineIDs := getMachineIDsFromRunOps(plan.Operations)
		m1Count := 0
		m2Count := 0
		for _, mid := range machineIDs {
			if mid == "m1" {
				m1Count++
			} else if mid == "m2" {
				m2Count++
			}
		}
		assert.Equal(t, 1, m1Count, "m1 should have 1 container (limited by resources)")
		assert.Equal(t, 3, m2Count, "m2 should have 3 containers")
	})

	t.Run("up-to-date containers dont consume additional scheduled resources", func(t *testing.T) {
		// Create a machine with limited capacity
		state := newTestClusterState(
			newTestMachine("m1", "node1", 2, 8),
		)
		// Simulate that this machine already has reserved resources for 1 running container
		state.Machines[0].Info.ReservedCpuNanos = 1 * core

		existingService := &api.Service{
			ID:   "svc-123",
			Name: "test-service",
			Mode: api.ServiceModeReplicated,
			Containers: []api.MachineServiceContainer{
				{
					MachineID: "m1",
					Container: api.ServiceContainer{
						Container: api.Container{
							InspectResponse: container.InspectResponse{
								ContainerJSONBase: &container.ContainerJSONBase{
									ID:    "container-1",
									State: &container.State{Running: true},
								},
							},
						},
						ServiceSpec: api.ServiceSpec{
							Name:     "test-service",
							Mode:     api.ServiceModeReplicated,
							Replicas: 1,
							Container: api.ContainerSpec{
								Image: "nginx:latest",
								Resources: api.ContainerResources{
									CPUReservation: 1 * core,
								},
							},
						},
					},
				},
			},
		}

		spec := api.ServiceSpec{
			Name:     "test-service",
			Mode:     api.ServiceModeReplicated,
			Replicas: 2, // Scale from 1 to 2
			Container: api.ContainerSpec{
				Image: "nginx:latest",
				Resources: api.ContainerResources{
					CPUReservation: 1 * core,
				},
			},
		}

		strategy := &RollingStrategy{}
		plan, err := strategy.Plan(state, existingService, spec)

		require.NoError(t, err)

		// Should only run 1 new container (existing is up-to-date)
		counts := countOperationsByType(plan.Operations)
		assert.Equal(t, 1, counts["run"], "Should only run 1 new container")
		assert.Equal(t, 0, counts["remove"], "Should not remove any containers")
	})

	t.Run("spread behavior distributes containers across machines", func(t *testing.T) {
		state := newTestClusterState(
			newTestMachine("m1", "node1", 8, 16),
			newTestMachine("m2", "node2", 8, 16),
			newTestMachine("m3", "node3", 8, 16),
		)

		spec := api.ServiceSpec{
			Name:     "test-service",
			Mode:     api.ServiceModeReplicated,
			Replicas: 6,
			Container: api.ContainerSpec{
				Image: "nginx:latest",
			},
		}

		strategy := &RollingStrategy{}
		plan, err := strategy.Plan(state, nil, spec)

		require.NoError(t, err)

		machineIDs := getMachineIDsFromRunOps(plan.Operations)
		machineCounts := make(map[string]int)
		for _, mid := range machineIDs {
			machineCounts[mid]++
		}

		// Each machine should get exactly 2 containers (even spread)
		assert.Equal(t, 2, machineCounts["m1"], "m1 should have 2 containers")
		assert.Equal(t, 2, machineCounts["m2"], "m2 should have 2 containers")
		assert.Equal(t, 2, machineCounts["m3"], "m3 should have 2 containers")
	})

	t.Run("spread ranking prefers machines with fewer containers", func(t *testing.T) {
		state := newTestClusterState(
			newTestMachine("m1", "node1", 8, 16),
			newTestMachine("m2", "node2", 8, 16),
		)

		// Existing service with 2 containers on m1
		existingService := &api.Service{
			ID:   "svc-123",
			Name: "test-service",
			Mode: api.ServiceModeReplicated,
			Containers: []api.MachineServiceContainer{
				{
					MachineID: "m1",
					Container: api.ServiceContainer{
						Container: api.Container{
							InspectResponse: container.InspectResponse{
								ContainerJSONBase: &container.ContainerJSONBase{
									ID:    "c1",
									State: &container.State{Running: true},
								},
								Config: &container.Config{
									Labels: map[string]string{},
								},
							},
						},
						ServiceSpec: api.ServiceSpec{
							Name:      "test-service",
							Container: api.ContainerSpec{Image: "nginx:old"},
						},
					},
				},
				{
					MachineID: "m1",
					Container: api.ServiceContainer{
						Container: api.Container{
							InspectResponse: container.InspectResponse{
								ContainerJSONBase: &container.ContainerJSONBase{
									ID:    "c2",
									State: &container.State{Running: true},
								},
								Config: &container.Config{
									Labels: map[string]string{},
								},
							},
						},
						ServiceSpec: api.ServiceSpec{
							Name:      "test-service",
							Container: api.ContainerSpec{Image: "nginx:old"},
						},
					},
				},
			},
		}

		spec := api.ServiceSpec{
			Name:     "test-service",
			Mode:     api.ServiceModeReplicated,
			Replicas: 4,
			Container: api.ContainerSpec{
				Image:     "nginx:latest",                                   // Image changed - needs recreate
				Resources: api.ContainerResources{CPUReservation: 1 * core}, // trigger spread ranker that considers existing load
			},
		}

		strategy := &RollingStrategy{}
		plan, err := strategy.Plan(state, existingService, spec)

		require.NoError(t, err)

		// Spread ranking prefers machines with fewer containers:
		// - m1 starts with 2 existing containers, m2 starts with 0
		// - Scheduler prefers m2 initially (0 < 2), then balances as containers are scheduled
		// - Result: m2 gets more new containers since it started empty
		machineIDs := getMachineIDsFromRunOps(plan.Operations)
		machineCounts := make(map[string]int)
		for _, mid := range machineIDs {
			machineCounts[mid]++
		}

		// m2 should get more new containers since it started with 0
		// m1's existing containers get replaced/removed as part of the plan
		assert.Greater(t, machineCounts["m2"], machineCounts["m1"],
			"m2 should get more new containers since it was initially empty")

		// Total should still be 4 replicas
		assert.Equal(t, 4, machineCounts["m1"]+machineCounts["m2"], "Total should be 4 replicas")
	})

	t.Run("port conflicts stop existing container before running new one", func(t *testing.T) {
		state := newTestClusterState(
			newTestMachine("m1", "node1", 2, 4),
		)

		existingService := &api.Service{
			ID:   "svc-123",
			Name: "test-service",
			Mode: api.ServiceModeReplicated,
			Containers: []api.MachineServiceContainer{
				{
					MachineID: "m1",
					Container: api.ServiceContainer{
						Container: api.Container{InspectResponse: container.InspectResponse{
							ContainerJSONBase: &container.ContainerJSONBase{
								ID:    "c1",
								State: &container.State{Running: true},
							},
							Config: &container.Config{Labels: map[string]string{
								api.LabelServicePorts: "8080:80/tcp@host",
							}},
						}},
						ServiceSpec: api.ServiceSpec{ // old spec
							Name:      "test-service",
							Mode:      api.ServiceModeReplicated,
							Container: api.ContainerSpec{Image: "nginx:old"},
						},
					},
				},
			},
		}

		spec := api.ServiceSpec{
			Name:     "test-service",
			Mode:     api.ServiceModeReplicated,
			Replicas: 1,
			Container: api.ContainerSpec{
				Image: "nginx:new",
			},
			Ports: []api.PortSpec{{
				Mode:          api.PortModeHost,
				PublishedPort: 8080,
				ContainerPort: 80,
				Protocol:      api.ProtocolTCP,
			}},
		}

		strategy := &RollingStrategy{}
		plan, err := strategy.Plan(state, existingService, spec)

		require.NoError(t, err)
		counts := countOperationsByType(plan.Operations)
		assert.Equal(t, 1, counts["stop"], "conflicting container should be stopped")
		assert.Equal(t, 1, counts["run"], "new container should be run")
		assert.Equal(t, 1, counts["remove"], "old container should be removed")
	})

	t.Run("force recreate replaces even up-to-date containers", func(t *testing.T) {
		state := newTestClusterState(
			newTestMachine("m1", "node1", 2, 4),
		)

		existingService := &api.Service{
			ID:   "svc-123",
			Name: "test-service",
			Mode: api.ServiceModeReplicated,
			Containers: []api.MachineServiceContainer{
				{
					MachineID: "m1",
					Container: api.ServiceContainer{
						Container: api.Container{InspectResponse: container.InspectResponse{
							ContainerJSONBase: &container.ContainerJSONBase{
								ID:    "c1",
								State: &container.State{Running: true},
							},
							Config: &container.Config{Labels: map[string]string{}},
						}},
						ServiceSpec: api.ServiceSpec{
							Name:      "test-service",
							Mode:      api.ServiceModeReplicated,
							Container: api.ContainerSpec{Image: "nginx:latest"},
						},
					},
				},
			},
		}

		spec := api.ServiceSpec{
			Name:      "test-service",
			Mode:      api.ServiceModeReplicated,
			Replicas:  1,
			Container: api.ContainerSpec{Image: "nginx:latest"},
		}

		strategy := &RollingStrategy{ForceRecreate: true}
		plan, err := strategy.Plan(state, existingService, spec)

		require.NoError(t, err)
		counts := countOperationsByType(plan.Operations)
		assert.Equal(t, 1, counts["run"], "up-to-date container should still be recreated")
		assert.Equal(t, 1, counts["remove"], "old container should be removed")
	})
}

func TestRollingStrategy_planGlobal_WithResources(t *testing.T) {
	t.Parallel()

	t.Run("all machines must satisfy constraints", func(t *testing.T) {
		state := newTestClusterState(
			newTestMachine("m1", "node1", 4, 8),
			newTestMachine("m2", "node2", 4, 8),
			newTestMachine("m3", "node3", 1, 8), // Not enough CPU
		)

		spec := api.ServiceSpec{
			Name: "test-service",
			Mode: api.ServiceModeGlobal,
			Container: api.ContainerSpec{
				Image: "nginx:latest",
				Resources: api.ContainerResources{
					CPUReservation: 2 * core,
				},
			},
		}

		strategy := &RollingStrategy{}
		_, err := strategy.Plan(state, nil, spec)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "global service")
		assert.Contains(t, err.Error(), "2 of 3 machines are eligible")
	})

	t.Run("global service with resource requirements on sufficient cluster", func(t *testing.T) {
		state := newTestClusterState(
			newTestMachine("m1", "node1", 4, 8),
			newTestMachine("m2", "node2", 4, 8),
			newTestMachine("m3", "node3", 4, 8),
		)

		spec := api.ServiceSpec{
			Name: "test-service",
			Mode: api.ServiceModeGlobal,
			Container: api.ContainerSpec{
				Image: "nginx:latest",
				Resources: api.ContainerResources{
					CPUReservation:    2 * core,
					MemoryReservation: 4 * gb,
				},
			},
		}

		strategy := &RollingStrategy{}
		plan, err := strategy.Plan(state, nil, spec)

		require.NoError(t, err)

		// Should have 3 RunContainerOperations (one per machine)
		counts := countOperationsByType(plan.Operations)
		assert.Equal(t, 3, counts["run"])
	})

	t.Run("error when any machine lacks required resources", func(t *testing.T) {
		state := newTestClusterState(
			newTestMachine("m1", "node1", 4, 8),
			newTestMachine("m2", "node2", 4, 2), // Not enough memory
		)

		spec := api.ServiceSpec{
			Name: "test-service",
			Mode: api.ServiceModeGlobal,
			Container: api.ContainerSpec{
				Image: "nginx:latest",
				Resources: api.ContainerResources{
					MemoryReservation: 4 * gb,
				},
			},
		}

		strategy := &RollingStrategy{}
		_, err := strategy.Plan(state, nil, spec)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "1 of 2 machines are eligible")
	})

	t.Run("global service without resource constraints succeeds", func(t *testing.T) {
		state := newTestClusterState(
			newTestMachine("m1", "node1", 1, 1),
			newTestMachine("m2", "node2", 1, 1),
		)

		spec := api.ServiceSpec{
			Name: "test-service",
			Mode: api.ServiceModeGlobal,
			Container: api.ContainerSpec{
				Image: "nginx:latest",
				// No resource constraints
			},
		}

		strategy := &RollingStrategy{}
		plan, err := strategy.Plan(state, nil, spec)

		require.NoError(t, err)
		counts := countOperationsByType(plan.Operations)
		assert.Equal(t, 2, counts["run"])
	})
}

func TestRollingStrategy_Plan_RequiresClusterState(t *testing.T) {
	t.Parallel()

	spec := api.ServiceSpec{
		Name: "test-service",
		Mode: api.ServiceModeReplicated,
		Container: api.ContainerSpec{
			Image: "nginx:latest",
		},
	}

	strategy := &RollingStrategy{}
	_, err := strategy.Plan(nil, nil, spec)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cluster state must be provided")
}

func TestRollingStrategy_planGlobal_WithPlacementConstraints(t *testing.T) {
	t.Parallel()

	t.Run("global service with x-machine deploys only to specified machines", func(t *testing.T) {
		// Cluster has 3 machines, but we only want to deploy to 2 of them using x-machine
		state := newTestClusterState(
			newTestMachine("m1", "node1", 4, 8),
			newTestMachine("m2", "node2", 4, 8),
			newTestMachine("m3", "node3", 4, 8),
		)

		spec := api.ServiceSpec{
			Name: "test-service",
			Mode: api.ServiceModeGlobal,
			Container: api.ContainerSpec{
				Image: "nginx:latest",
			},
			Placement: api.Placement{
				Machines: []string{"node1", "node2"}, // x-machine constraint: only deploy to node1 and node2
			},
		}

		strategy := &RollingStrategy{}
		plan, err := strategy.Plan(state, nil, spec)

		// This should succeed - global with x-machine should deploy to all machines in the constraint list
		require.NoError(t, err, "global service with x-machine should succeed when all specified machines are eligible")

		// Should have 2 RunContainerOperations (one per machine in the constraint)
		counts := countOperationsByType(plan.Operations)
		assert.Equal(t, 2, counts["run"], "should run containers on both specified machines")

		// Verify containers are scheduled on the correct machines
		machineIDs := getMachineIDsFromRunOps(plan.Operations)
		assert.Contains(t, machineIDs, "m1", "should deploy to node1")
		assert.Contains(t, machineIDs, "m2", "should deploy to node2")
		assert.NotContains(t, machineIDs, "m3", "should not deploy to node3")
	})
}

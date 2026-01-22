package scheduler

import (
	"testing"

	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpreadRanker(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		machineA *Machine
		machineB *Machine
		wantLess bool // true if A should be preferred over B
	}{
		{
			name: "prefers machine with fewer total containers",
			machineA: &Machine{
				ExistingContainers:  1,
				ScheduledContainers: 0,
			},
			machineB: &Machine{
				ExistingContainers:  3,
				ScheduledContainers: 0,
			},
			wantLess: true,
		},
		{
			name: "considers scheduled containers too",
			machineA: &Machine{
				ExistingContainers:  1,
				ScheduledContainers: 2,
			},
			machineB: &Machine{
				ExistingContainers:  2,
				ScheduledContainers: 0,
			},
			wantLess: false, // A has 3 total, B has 2 total
		},
		{
			name: "considers both existing and scheduled",
			machineA: &Machine{
				ExistingContainers:  2,
				ScheduledContainers: 1,
			},
			machineB: &Machine{
				ExistingContainers:  1,
				ScheduledContainers: 3,
			},
			wantLess: true, // A has 3 total, B has 4 total
		},
		{
			name: "equal counts",
			machineA: &Machine{
				ExistingContainers:  2,
				ScheduledContainers: 1,
			},
			machineB: &Machine{
				ExistingContainers:  2,
				ScheduledContainers: 1,
			},
			wantLess: false, // Not less when equal
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SpreadRanker.Less(tt.machineA, tt.machineB)
			assert.Equal(t, tt.wantLess, result)
		})
	}
}

func TestServiceScheduler_EligibleMachines(t *testing.T) {
	t.Parallel()

	const (
		core = int64(1e9)
		gb   = int64(1024 * 1024 * 1024)
	)

	tests := []struct {
		name        string
		state       *ClusterState
		spec        api.ServiceSpec
		wantCount   int
		wantErr     string
		wantMachine string // Optional: check specific machine is included
	}{
		{
			name: "all machines eligible - no constraints",
			state: &ClusterState{
				Machines: []*Machine{
					{Info: &pb.MachineInfo{Id: "m1", TotalCpuNanos: 4 * core, TotalMemoryBytes: 8 * gb}},
					{Info: &pb.MachineInfo{Id: "m2", TotalCpuNanos: 4 * core, TotalMemoryBytes: 8 * gb}},
					{Info: &pb.MachineInfo{Id: "m3", TotalCpuNanos: 4 * core, TotalMemoryBytes: 8 * gb}},
				},
			},
			spec:      api.ServiceSpec{},
			wantCount: 3,
		},
		{
			name: "placement constraint filters machines",
			state: &ClusterState{
				Machines: []*Machine{
					{Info: &pb.MachineInfo{Id: "m1", Name: "node1", TotalCpuNanos: 4 * core, TotalMemoryBytes: 8 * gb}},
					{Info: &pb.MachineInfo{Id: "m2", Name: "node2", TotalCpuNanos: 4 * core, TotalMemoryBytes: 8 * gb}},
					{Info: &pb.MachineInfo{Id: "m3", Name: "node3", TotalCpuNanos: 4 * core, TotalMemoryBytes: 8 * gb}},
				},
			},
			spec: api.ServiceSpec{
				Placement: api.Placement{Machines: []string{"node1", "node3"}},
			},
			wantCount: 2,
		},
		{
			name: "resource constraint filters machines without capacity",
			state: &ClusterState{
				Machines: []*Machine{
					{Info: &pb.MachineInfo{Id: "m1", TotalCpuNanos: 4 * core, TotalMemoryBytes: 8 * gb}},
					{Info: &pb.MachineInfo{Id: "m2", TotalCpuNanos: 1 * core, TotalMemoryBytes: 8 * gb}}, // Not enough CPU
					{Info: &pb.MachineInfo{Id: "m3", TotalCpuNanos: 4 * core, TotalMemoryBytes: 8 * gb}},
				},
			},
			spec: api.ServiceSpec{
				Container: api.ContainerSpec{
					Resources: api.ContainerResources{CPUReservation: 2 * core},
				},
			},
			wantCount: 2,
		},
		{
			name: "multiple constraints combined",
			state: &ClusterState{
				Machines: []*Machine{
					{Info: &pb.MachineInfo{Id: "m1", Name: "node1", TotalCpuNanos: 4 * core, TotalMemoryBytes: 8 * gb}},
					{Info: &pb.MachineInfo{Id: "m2", Name: "node2", TotalCpuNanos: 1 * core, TotalMemoryBytes: 8 * gb}}, // Not enough CPU
					{Info: &pb.MachineInfo{Id: "m3", Name: "node3", TotalCpuNanos: 4 * core, TotalMemoryBytes: 8 * gb}}, // Not in placement
				},
			},
			spec: api.ServiceSpec{
				Placement: api.Placement{Machines: []string{"node1", "node2"}},
				Container: api.ContainerSpec{
					Resources: api.ContainerResources{CPUReservation: 2 * core},
				},
			},
			wantCount:   1,
			wantMachine: "m1",
		},
		{
			name: "no eligible machines - returns error",
			state: &ClusterState{
				Machines: []*Machine{
					{Info: &pb.MachineInfo{Id: "m1", TotalCpuNanos: 1 * core, TotalMemoryBytes: 8 * gb}},
					{Info: &pb.MachineInfo{Id: "m2", TotalCpuNanos: 1 * core, TotalMemoryBytes: 8 * gb}},
				},
			},
			spec: api.ServiceSpec{
				Container: api.ContainerSpec{
					Resources: api.ContainerResources{CPUReservation: 4 * core},
				},
			},
			wantErr: "no eligible machines",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sched := NewServiceScheduler(tt.state, tt.spec)
			machines, report, err := sched.EligibleMachines()

			if tt.wantErr != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.NotNil(t, report, "report should be returned even on error")
				assert.NotEmpty(t, report.Error(), "report error should provide details")
				return
			}

			require.NoError(t, err)
			assert.Len(t, machines, tt.wantCount)
			assert.NotNil(t, report)

			if tt.wantMachine != "" {
				found := false
				for _, m := range machines {
					if m.Info.Id == tt.wantMachine {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected machine %s to be in eligible list", tt.wantMachine)
			}
		})
	}
}

func TestServiceScheduler_ScheduleContainer(t *testing.T) {
	t.Parallel()

	const (
		core = int64(1e9)
		gb   = int64(1024 * 1024 * 1024)
	)

	t.Run("single machine single container", func(t *testing.T) {
		state := &ClusterState{
			Machines: []*Machine{
				{Info: &pb.MachineInfo{Id: "m1", TotalCpuNanos: 4 * core, TotalMemoryBytes: 8 * gb}},
			},
		}
		spec := api.ServiceSpec{
			Container: api.ContainerSpec{
				Resources: api.ContainerResources{CPUReservation: 1 * core},
			},
		}

		sched := NewServiceScheduler(state, spec)
		m, report, err := sched.ScheduleContainer()

		require.NoError(t, err)
		assert.NotNil(t, report)
		assert.Equal(t, "m1", m.Info.Id)
		assert.Equal(t, 1*core, m.ScheduledCPU)
		assert.Equal(t, 1, m.ScheduledContainers)
	})

	t.Run("multiple machines prefers least loaded when reservations set", func(t *testing.T) {
		state := &ClusterState{
			Machines: []*Machine{
				{
					Info:               &pb.MachineInfo{Id: "m1", TotalCpuNanos: 4 * core, TotalMemoryBytes: 8 * gb},
					ExistingContainers: 3,
				},
				{
					Info:               &pb.MachineInfo{Id: "m2", TotalCpuNanos: 4 * core, TotalMemoryBytes: 8 * gb},
					ExistingContainers: 1,
				},
				{
					Info:               &pb.MachineInfo{Id: "m3", TotalCpuNanos: 4 * core, TotalMemoryBytes: 8 * gb},
					ExistingContainers: 2,
				},
			},
		}
		spec := api.ServiceSpec{
			Container: api.ContainerSpec{Resources: api.ContainerResources{CPUReservation: 1 * core}},
		}

		sched := NewServiceScheduler(state, spec)
		m, _, err := sched.ScheduleContainer()

		require.NoError(t, err)
		assert.Equal(t, "m2", m.Info.Id) // Least loaded machine
	})

	t.Run("no reservations round robins ignoring existing load", func(t *testing.T) {
		state := &ClusterState{
			Machines: []*Machine{
				{Info: &pb.MachineInfo{Id: "m1", TotalCpuNanos: 4 * core, TotalMemoryBytes: 8 * gb}, ExistingContainers: 6},
				{Info: &pb.MachineInfo{Id: "m2", TotalCpuNanos: 4 * core, TotalMemoryBytes: 8 * gb}, ExistingContainers: 2},
			},
		}
		spec := api.ServiceSpec{} // no reservations

		sched := NewServiceScheduler(state, spec)

		for i := 0; i < 4; i++ {
			_, _, err := sched.ScheduleContainer()
			require.NoError(t, err)
		}

		assert.Equal(t, 2, state.Machines[0].ScheduledContainers)
		assert.Equal(t, 2, state.Machines[1].ScheduledContainers)
	})

	t.Run("reserves resources on selected machine", func(t *testing.T) {
		state := &ClusterState{
			Machines: []*Machine{
				{Info: &pb.MachineInfo{Id: "m1", TotalCpuNanos: 4 * core, TotalMemoryBytes: 8 * gb}},
			},
		}
		spec := api.ServiceSpec{
			Container: api.ContainerSpec{
				Resources: api.ContainerResources{
					CPUReservation:    2 * core,
					MemoryReservation: 1 * gb,
				},
			},
		}

		sched := NewServiceScheduler(state, spec)
		m, _, err := sched.ScheduleContainer()

		require.NoError(t, err)
		assert.Equal(t, 2*core, m.ScheduledCPU)
		assert.Equal(t, 1*gb, m.ScheduledMemory)
		assert.Equal(t, 1, m.ScheduledContainers)
	})

	t.Run("multiple calls spread across machines", func(t *testing.T) {
		state := &ClusterState{
			Machines: []*Machine{
				{Info: &pb.MachineInfo{Id: "m1", TotalCpuNanos: 4 * core, TotalMemoryBytes: 8 * gb}},
				{Info: &pb.MachineInfo{Id: "m2", TotalCpuNanos: 4 * core, TotalMemoryBytes: 8 * gb}},
			},
		}
		spec := api.ServiceSpec{}

		sched := NewServiceScheduler(state, spec)

		m1, _, err := sched.ScheduleContainer()
		require.NoError(t, err)
		m2, _, err := sched.ScheduleContainer()
		require.NoError(t, err)
		m3, _, err := sched.ScheduleContainer()
		require.NoError(t, err)
		m4, _, err := sched.ScheduleContainer()
		require.NoError(t, err)

		// Should alternate between machines (due to spread ranking)
		scheduled := map[string]int{
			m1.Info.Id: 1,
			m2.Info.Id: 1,
			m3.Info.Id: 1,
			m4.Info.Id: 1,
		}
		// Each machine should have 2 containers scheduled
		assert.Equal(t, 2, state.Machines[0].ScheduledContainers)
		assert.Equal(t, 2, state.Machines[1].ScheduledContainers)
		_ = scheduled
	})

	t.Run("returns error when no capacity - resource exhaustion", func(t *testing.T) {
		state := &ClusterState{
			Machines: []*Machine{
				{Info: &pb.MachineInfo{Id: "m1", Name: "node1", TotalCpuNanos: 2 * core, TotalMemoryBytes: 8 * gb}},
			},
		}
		spec := api.ServiceSpec{
			Container: api.ContainerSpec{
				Resources: api.ContainerResources{CPUReservation: 1 * core},
			},
		}

		sched := NewServiceScheduler(state, spec)

		// First two should succeed
		_, _, err := sched.ScheduleContainer()
		require.NoError(t, err)
		_, _, err = sched.ScheduleContainer()
		require.NoError(t, err)

		// Third should fail - no more CPU capacity
		_, report, err := sched.ScheduleContainer()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no eligible machines")
		assert.NotNil(t, report)
		assert.Contains(t, report.Error(), "insufficient CPU")
	})

	t.Run("re-filters after resource exhaustion on some machines", func(t *testing.T) {
		state := &ClusterState{
			Machines: []*Machine{
				{Info: &pb.MachineInfo{Id: "m1", TotalCpuNanos: 2 * core, TotalMemoryBytes: 8 * gb}},
				{Info: &pb.MachineInfo{Id: "m2", TotalCpuNanos: 4 * core, TotalMemoryBytes: 8 * gb}},
			},
		}
		spec := api.ServiceSpec{
			Container: api.ContainerSpec{
				Resources: api.ContainerResources{CPUReservation: 1 * core},
			},
		}

		sched := NewServiceScheduler(state, spec)

		// Schedule 4 containers - first 2 per machine, then m1 runs out
		for i := 0; i < 4; i++ {
			m, _, err := sched.ScheduleContainer()
			require.NoError(t, err)
			t.Logf("Scheduled container %d on %s", i+1, m.Info.Id)
		}

		// m1 should have 2 containers, m2 should have 2 containers
		// (spread ranking keeps them balanced until m1 runs out)
		assert.Equal(t, 2*core, state.Machines[0].ScheduledCPU)
		assert.Equal(t, 2*core, state.Machines[1].ScheduledCPU)

		// Next 2 containers should go to m2 only (m1 is exhausted)
		for i := 0; i < 2; i++ {
			m, _, err := sched.ScheduleContainer()
			require.NoError(t, err)
			assert.Equal(t, "m2", m.Info.Id)
		}

		// m2 should now be at capacity
		_, _, err := sched.ScheduleContainer()
		assert.Error(t, err)
	})
}

func TestServiceScheduler_UnscheduleContainer(t *testing.T) {
	t.Parallel()

	const (
		core = int64(1e9)
		gb   = int64(1024 * 1024 * 1024)
	)

	t.Run("decrements scheduled resources", func(t *testing.T) {
		state := &ClusterState{
			Machines: []*Machine{
				{Info: &pb.MachineInfo{Id: "m1", TotalCpuNanos: 4 * core, TotalMemoryBytes: 8 * gb}},
			},
		}
		spec := api.ServiceSpec{
			Container: api.ContainerSpec{
				Resources: api.ContainerResources{
					CPUReservation:    1 * core,
					MemoryReservation: 512 * 1024 * 1024,
				},
			},
		}

		sched := NewServiceScheduler(state, spec)
		m, _, err := sched.ScheduleContainer()
		require.NoError(t, err)

		assert.Equal(t, 1*core, m.ScheduledCPU)
		assert.Equal(t, int64(512*1024*1024), m.ScheduledMemory)
		assert.Equal(t, 1, m.ScheduledContainers)

		sched.UnscheduleContainer(m)

		assert.Equal(t, int64(0), m.ScheduledCPU)
		assert.Equal(t, int64(0), m.ScheduledMemory)
		assert.Equal(t, 0, m.ScheduledContainers)
	})

	t.Run("scheduled containers minimum is 0", func(t *testing.T) {
		state := &ClusterState{
			Machines: []*Machine{
				{Info: &pb.MachineInfo{Id: "m1", TotalCpuNanos: 4 * core, TotalMemoryBytes: 8 * gb}},
			},
		}
		spec := api.ServiceSpec{}

		sched := NewServiceScheduler(state, spec)
		m, _, err := sched.ScheduleContainer()
		require.NoError(t, err)

		// Unschedule twice - should not go negative
		sched.UnscheduleContainer(m)
		sched.UnscheduleContainer(m)

		assert.Equal(t, 0, m.ScheduledContainers)
	})

	t.Run("no-op if heap not initialized", func(t *testing.T) {
		state := &ClusterState{
			Machines: []*Machine{
				{Info: &pb.MachineInfo{Id: "m1", TotalCpuNanos: 4 * core, TotalMemoryBytes: 8 * gb}},
			},
		}
		spec := api.ServiceSpec{
			Container: api.ContainerSpec{
				Resources: api.ContainerResources{CPUReservation: 1 * core},
			},
		}

		sched := NewServiceScheduler(state, spec)

		// Call UnscheduleContainer before any ScheduleContainer call
		// Should not panic
		sched.UnscheduleContainer(state.Machines[0])
	})
}

func TestServiceScheduler_CustomRanker(t *testing.T) {
	t.Parallel()

	const (
		core = int64(1e9)
		gb   = int64(1024 * 1024 * 1024)
	)

	// Custom ranker that prefers machines with more memory
	memoryRanker := MachineRankerFunc(func(a, b *Machine) bool {
		return a.AvailableMemory() > b.AvailableMemory()
	})

	state := &ClusterState{
		Machines: []*Machine{
			{Info: &pb.MachineInfo{Id: "m1", TotalCpuNanos: 4 * core, TotalMemoryBytes: 4 * gb}},
			{Info: &pb.MachineInfo{Id: "m2", TotalCpuNanos: 4 * core, TotalMemoryBytes: 8 * gb}},
			{Info: &pb.MachineInfo{Id: "m3", TotalCpuNanos: 4 * core, TotalMemoryBytes: 16 * gb}},
		},
	}
	spec := api.ServiceSpec{}

	sched := NewServiceSchedulerWithRanker(state, spec, memoryRanker)
	m, _, err := sched.ScheduleContainer()

	require.NoError(t, err)
	assert.Equal(t, "m3", m.Info.Id) // Should pick machine with most memory
}

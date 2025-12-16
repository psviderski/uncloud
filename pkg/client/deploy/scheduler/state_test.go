package scheduler

import (
	"testing"

	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/stretchr/testify/assert"
)

func TestMachine_AvailableCPU(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		machine *Machine
		wantCPU int64
	}{
		{
			name: "fresh machine with no reservations",
			machine: &Machine{
				Info: &pb.MachineInfo{
					TotalCpuNanos:    4e9, // 4 cores
					ReservedCpuNanos: 0,
				},
			},
			wantCPU: 4e9,
		},
		{
			name: "machine with existing reserved resources",
			machine: &Machine{
				Info: &pb.MachineInfo{
					TotalCpuNanos:    4e9,
					ReservedCpuNanos: 1e9, // 1 core reserved
				},
			},
			wantCPU: 3e9,
		},
		{
			name: "machine with scheduled resources",
			machine: &Machine{
				Info: &pb.MachineInfo{
					TotalCpuNanos:    4e9,
					ReservedCpuNanos: 0,
				},
				ScheduledCPU: 2e9, // 2 cores scheduled
			},
			wantCPU: 2e9,
		},
		{
			name: "machine with both reserved and scheduled resources",
			machine: &Machine{
				Info: &pb.MachineInfo{
					TotalCpuNanos:    4e9,
					ReservedCpuNanos: 1e9, // 1 core reserved
				},
				ScheduledCPU: 1e9, // 1 core scheduled
			},
			wantCPU: 2e9,
		},
		{
			name: "fully utilized machine",
			machine: &Machine{
				Info: &pb.MachineInfo{
					TotalCpuNanos:    4e9,
					ReservedCpuNanos: 2e9,
				},
				ScheduledCPU: 2e9,
			},
			wantCPU: 0,
		},
		{
			name: "over-committed machine returns negative",
			machine: &Machine{
				Info: &pb.MachineInfo{
					TotalCpuNanos:    4e9,
					ReservedCpuNanos: 3e9,
				},
				ScheduledCPU: 2e9,
			},
			wantCPU: -1e9,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.machine.AvailableCPU()
			assert.Equal(t, tt.wantCPU, result)
		})
	}
}

func TestMachine_AvailableMemory(t *testing.T) {
	t.Parallel()

	const gb = 1024 * 1024 * 1024

	tests := []struct {
		name       string
		machine    *Machine
		wantMemory int64
	}{
		{
			name: "fresh machine with no reservations",
			machine: &Machine{
				Info: &pb.MachineInfo{
					TotalMemoryBytes:    8 * gb,
					ReservedMemoryBytes: 0,
				},
			},
			wantMemory: 8 * gb,
		},
		{
			name: "machine with existing reserved resources",
			machine: &Machine{
				Info: &pb.MachineInfo{
					TotalMemoryBytes:    8 * gb,
					ReservedMemoryBytes: 2 * gb,
				},
			},
			wantMemory: 6 * gb,
		},
		{
			name: "machine with scheduled resources",
			machine: &Machine{
				Info: &pb.MachineInfo{
					TotalMemoryBytes:    8 * gb,
					ReservedMemoryBytes: 0,
				},
				ScheduledMemory: 3 * gb,
			},
			wantMemory: 5 * gb,
		},
		{
			name: "machine with both reserved and scheduled resources",
			machine: &Machine{
				Info: &pb.MachineInfo{
					TotalMemoryBytes:    8 * gb,
					ReservedMemoryBytes: 2 * gb,
				},
				ScheduledMemory: 2 * gb,
			},
			wantMemory: 4 * gb,
		},
		{
			name: "fully utilized machine",
			machine: &Machine{
				Info: &pb.MachineInfo{
					TotalMemoryBytes:    8 * gb,
					ReservedMemoryBytes: 4 * gb,
				},
				ScheduledMemory: 4 * gb,
			},
			wantMemory: 0,
		},
		{
			name: "over-committed machine returns negative",
			machine: &Machine{
				Info: &pb.MachineInfo{
					TotalMemoryBytes:    8 * gb,
					ReservedMemoryBytes: 6 * gb,
				},
				ScheduledMemory: 4 * gb,
			},
			wantMemory: -2 * gb,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.machine.AvailableMemory()
			assert.Equal(t, tt.wantMemory, result)
		})
	}
}

func TestMachine_ReserveResources(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		initialCPU       int64
		initialMemory    int64
		reserveCPU       int64
		reserveMemory    int64
		wantScheduledCPU int64
		wantScheduledMem int64
	}{
		{
			name:             "reserve CPU only",
			initialCPU:       0,
			initialMemory:    0,
			reserveCPU:       1e9,
			reserveMemory:    0,
			wantScheduledCPU: 1e9,
			wantScheduledMem: 0,
		},
		{
			name:             "reserve memory only",
			initialCPU:       0,
			initialMemory:    0,
			reserveCPU:       0,
			reserveMemory:    512 * 1024 * 1024,
			wantScheduledCPU: 0,
			wantScheduledMem: 512 * 1024 * 1024,
		},
		{
			name:             "reserve both CPU and memory",
			initialCPU:       0,
			initialMemory:    0,
			reserveCPU:       2e9,
			reserveMemory:    1024 * 1024 * 1024,
			wantScheduledCPU: 2e9,
			wantScheduledMem: 1024 * 1024 * 1024,
		},
		{
			name:             "accumulate reservations",
			initialCPU:       1e9,
			initialMemory:    512 * 1024 * 1024,
			reserveCPU:       1e9,
			reserveMemory:    512 * 1024 * 1024,
			wantScheduledCPU: 2e9,
			wantScheduledMem: 1024 * 1024 * 1024,
		},
		{
			name:             "zero reservation has no effect",
			initialCPU:       1e9,
			initialMemory:    512 * 1024 * 1024,
			reserveCPU:       0,
			reserveMemory:    0,
			wantScheduledCPU: 1e9,
			wantScheduledMem: 512 * 1024 * 1024,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			machine := &Machine{
				Info:            &pb.MachineInfo{},
				ScheduledCPU:    tt.initialCPU,
				ScheduledMemory: tt.initialMemory,
			}

			machine.ReserveResources(tt.reserveCPU, tt.reserveMemory)

			assert.Equal(t, tt.wantScheduledCPU, machine.ScheduledCPU)
			assert.Equal(t, tt.wantScheduledMem, machine.ScheduledMemory)
		})
	}
}

func TestClusterState_Machine(t *testing.T) {
	t.Parallel()

	state := &ClusterState{
		Machines: []*Machine{
			{
				Info: &pb.MachineInfo{
					Id:   "machine-1",
					Name: "node1",
				},
			},
			{
				Info: &pb.MachineInfo{
					Id:   "machine-2",
					Name: "node2",
				},
			},
		},
	}

	tests := []struct {
		name      string
		nameOrID  string
		wantFound bool
		wantID    string
	}{
		{
			name:      "find by ID",
			nameOrID:  "machine-1",
			wantFound: true,
			wantID:    "machine-1",
		},
		{
			name:      "find by name",
			nameOrID:  "node2",
			wantFound: true,
			wantID:    "machine-2",
		},
		{
			name:      "not found",
			nameOrID:  "nonexistent",
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			machine, found := state.Machine(tt.nameOrID)
			assert.Equal(t, tt.wantFound, found)
			if tt.wantFound {
				assert.Equal(t, tt.wantID, machine.Info.Id)
			}
		})
	}
}

package scheduler

import (
	"container/heap"
	"errors"

	"github.com/psviderski/uncloud/pkg/api"
)

// MachineRanker provides a comparison function for sorting machines during scheduling.
type MachineRanker interface {
	// Less returns true if machine a should be preferred over machine b for scheduling.
	Less(a, b *Machine) bool
}

// MachineRankerFunc is an adapter to allow ordinary functions to be used as MachineRankers.
type MachineRankerFunc func(a, b *Machine) bool

func (f MachineRankerFunc) Less(a, b *Machine) bool {
	return f(a, b)
}

// SpreadRanker prefers machines with fewer total containers (existing + scheduled), spreading load evenly.
// This provides round-robin-like behavior across machines.
var SpreadRanker = MachineRankerFunc(func(a, b *Machine) bool {
	aTotal := a.ExistingContainers + a.ScheduledContainers
	bTotal := b.ExistingContainers + b.ScheduledContainers
	return aTotal < bTotal
})

type ServiceScheduler struct {
	state       *ClusterState
	spec        api.ServiceSpec
	constraints []Constraint
	ranker      MachineRanker
	heap        *machineHeap
}

// NewServiceScheduler creates a new ServiceScheduler with the given cluster state and service specification.
func NewServiceScheduler(state *ClusterState, spec api.ServiceSpec) *ServiceScheduler {
	return NewServiceSchedulerWithRanker(state, spec, defaultRanker(spec))
}

// NewServiceSchedulerWithRanker creates a new ServiceScheduler with a custom machine ranker.
func NewServiceSchedulerWithRanker(state *ClusterState, spec api.ServiceSpec, ranker MachineRanker) *ServiceScheduler {
	constraints := constraintsFromSpec(spec)

	return &ServiceScheduler{
		state:       state,
		spec:        spec,
		constraints: constraints,
		ranker:      ranker,
	}
}

// defaultRanker selects the ranker based on resource reservations. When no CPU/memory
// reservations are set, use a round-robin-like ranker that ignores existing containers
// to preserve HA spread even if a machine already hosts other workloads. When any
// reservation is set, fall back to SpreadRanker which considers existing + scheduled.
func defaultRanker(spec api.ServiceSpec) MachineRanker {
	resources := spec.Container.Resources
	if resources.CPUReservation == 0 && resources.MemoryReservation == 0 {
		return NoReservationRanker
	}
	return SpreadRanker
}

// NoReservationRanker ignores existing containers and balances only the containers
// being scheduled in the current plan. This behaves like round-robin across eligible
// machines when no resource reservations are requested.
var NoReservationRanker = MachineRankerFunc(func(a, b *Machine) bool {
	if a.ScheduledContainers != b.ScheduledContainers {
		return a.ScheduledContainers < b.ScheduledContainers
	}
	return a.Info.Id < b.Info.Id
})

// EligibleMachines returns a list of machines that satisfy all constraints.
// Returns an error if no machines are eligible.
func (s *ServiceScheduler) EligibleMachines() ([]*Machine, error) {
	var available []*Machine
	for _, machine := range s.state.Machines {
		if s.evaluateConstraints(machine) {
			available = append(available, machine)
		}
	}
	if len(available) == 0 {
		return nil, errors.New("no eligible machines")
	}
	return available, nil
}

func (s *ServiceScheduler) evaluateConstraints(machine *Machine) bool {
	for _, c := range s.constraints {
		if !c.Evaluate(machine) {
			return false
		}
	}
	return true
}

// ScheduleContainer finds the best eligible machine for the next container, reserves resources on it,
// and returns the machine. Returns an error if no machine can accommodate the container.
func (s *ServiceScheduler) ScheduleContainer() (*Machine, error) {
	// Initialize heap on first call.
	if s.heap == nil {
		eligible, err := s.EligibleMachines()
		if err != nil {
			return nil, err
		}
		s.heap = &machineHeap{machines: eligible, ranker: s.ranker}
		heap.Init(s.heap)
	}

	// Re-filter the heap to remove machines that no longer satisfy constraints (e.g., out of resources).
	s.heap.machines = filterEligible(s.heap.machines, s.evaluateConstraints)
	if len(s.heap.machines) == 0 {
		return nil, errors.New("no eligible machines with sufficient resources")
	}
	heap.Init(s.heap)

	// Pop the best machine.
	m := heap.Pop(s.heap).(*Machine)

	// Reserve resources for this container.
	resources := s.spec.Container.Resources
	m.ReserveResources(resources.CPUReservation, resources.MemoryReservation)
	m.ScheduledContainers++

	// Push back with updated state so it gets re-ranked.
	heap.Push(s.heap, m)

	return m, nil
}

// UnscheduleContainer rolls back a previous reservation for a container on the given machine.
// Useful when scheduling determined that no new container needs to run (e.g., an existing one is up-to-date).
func (s *ServiceScheduler) UnscheduleContainer(m *Machine) {
	if s.heap == nil {
		return
	}

	resources := s.spec.Container.Resources
	m.ScheduledCPU -= resources.CPUReservation
	m.ScheduledMemory -= resources.MemoryReservation
	if m.ScheduledContainers > 0 {
		m.ScheduledContainers--
	}

	// Re-rank machines after resource adjustment.
	s.heap.machines = filterEligible(s.heap.machines, s.evaluateConstraints)
	heap.Init(s.heap)
}

func filterEligible(machines []*Machine, predicate func(*Machine) bool) []*Machine {
	result := machines[:0]
	for _, m := range machines {
		if predicate(m) {
			result = append(result, m)
		}
	}
	return result
}

// machineHeap implements heap.Interface for scheduling machines.
type machineHeap struct {
	machines []*Machine
	ranker   MachineRanker
}

func (h *machineHeap) Len() int { return len(h.machines) }

func (h *machineHeap) Less(i, j int) bool {
	return h.ranker.Less(h.machines[i], h.machines[j])
}

func (h *machineHeap) Swap(i, j int) {
	h.machines[i], h.machines[j] = h.machines[j], h.machines[i]
}

func (h *machineHeap) Push(x any) {
	h.machines = append(h.machines, x.(*Machine))
}

func (h *machineHeap) Pop() any {
	old := h.machines
	n := len(old)
	m := old[n-1]
	old[n-1] = nil
	h.machines = old[:n-1]
	return m
}

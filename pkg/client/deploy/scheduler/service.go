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
	// lastReport holds the most recent scheduling report for error reporting.
	lastReport *SchedulingReport
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

// EligibleMachines returns a list of machines that satisfy all constraints and a scheduling report.
// The report includes details on why ineligible machines failed.
// Returns an error if no machines are eligible.
func (s *ServiceScheduler) EligibleMachines() ([]*Machine, *SchedulingReport, error) {
	report := &SchedulingReport{}

	for _, machine := range s.state.Machines {
		eval := s.evaluateAllConstraints(machine)
		if eval.Passed() {
			report.Eligible = append(report.Eligible, machine)
		} else {
			report.Ineligible = append(report.Ineligible, eval)
		}
	}

	s.lastReport = report

	if len(report.Eligible) == 0 {
		return nil, report, errors.New("no eligible machines")
	}
	return report.Eligible, report, nil
}

// evaluateAllConstraints evaluates all constraints for a machine and returns the evaluation.
func (s *ServiceScheduler) evaluateAllConstraints(machine *Machine) MachineEvaluation {
	eval := MachineEvaluation{Machine: machine}

	for _, c := range s.constraints {
		result := c.Evaluate(machine)
		eval.Results = append(eval.Results, result)
	}

	return eval
}

// evaluateConstraintsSatisfied returns true if the machine satisfies all constraints.
func (s *ServiceScheduler) evaluateConstraintsSatisfied(machine *Machine) bool {
	for _, c := range s.constraints {
		if !c.Evaluate(machine).Satisfied {
			return false
		}
	}
	return true
}

// ScheduleContainer finds the best eligible machine for the next container, reserves resources on it,
// and returns the machine along with the scheduling report. Returns an error if no machine can accommodate
// the container. The report explains why each ineligible machine failed constraints.
func (s *ServiceScheduler) ScheduleContainer() (*Machine, *SchedulingReport, error) {
	// Initialize heap on first call.
	if s.heap == nil {
		eligible, report, err := s.EligibleMachines()
		if err != nil {
			return nil, report, err
		}
		s.heap = &machineHeap{machines: eligible, ranker: s.ranker}
		heap.Init(s.heap)
	}

	// Re-filter the heap to remove machines that no longer satisfy constraints (e.g., out of resources).
	// Also update the report with current constraint evaluations for all machines.
	s.updateReportForCurrentState()

	s.heap.machines = filterEligible(s.heap.machines, s.evaluateConstraintsSatisfied)
	if len(s.heap.machines) == 0 {
		return nil, s.lastReport, errors.New("no eligible machines with sufficient resources")
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

	return m, s.lastReport, nil
}

// updateReportForCurrentState updates the lastReport with fresh constraint evaluations
// for all machines in the cluster, reflecting the current scheduling state.
func (s *ServiceScheduler) updateReportForCurrentState() {
	report := &SchedulingReport{}

	for _, machine := range s.state.Machines {
		eval := s.evaluateAllConstraints(machine)
		if eval.Passed() {
			report.Eligible = append(report.Eligible, machine)
		} else {
			report.Ineligible = append(report.Ineligible, eval)
		}
	}

	s.lastReport = report
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
	s.heap.machines = filterEligible(s.heap.machines, s.evaluateConstraintsSatisfied)
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

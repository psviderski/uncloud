package scheduler

import (
	"errors"

	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/pkg/api"
)

type ServiceScheduler struct {
	state       *ClusterState
	spec        api.ServiceSpec
	constraints []Constraint
}

// NewServiceScheduler creates a new ServiceScheduler with the given cluster state and service specification.
func NewServiceScheduler(state *ClusterState, spec api.ServiceSpec) *ServiceScheduler {
	constraints := constraintsFromSpec(spec)

	return &ServiceScheduler{
		state:       state,
		spec:        spec,
		constraints: constraints,
	}
}

// EligibleMachines returns a list of machines that satisfy all constraints for the next scheduled container.
func (s *ServiceScheduler) EligibleMachines() ([]*Machine, error) {
	var available []*Machine
	for _, machine := range s.state.Machines {
		if s.evaluateConstraints(machine) {
			available = append(available, machine)
		}
	}
	if len(available) == 0 {
		return nil, errors.New("no machines available that satisfy all constraints")
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

func (s *ServiceScheduler) ScheduleContainer() ([]*pb.MachineInfo, error) {
	// TODO: organise machines in a heap and supply a sort function from the strategy. Each scheduled container
	//  should update the machine and reorder it in the heap.
	return nil, errors.New("not implemented")
}

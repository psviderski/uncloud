package scheduler

import (
	"errors"

	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/pkg/api"
)

type ServiceScheduler struct {
	machines    []*Machine
	spec        api.ServiceSpec
	constraints []Constraint
}

func NewServiceScheduler(machines []*Machine, spec api.ServiceSpec, constraints []Constraint) *ServiceScheduler {
	specConstraints := constraintsFromSpec(spec)
	specConstraints = append(specConstraints, constraints...)

	return &ServiceScheduler{
		machines:    machines,
		spec:        spec,
		constraints: specConstraints,
	}
}

func (s *ServiceScheduler) AvailableMachines() ([]*Machine, error) {
	var available []*Machine
	for _, machine := range s.machines {
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
	return nil, errors.New("not implemented")
}

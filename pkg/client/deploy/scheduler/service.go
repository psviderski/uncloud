package scheduler

import (
	"context"
	"errors"
	"fmt"

	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/pkg/api"
)

type ServiceScheduler struct {
	Machines    []*Machine
	Spec        api.ServiceSpec
	Constraints []Constraint
}

func NewServiceScheduler(ctx context.Context, cli Client, spec api.ServiceSpec) (*ServiceScheduler, error) {
	machines, err := InspectMachines(ctx, cli)
	if err != nil {
		return nil, fmt.Errorf("inspect machines: %w", err)
	}

	constraints := constraintsFromSpec(spec)

	return &ServiceScheduler{
		Machines:    machines,
		Spec:        spec,
		Constraints: constraints,
	}, nil
}

func (s *ServiceScheduler) AvailableMachines() ([]*Machine, error) {
	var available []*Machine
	for _, machine := range s.Machines {
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
	for _, c := range s.Constraints {
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

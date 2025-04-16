package scheduler

import (
	"strings"

	"github.com/psviderski/uncloud/pkg/api"
)

// Constraint is the base interface for all scheduling constraints.
type Constraint interface {
	// Evaluate determines if a machine satisfies the constraint.
	Evaluate(machine *Machine) bool

	// Description returns a human-readable description of the constraint.
	Description() string
}

func constraintsFromSpec(spec api.ServiceSpec) []Constraint {
	var constraints []Constraint

	if len(spec.Placement.Machines) > 0 {
		constraints = append(constraints, &PlacementConstraint{
			Machines: spec.Placement.Machines,
		})
	}

	// TODO: inspect and add VolumeConstraint.

	return constraints
}

type PlacementConstraint struct {
	// Machines is a list of machine names or IDs where service containers are allowed to be deployed.
	// If empty, containers can be deployed to any available machine in the cluster.
	Machines []string
}

func (c *PlacementConstraint) Evaluate(machine *Machine) bool {
	for _, nameOrID := range c.Machines {
		if machine.Info.Id == nameOrID || machine.Info.Name == nameOrID {
			return true
		}
	}
	return false
}

func (c *PlacementConstraint) Description() string {
	return "Placement constraint by machines: " + strings.Join(c.Machines, ", ")
}

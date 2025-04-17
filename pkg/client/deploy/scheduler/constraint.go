package scheduler

import (
	"slices"
	"strings"

	"github.com/docker/docker/api/types/volume"
	"github.com/psviderski/uncloud/pkg/api"
)

// Constraint is the base interface for all scheduling constraints.
type Constraint interface {
	// Evaluate determines if a machine satisfies the constraint.
	Evaluate(machine *Machine) bool

	// Description returns a human-readable description of the constraint.
	Description() string
}

// constraintsFromSpec derives scheduling constraints from the service specification.
func constraintsFromSpec(spec api.ServiceSpec) []Constraint {
	var constraints []Constraint

	if len(spec.Placement.Machines) > 0 {
		constraints = append(constraints, &PlacementConstraint{
			Machines: spec.Placement.Machines,
		})
	}

	// Add a VolumesConstraint for named Docker volumes that are mounted in the container.
	var volumes []api.VolumeSpec
	for _, m := range spec.Container.VolumeMounts {
		if v, ok := spec.Volume(m.VolumeName); ok && v.Type == api.VolumeTypeVolume {
			volumes = append(volumes, v)
		}
	}
	if len(volumes) > 0 {
		constraints = append(constraints, &VolumesConstraint{
			Volumes: volumes,
		})
	}

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
	slices.Sort(c.Machines)
	return "Placement constraint by machines: " + strings.Join(c.Machines, ", ")
}

// VolumesConstraint restricts container placement to machines that have the required named Docker volumes.
type VolumesConstraint struct {
	// Volumes is a list of named Docker volumes of type api.VolumeTypeVolume that must exist on the machine.
	Volumes []api.VolumeSpec
}

// Evaluate determines if a machine has all the required volumes.
// Returns true if all required volumes exist on the machine or if there are no required volumes.
func (c *VolumesConstraint) Evaluate(machine *Machine) bool {
	if len(c.Volumes) == 0 {
		return true
	}

	for _, v := range c.Volumes {
		if v.Type != api.VolumeTypeVolume {
			continue
		}

		// TODO: should we check the volume driver to be local or any matched volume by name is ok?
		if !slices.ContainsFunc(machine.Volumes, func(vol volume.Volume) bool {
			return vol.Name == v.DockerVolumeName()
		}) {
			return false
		}
	}

	return true
}

func (c *VolumesConstraint) Description() string {
	volumeNames := make([]string, 0, len(c.Volumes))
	for _, v := range c.Volumes {
		if v.Type == api.VolumeTypeVolume {
			volumeNames = append(volumeNames, v.DockerVolumeName())
		}
	}
	slices.Sort(volumeNames)

	if len(volumeNames) == 0 {
		return "No volumes constraint"
	}

	return "Volumes: " + strings.Join(volumeNames, ", ")
}

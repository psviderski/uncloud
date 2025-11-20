package scheduler

import (
	"fmt"
	"slices"
	"strings"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/psviderski/uncloud/pkg/api"
)

// VolumeScheduler determines what missing volumes should be created and where for a multi-service deployment.
// It satisfies the following constraints:
//   - Services that share a volume must be placed on the same machine where the volume is located.
//     If the volume is located on multiple machines, services can be placed on any of them.
//   - Services must respect their individual placement constraints.
//   - If a volume already exists on a machine, it must be used instead of creating a new one.
//   - A missing volume must only be created on one machine.
type VolumeScheduler struct {
	// state is the current and planned state of machines and their resources in the cluster.
	state *ClusterState
	// serviceSpecs is a list of service specifications included in the deployment.
	serviceSpecs []api.ServiceSpec
	// volumeSpecs is a map of volume names to their specifications from the service specs in a canonical form.
	volumeSpecs map[string]api.VolumeSpec
	// volumeServices is a map of volume names to the list of service names that use the volume.
	// TODO: not all service spec may contain the service name, so use a slice of int indexes instead of names.
	volumeServices map[string][]string
	// existingVolumeMachines is a map of volume names to the set of machine IDs where those volumes are located.
	// Contains only volumes that are used by at least one service in serviceSpecs.
	existingVolumeMachines map[string]mapset.Set[string]
}

// NewVolumeScheduler creates a new VolumeScheduler with the given cluster state and service specifications.
func NewVolumeScheduler(state *ClusterState, specs []api.ServiceSpec) (*VolumeScheduler, error) {
	var specsWithVolumes []api.ServiceSpec
	// Docker volume name -> VolumeSpec.
	volumeSpecs := make(map[string]api.VolumeSpec)
	// Volume name -> list of service names that use the volume.
	volumeServices := make(map[string][]string)
	// Volume name -> set of machine IDs where the volume is located.
	existingVolumeMachines := make(map[string]mapset.Set[string])

	// Validate all service names are unique to avoid scheduling conflicts.
	serviceNames := make(map[string]struct{}, len(specs))
	for _, spec := range specs {
		if err := spec.Validate(); err != nil {
			return nil, fmt.Errorf("invalid service spec: %w", err)
		}

		if _, exists := serviceNames[spec.Name]; exists {
			return nil, fmt.Errorf("duplicate service name: '%s'", spec.Name)
		}
		serviceNames[spec.Name] = struct{}{}

		mountedVolumes := spec.MountedDockerVolumes()
		if len(mountedVolumes) == 0 {
			continue
		}
		specsWithVolumes = append(specsWithVolumes, spec)

		for _, v := range mountedVolumes {
			v = v.SetDefaults()
			// Reset any aliases in a service spec to the actual Docker volume name.
			v.Name = v.DockerVolumeName()

			if seenVolume, ok := volumeSpecs[v.Name]; ok {
				if !seenVolume.Equals(v) {
					return nil, fmt.Errorf("volume '%s' is defined multiple times with different options", v.Name)
				}
			} else {
				volumeSpecs[v.Name] = v
			}

			volumeServices[v.Name] = append(volumeServices[v.Name], spec.Name)
		}
	}

	// Validate the configurations of existing volumes on machines don't conflict with the volume specs, for example,
	// a volume and a spec with the same name don't have different drivers.
	for _, machine := range state.Machines {
		for _, vol := range machine.Volumes {
			if spec, ok := volumeSpecs[vol.Name]; ok {
				if !spec.MatchesDockerVolume(vol) {
					return nil, fmt.Errorf("volume '%s' specification does not match the existing volume "+
						"on machine '%s'", vol.Name, machine.Info.Name)
				}

				if _, setInitialised := existingVolumeMachines[vol.Name]; !setInitialised {
					existingVolumeMachines[vol.Name] = mapset.NewSet[string]()
				}
				existingVolumeMachines[vol.Name].Add(machine.Info.Id)
			}
		}
	}

	return &VolumeScheduler{
		state:                  state,
		serviceSpecs:           specsWithVolumes,
		volumeSpecs:            volumeSpecs,
		volumeServices:         volumeServices,
		existingVolumeMachines: existingVolumeMachines,
	}, nil
}

// Schedule determines what missing volumes should be created and where for services in the multi-service deployment.
// It returns a map of machine IDs to a list of api.VolumeSpec that should be created on that machine,
// or an error if services can't be scheduled due to scheduling constraints.
// It also updates the state of the machines in the cluster state to reflect the scheduled volumes.
func (s *VolumeScheduler) Schedule() (map[string][]api.VolumeSpec, error) {
	if len(s.serviceSpecs) == 0 {
		// No services with volume mounts, nothing to schedule.
		return nil, nil
	}

	// Service name -> set of machine IDs where the service can be scheduled.
	serviceEligibleMachines := make(map[string]mapset.Set[string])
	// Volume name -> list of service names that use the volume.
	// Get eligible machines for each service without considering its volume mounts.
	for _, spec := range s.serviceSpecs {
		machineIDs, err := s.serviceEligibleMachinesWithoutVolumes(spec)
		if err != nil {
			return nil, err
		}
		serviceEligibleMachines[spec.Name] = machineIDs
	}

	// For each volume that exists on any machine(s) (which shouldn't be created), intersect each service's
	// eligible machines that use the volume with the machines the volume is located on.
	//
	// Service name -> list of processed volume names (quoted) to format the error message.
	quotedServiceVolumes := make(map[string][]string)
	for volumeName, volumeMachines := range s.existingVolumeMachines {
		for _, serviceName := range s.volumeServices[volumeName] {
			quotedServiceVolumes[serviceName] = append(quotedServiceVolumes[serviceName],
				fmt.Sprintf("'%s'", volumeName))
			newEligibleMachines := serviceEligibleMachines[serviceName].Intersect(volumeMachines)
			if newEligibleMachines.Cardinality() == 0 {
				volumes := strings.Join(quotedServiceVolumes[serviceName], ", ")
				return nil, fmt.Errorf("unable to find a machine that satisfies service '%s' "+
					"placement constraints and has all required volumes: %s", serviceName, volumes)
			}
			serviceEligibleMachines[serviceName] = newEligibleMachines
		}
	}

	// Skip constraints propagation for volumes that already exist on machines as the propagation only works
	// for missing volumes.
	placedVolumes := make(map[string]struct{})
	for volumeName := range s.existingVolumeMachines {
		placedVolumes[volumeName] = struct{}{}
	}

	if err := s.propagateConstraintsUntilConvergence(serviceEligibleMachines, placedVolumes); err != nil {
		return nil, err
	}

	// Schedule each missing volume on one of its eligible machines.
	scheduledVolumes := make(map[string][]api.VolumeSpec)
	for missingVolumeName, missingVolumeSpec := range s.volumeSpecs {
		// Skip volumes that already exist on machines.
		if _, ok := s.existingVolumeMachines[missingVolumeName]; ok {
			continue
		}

		serviceNames := s.volumeServices[missingVolumeName]
		if len(serviceNames) == 0 {
			return nil, fmt.Errorf("bug detected: no services using volume '%s'", missingVolumeName)
		}

		// Get the current eligible machines (any service using the volume will have the same set after convergence).
		eligibleMachines := serviceEligibleMachines[serviceNames[0]]
		if eligibleMachines.Cardinality() == 0 {
			return nil, fmt.Errorf("bug detected: no eligible machines for volume '%s'", missingVolumeName)
		}

		// Choose the first machine in the sorted eligible machines to schedule the volume on.
		// Sort the eligible machines to ensure deterministic behavior.
		sortedEligibleMachines := eligibleMachines.ToSlice()
		slices.Sort(sortedEligibleMachines)
		machineID := sortedEligibleMachines[0]
		// Update constraints for all services that use this volume to be placed on the selected machine.
		for _, serviceName := range serviceNames {
			serviceEligibleMachines[serviceName] = mapset.NewSet(machineID)
		}
		placedVolumes[missingVolumeName] = struct{}{}
		scheduledVolumes[machineID] = append(scheduledVolumes[machineID], missingVolumeSpec)

		// Propagate the updated constraints.
		if err := s.propagateConstraintsUntilConvergence(serviceEligibleMachines, placedVolumes); err != nil {
			return nil, fmt.Errorf("unexpected error while propagating constraints after "+
				"scheduling volume '%s' on machine '%s': %w", missingVolumeName, machineID, err)
		}
	}

	// Update the state of the machines with the scheduled volumes.
	for machineID, volumes := range scheduledVolumes {
		for _, m := range s.state.Machines {
			if m.Info.Id == machineID {
				m.ScheduledVolumes = append(m.ScheduledVolumes, volumes...)
				break
			}
		}
	}

	return scheduledVolumes, nil
}

// serviceEligibleMachinesWithoutVolumes returns a set of machine IDs where the service can be scheduled
// without considering its volume mounts.
func (s *VolumeScheduler) serviceEligibleMachinesWithoutVolumes(spec api.ServiceSpec) (mapset.Set[string], error) {
	specWithoutVolumes := spec.Clone()
	specWithoutVolumes.Container.VolumeMounts = nil

	scheduler := NewServiceScheduler(s.state, specWithoutVolumes)
	machines, err := scheduler.EligibleMachines()
	if err != nil {
		return nil, fmt.Errorf("schedule service '%s': %w", spec.Name, err)
	}

	machineIDs := mapset.NewSetWithSize[string](len(machines))
	for _, m := range machines {
		machineIDs.Add(m.Info.Id)
	}

	return machineIDs, nil
}

// propagateConstraintsUntilConvergence iteratively narrows down eligible machines for services by propagating
// constraints through shared volumes until convergence. It only processes volumes that need to be created (not
// existing volumes) and ensures services sharing a volume converge to the same set of eligible machines.
// If skipVolumes is provided, those volumes are excluded from constraint propagation.
// Returns an error if any services have no eligible machines after constraint propagation.
func (s *VolumeScheduler) propagateConstraintsUntilConvergence(
	serviceEligibleMachines map[string]mapset.Set[string],
	skipVolumes map[string]struct{},
) error {
	changed := true
	// Loop until no more changes occur.
	for changed {
		changed = false

		// For each volume, find the intersection of eligible machines for all services using that volume and update
		// their eligible machines with the intersection. This will narrow down the machines where the volume can
		// be created.
		for volumeName, serviceNames := range s.volumeServices {
			if skipVolumes != nil {
				if _, ok := skipVolumes[volumeName]; ok {
					continue
				}
			}
			// Skip if there are no services using this volume (shouldn't happen).
			if len(serviceNames) == 0 {
				continue
			}

			// Find the intersection of eligible machines for all services using this volume.
			var eligibleMachinesForVolume mapset.Set[string]
			first := true
			for _, serviceName := range serviceNames {
				if first {
					// First service: initialise the intersection.
					eligibleMachinesForVolume = serviceEligibleMachines[serviceName].Clone()
					first = false
				} else {
					eligibleMachinesForVolume = serviceEligibleMachines[serviceName].Intersect(
						eligibleMachinesForVolume)
				}
			}

			// If no machines are eligible for this volume, we have a constraint violation.
			//goland:noinspection GoDfaNilDereference
			if eligibleMachinesForVolume.Cardinality() == 0 {
				var quotedServiceNames []string // Used to format the error message.
				for _, svcName := range serviceNames {
					quotedServiceNames = append(quotedServiceNames, fmt.Sprintf("'%s'", svcName))
				}
				return fmt.Errorf("unable to find a machine that satisfies placement constraints "+
					"for services %s that must be placed together to share volume '%s'",
					strings.Join(quotedServiceNames, ", "), volumeName)
			}

			// Update eligible machines for all services using this volume.
			newCount := eligibleMachinesForVolume.Cardinality()
			for _, serviceName := range serviceNames {
				oldCount := serviceEligibleMachines[serviceName].Cardinality()
				serviceEligibleMachines[serviceName] = eligibleMachinesForVolume

				if oldCount != newCount {
					changed = true
				}
			}
		}
	}

	return nil
}

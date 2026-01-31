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
//   - Volumes used by global services will be created on all eligible machines.
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
						"on machine '%s'. Use a different volume name or adjust the volume options to match "+
						"the existing volume. You can also remove the existing volume from the machine(s) with "+
						"'uc volume rm' (WARNING: the data will be lost) and run the deployment again to create "+
						"a new volume with the correct specification", vol.Name, machine.Info.Name)
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

	// For each volume that exists on any machine(s), intersect each non-global service's
	// eligible machines that use the volume with the machines the volume is located on.
	// Global services skip this constraint as they need the volume on ALL eligible machines,
	// and the volume will be created on machines that don't have it.
	//
	// Service name -> list of processed volume names (quoted) to format the error message.
	quotedServiceVolumes := make(map[string][]string)
	for volumeName, volumeMachines := range s.existingVolumeMachines {
		// Skip constraint narrowing for global services - they don't need to be constrained
		// to machines that already have the volume.
		if s.isVolumeForGlobalService(volumeName) {
			continue
		}

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

	// Skip constraints propagation for:
	// 1. Volumes that already exist on machines (for replicated services) as the propagation only works
	//    for missing volumes. Global service volumes are NOT marked as placed here since they still need
	//    to be scheduled on machines that don't have them.
	// 2. Volumes only used by global services - these need UNION of eligible machines, not intersection.
	placedVolumes := make(map[string]struct{})
	for volumeName := range s.existingVolumeMachines {
		if !s.isVolumeForGlobalService(volumeName) {
			placedVolumes[volumeName] = struct{}{}
		}
	}
	// Skip constraint propagation for volumes used by global services.
	// Also check for invalid configuration: volume shared between global and replicated services.
	for volumeName := range s.volumeSpecs {
		if s.isVolumeSharedBetweenGlobalAndReplicated(volumeName) {
			return nil, fmt.Errorf("volume '%s' cannot be shared between global and replicated services: "+
				"global services require the volume on all machines while replicated services require "+
				"co-location with the volume", volumeName)
		}
		if s.isVolumeForGlobalService(volumeName) {
			placedVolumes[volumeName] = struct{}{}
		}
	}

	if err := s.propagateConstraintsUntilConvergence(serviceEligibleMachines, placedVolumes); err != nil {
		return nil, err
	}

	// Schedule each missing volume on eligible machines.
	// For global services: schedule on ALL eligible machines that don't already have the volume.
	// For replicated services: schedule on ONE eligible machine (skip if volume exists anywhere).
	scheduledVolumes := make(map[string][]api.VolumeSpec)
	for volumeName, volumeSpec := range s.volumeSpecs {
		existingMachines := s.existingVolumeMachines[volumeName]
		serviceNames := s.volumeServices[volumeName]
		if len(serviceNames) == 0 {
			return nil, fmt.Errorf("bug detected: no services using volume '%s'", volumeName)
		}

		// Get the eligible machines for this volume.
		// For volumes used by global services: compute UNION of all services' eligible machines.
		// For other volumes: any service will have the same set after constraint convergence.
		var eligibleMachines mapset.Set[string]
		if s.isVolumeForGlobalService(volumeName) {
			// Compute union of eligible machines for all global services using this volume.
			eligibleMachines = mapset.NewSet[string]()
			for _, serviceName := range serviceNames {
				eligibleMachines = eligibleMachines.Union(serviceEligibleMachines[serviceName])
			}
		} else {
			eligibleMachines = serviceEligibleMachines[serviceNames[0]]
		}
		if eligibleMachines.Cardinality() == 0 {
			return nil, fmt.Errorf("bug detected: no eligible machines for volume '%s'", volumeName)
		}

		// Sort the eligible machines to ensure deterministic behavior.
		sortedEligibleMachines := eligibleMachines.ToSlice()
		slices.Sort(sortedEligibleMachines)

		if s.isVolumeForGlobalService(volumeName) {
			// Global service: schedule volume on eligible machines that don't already have it.
			for _, machineID := range sortedEligibleMachines {
				if existingMachines != nil && existingMachines.Contains(machineID) {
					// Volume already exists on this machine, skip it.
					continue
				}
				scheduledVolumes[machineID] = append(scheduledVolumes[machineID], volumeSpec)
			}
			// Mark volume as placed - no constraint propagation needed since volume will be on all machines.
			placedVolumes[volumeName] = struct{}{}
		} else {
			// Replicated service: skip if volume already exists on any machine (services will use that location).
			if existingMachines != nil && existingMachines.Cardinality() > 0 {
				continue
			}

			// Schedule volume on ONE machine (first in sorted order).
			machineID := sortedEligibleMachines[0]
			// Update constraints for all services that use this volume to be placed on the selected machine.
			for _, serviceName := range serviceNames {
				serviceEligibleMachines[serviceName] = mapset.NewSet(machineID)
			}
			placedVolumes[volumeName] = struct{}{}
			scheduledVolumes[machineID] = append(scheduledVolumes[machineID], volumeSpec)

			// Propagate the updated constraints.
			if err := s.propagateConstraintsUntilConvergence(serviceEligibleMachines, placedVolumes); err != nil {
				return nil, fmt.Errorf("unexpected error while propagating constraints after "+
					"scheduling volume '%s' on machine '%s': %w", volumeName, machineID, err)
			}
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

// isVolumeForGlobalService returns true if any service using this volume is a global service.
func (s *VolumeScheduler) isVolumeForGlobalService(volumeName string) bool {
	serviceNames := s.volumeServices[volumeName]
	for _, serviceName := range serviceNames {
		for _, spec := range s.serviceSpecs {
			if spec.Name == serviceName && spec.Mode == api.ServiceModeGlobal {
				return true
			}
		}
	}
	return false
}

// isVolumeSharedBetweenGlobalAndReplicated returns true if a volume is used by both
// global and replicated services, which is an invalid configuration.
func (s *VolumeScheduler) isVolumeSharedBetweenGlobalAndReplicated(volumeName string) bool {
	serviceNames := s.volumeServices[volumeName]
	hasGlobal := false
	hasReplicated := false

	for _, serviceName := range serviceNames {
		for _, spec := range s.serviceSpecs {
			if spec.Name == serviceName {
				mode := spec.Mode
				if mode == "" {
					mode = api.ServiceModeReplicated
				}
				if mode == api.ServiceModeGlobal {
					hasGlobal = true
				} else {
					hasReplicated = true
				}
			}
		}
	}

	return hasGlobal && hasReplicated
}

package scheduler

import (
	"context"
	"fmt"
	"slices"
	"strings"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/psviderski/uncloud/pkg/api"
)

// VolumeScheduler determines what missing volumes should be created and where for a multi-service deployment.
// TODO: add rules that if a volume exists, it should be used instead of creating a new one.
type VolumeScheduler struct {
	// machines is a list of available machines in the cluster.
	machines []*Machine
	// serviceSpecs is a list of service specifications included in the deployment.
	serviceSpecs []api.ServiceSpec
	// volumeSpecs is a map of volume names to their specifications from the service specs in a canonical form.
	volumeSpecs map[string]api.VolumeSpec
	// existingVolumeMachines is a map of volume names to the set of machine IDs where those volumes are located.
	// Contains only volumes that are used by at least one service in serviceSpecs.
	existingVolumeMachines map[string]mapset.Set[string]
}

// NewVolumeSchedulerWithClient creates a new VolumeScheduler with the given cluster client and service specifications.
func NewVolumeSchedulerWithClient(ctx context.Context, cli Client, specs []api.ServiceSpec) (*VolumeScheduler, error) {
	machines, err := InspectMachines(ctx, cli)
	if err != nil {
		return nil, fmt.Errorf("inspect machines: %w", err)
	}

	return NewVolumeSchedulerWithMachines(machines, specs)
}

// NewVolumeSchedulerWithMachines creates a new VolumeScheduler with the given cluster machines
// and service specifications.
func NewVolumeSchedulerWithMachines(machines []*Machine, specs []api.ServiceSpec) (*VolumeScheduler, error) {
	// TODO: validate specs before scheduling, need to update tests to use helper functions to create specs with images.
	var specsWithVolumes []api.ServiceSpec
	// Docker volume name -> VolumeSpec.
	volumeSpecs := make(map[string]api.VolumeSpec)
	// Volume name -> set of machine IDs where the volume is located.
	volumeMachines := make(map[string]mapset.Set[string])

	// Validate all service names are unique to avoid scheduling conflicts.
	serviceNames := make(map[string]struct{}, len(specs))
	for _, spec := range specs {
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
			if seenVolume, ok := volumeSpecs[v.DockerVolumeName()]; ok {
				if !seenVolume.Equals(v) {
					return nil, fmt.Errorf("volume '%s' is defined multiple times with different options",
						v.DockerVolumeName())
				}
			} else {
				v = v.SetDefaults()
				v.Name = v.DockerVolumeName() // Reset any aliases in a service spec to the actual Docker volume name.
				volumeSpecs[v.Name] = v
			}
		}
	}

	// Validate the configurations of existing volumes on machines don't conflict with the volume specs, for example,
	// a volume and a spec with the same name don't have different drivers.
	for _, machine := range machines {
		for _, vol := range machine.Volumes {
			if spec, ok := volumeSpecs[vol.Name]; ok {
				if !spec.MatchesDockerVolume(vol) {
					return nil, fmt.Errorf("volume '%s' specification does not match the existing volume "+
						"on machine '%s'", vol.Name, machine.Info.Name)
				}

				if _, setInitialised := volumeMachines[vol.Name]; !setInitialised {
					volumeMachines[vol.Name] = mapset.NewSet[string]()
				}
				volumeMachines[vol.Name].Add(machine.Info.Id)
			}
		}
	}

	return &VolumeScheduler{
		machines:               machines,
		serviceSpecs:           specsWithVolumes,
		volumeSpecs:            volumeSpecs,
		existingVolumeMachines: volumeMachines,
	}, nil
}

// Schedule determines what missing volumes should be created and where for services in the multi-service deployment.
// It returns a map of machine IDs to a list of api.VolumeSpec that should be created on that machine,
// or an error if services can't be scheduled due to scheduling constraints.
func (s *VolumeScheduler) Schedule() (map[string][]api.VolumeSpec, error) {
	if len(s.serviceSpecs) == 0 {
		// No services with volume mounts, nothing to schedule.
		return nil, nil
	}

	// Service name -> set of machine IDs where the service can be scheduled.
	serviceEligibleMachines := make(map[string]mapset.Set[string])
	// Volume name -> list of service names that use the volume.
	volumeServices := make(map[string][]string)
	// Get eligible machines for each service without considering its volume mounts.
	for _, spec := range s.serviceSpecs {
		machineIDs, err := s.serviceEligibleMachinesWithoutVolumes(spec)
		if err != nil {
			return nil, err
		}
		serviceEligibleMachines[spec.Name] = machineIDs

		// Populate volumeServices with Docker volumes used by this service.
		for _, v := range spec.MountedDockerVolumes() {
			volumeName := v.DockerVolumeName()
			volumeServices[volumeName] = append(volumeServices[volumeName], spec.Name)
		}
	}

	// For each volume that exists on any machine(s) (which shouldn't be created), intersect each service's
	// eligible machines that use the volume with the machines the volume is located on.
	// Service name -> list of processed volume names (quoted) to format the error message.
	quotedServiceVolumes := make(map[string][]string)
	for volumeName, volumeMachines := range s.existingVolumeMachines {
		for _, serviceName := range volumeServices[volumeName] {
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

	for serviceName, eligibleMachines := range serviceEligibleMachines {
		fmt.Printf("### Service '%s' can be scheduled on machines: %v\n", serviceName, eligibleMachines.ToSlice())
	}

	// For each missing volume, intersect the eligible machines for all services using the volume
	// and choose the first machine in the sorted intersection to create the volume on.
	scheduledVolumes := make(map[string][]api.VolumeSpec)
	for missingVolumeName, missingVolumeSpec := range s.volumeSpecs {
		if _, ok := s.existingVolumeMachines[missingVolumeName]; ok {
			// This volume already exists, no need to create it.
			continue
		}

		var eligibleMachines mapset.Set[string]
		var quotedServiceNames []string // Used to format the error message.
		for i, serviceName := range volumeServices[missingVolumeName] {
			if i == 0 {
				eligibleMachines = serviceEligibleMachines[serviceName]
			} else {
				eligibleMachines = serviceEligibleMachines[serviceName].Intersect(eligibleMachines)
			}
			quotedServiceNames = append(quotedServiceNames, fmt.Sprintf("'%s'", serviceName))
		}

		if eligibleMachines == nil {
			return nil, fmt.Errorf("bug detected: no services using volume '%s'", missingVolumeName)
		}
		if eligibleMachines.Cardinality() == 0 {
			return nil, fmt.Errorf("unable to find a machine that satisfies placement constraints "+
				"for services %s that must be placed together to share volume '%s'",
				strings.Join(quotedServiceNames, ", "), missingVolumeName)
		}

		// Choose the first machine in the sorted eligible machines to create the volume on.
		// Sort the eligible machines to ensure deterministic behavior.
		// TODO: the first machine might not be the optimal one. Ideally, we need to do the intersection for all volumes
		// 	multiple times until they converge. Then picking any machine is fine.
		sortedEligibleMachines := eligibleMachines.ToSlice()
		slices.Sort(sortedEligibleMachines)
		machineID := sortedEligibleMachines[0]
		scheduledVolumes[machineID] = append(scheduledVolumes[machineID], missingVolumeSpec)
		// Update the eligible machines to the chosen machine for all services using this volume.
		eligibleMachines = mapset.NewSet(machineID)
		for _, serviceName := range volumeServices[missingVolumeName] {
			serviceEligibleMachines[serviceName] = eligibleMachines
		}
	}

	return scheduledVolumes, nil
}

// serviceEligibleMachinesWithoutVolumes returns a set of machine IDs where the service can be scheduled
// without considering its volume mounts.
func (s *VolumeScheduler) serviceEligibleMachinesWithoutVolumes(spec api.ServiceSpec) (mapset.Set[string], error) {
	specWithoutVolumes := spec.Clone()
	specWithoutVolumes.Container.VolumeMounts = nil

	scheduler := NewServiceSchedulerWithMachines(s.machines, specWithoutVolumes)
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

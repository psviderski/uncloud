package scheduler

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/psviderski/uncloud/pkg/api"
)

// volumeResourceBudget tracks combined resource needs for services sharing a volume.
type volumeResourceBudget struct {
	TotalCPU    int64    // Sum of CPU reservations (in nanocores)
	TotalMemory int64    // Sum of memory reservations (in bytes)
	Services    []string // Service names for error messages
}

// VolumeScheduler determines what missing volumes should be created and where for a multi-service deployment.
// It satisfies the following constraints:
//   - Services that share a volume must be placed on the same machine where the volume is located.
//     If the volume is located on multiple machines, services can be placed on any of them.
//   - Services must respect their individual placement constraints.
//   - If a volume already exists on a machine, it must be used instead of creating a new one.
//   - A missing volume must only be created on one machine.
//
// VolumeScheduler uses ServiceScheduler internally to determine which machines satisfy each service's
// non-volume constraints (placement, resources). This dependency is intentional: VolumeScheduler handles
// volume-to-machine assignment, while ServiceScheduler handles container-to-machine assignment.
type VolumeScheduler struct {
	// state is the current and planned state of machines and their resources in the cluster.
	state *ClusterState
	// serviceSpecs is a list of service specifications included in the deployment.
	serviceSpecs []api.ServiceSpec
	// allServiceSpecs is a list of all service specifications (including those without volumes)
	// for resource budget calculations.
	allServiceSpecs []api.ServiceSpec
	// volumeSpecs is a map of volume names to their specifications from the service specs in a canonical form.
	volumeSpecs map[string]api.VolumeSpec
	// volumeServices is a map of volume names to the list of service names that use the volume.
	// TODO: not all service spec may contain the service name, so use a slice of int indexes instead of names.
	volumeServices map[string][]string
	// existingVolumeMachines is a map of volume names to the set of machine IDs where those volumes are located.
	// Contains only volumes that are used by at least one service in serviceSpecs.
	existingVolumeMachines map[string]mapset.Set[string]
	// volumeBudgets maps volume name to combined resource requirements of services sharing that volume.
	volumeBudgets map[string]*volumeResourceBudget
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

	s := &VolumeScheduler{
		state:                  state,
		serviceSpecs:           specsWithVolumes,
		allServiceSpecs:        specs,
		volumeSpecs:            volumeSpecs,
		volumeServices:         volumeServices,
		existingVolumeMachines: existingVolumeMachines,
	}
	s.calculateVolumeBudgets()
	return s, nil
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
	serviceVolumes := make(map[string][]string) // Service name -> processed volume names for error messages.
	for volumeName, volumeMachines := range s.existingVolumeMachines {
		for _, serviceName := range s.volumeServices[volumeName] {
			serviceVolumes[serviceName] = append(serviceVolumes[serviceName], volumeName)
			newEligibleMachines := serviceEligibleMachines[serviceName].Intersect(volumeMachines)
			if newEligibleMachines.Cardinality() == 0 {
				return nil, fmt.Errorf("unable to find a machine that satisfies service '%s' "+
					"placement constraints and has all required volumes: %s",
					serviceName, strings.Join(quoteStrings(serviceVolumes[serviceName]), ", "))
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
	// Sort volumes by resource requirements (descending) so larger workloads get placed first,
	// leaving flexibility for smaller ones.
	missingVolumeNames := s.sortVolumesByResourceBudget()

	scheduledVolumes := make(map[string][]api.VolumeSpec)
	for _, missingVolumeName := range missingVolumeNames {
		missingVolumeSpec := s.volumeSpecs[missingVolumeName]

		serviceNames := s.volumeServices[missingVolumeName]
		if len(serviceNames) == 0 {
			return nil, fmt.Errorf("bug detected: no services using volume '%s'", missingVolumeName)
		}

		// Get the current eligible machines (any service using the volume will have the same set after convergence).
		eligibleMachines := serviceEligibleMachines[serviceNames[0]]
		if eligibleMachines.Cardinality() == 0 {
			return nil, fmt.Errorf("bug detected: no eligible machines for volume '%s'", missingVolumeName)
		}

		// Choose the machine with fewest scheduled volumes to spread across machines.
		machineID := s.selectMachineForVolume(eligibleMachines)
		// Update constraints for all services that use this volume to be placed on the selected machine.
		for _, serviceName := range serviceNames {
			serviceEligibleMachines[serviceName] = mapset.NewSet(machineID)
		}
		placedVolumes[missingVolumeName] = struct{}{}
		scheduledVolumes[machineID] = append(scheduledVolumes[machineID], missingVolumeSpec)

		// Update the machine's state immediately so subsequent iterations see it
		// for spreading and resource decisions.
		if machine := s.machineByID(machineID); machine != nil {
			machine.ScheduledVolumes = append(machine.ScheduledVolumes, missingVolumeSpec)
			// Reserve resources for this volume's services so subsequent volumes
			// see reduced capacity and don't overcommit the machine.
			if budget := s.volumeBudgets[missingVolumeName]; budget != nil {
				machine.ScheduledCPU += budget.TotalCPU
				machine.ScheduledMemory += budget.TotalMemory
			}
		}

		// Propagate the updated constraints.
		if err := s.propagateConstraintsUntilConvergence(serviceEligibleMachines, placedVolumes); err != nil {
			return nil, fmt.Errorf("unexpected error while propagating constraints after "+
				"scheduling volume '%s' on machine '%s': %w", missingVolumeName, machineID, err)
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
	machines, report, err := scheduler.EligibleMachines()
	if err != nil {
		return nil, fmt.Errorf("schedule service '%s':\n%s", spec.Name, report.Error())
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
	// Loop until no more changes occur.
	for {
		changed := false

		// For each volume, find the intersection of eligible machines for all services using that volume and update
		// their eligible machines with the intersection. This will narrow down the machines where the volume can
		// be created.
		for volumeName, serviceNames := range s.volumeServices {
			if _, skip := skipVolumes[volumeName]; skip || len(serviceNames) == 0 {
				continue
			}

			// Find the intersection of eligible machines for all services using this volume.
			eligibleMachinesForVolume := serviceEligibleMachines[serviceNames[0]].Clone()
			for _, serviceName := range serviceNames[1:] {
				eligibleMachinesForVolume = eligibleMachinesForVolume.Intersect(
					serviceEligibleMachines[serviceName])
			}

			// If no machines are eligible for this volume, we have a constraint violation.
			if eligibleMachinesForVolume.Cardinality() == 0 {
				quotedServiceNames := quoteStrings(serviceNames)
				return fmt.Errorf("unable to find a machine that satisfies placement constraints "+
					"for services %s that must be placed together to share volume '%s'",
					strings.Join(quotedServiceNames, ", "), volumeName)
			}

			// Filter by resource budget: ensure the machine can fit all services sharing this volume.
			if budget, ok := s.volumeBudgets[volumeName]; ok {
				eligibleMachinesForVolume = s.filterByResourceBudget(eligibleMachinesForVolume, budget)
				if eligibleMachinesForVolume.Cardinality() == 0 {
					return errors.New(s.formatResourceBudgetError(volumeName, budget))
				}
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

		if !changed {
			return nil
		}
	}
}

// calculateVolumeBudgets computes the combined resource requirements for each volume
// based on all services that share it.
func (s *VolumeScheduler) calculateVolumeBudgets() {
	s.volumeBudgets = make(map[string]*volumeResourceBudget)

	for volumeName, serviceNames := range s.volumeServices {
		budget := &volumeResourceBudget{
			Services: serviceNames,
		}

		for _, serviceName := range serviceNames {
			spec := s.serviceSpecByName(serviceName)
			if spec == nil {
				continue
			}

			replicaCount := s.effectiveReplicaCount(*spec)
			resources := spec.Container.Resources
			budget.TotalCPU += resources.CPUReservation * int64(replicaCount)
			budget.TotalMemory += resources.MemoryReservation * int64(replicaCount)
		}

		s.volumeBudgets[volumeName] = budget
	}
}

// effectiveReplicaCount returns the number of replicas that will run on the volume's machine.
// For global services, only 1 replica runs per machine, so we count it as 1.
// For replicated services, all replicas run on the same machine (where the volume is).
func (s *VolumeScheduler) effectiveReplicaCount(spec api.ServiceSpec) int {
	if spec.Mode == api.ServiceModeGlobal {
		return 1
	}
	// For replicated mode (or default), use the replica count.
	// SetDefaults() ensures Replicas is at least 1 for replicated mode.
	spec = spec.SetDefaults()
	return int(spec.Replicas)
}

// serviceSpecByName returns a pointer to the service spec with the given name, or nil if not found.
func (s *VolumeScheduler) serviceSpecByName(name string) *api.ServiceSpec {
	for i := range s.allServiceSpecs {
		if s.allServiceSpecs[i].Name == name {
			return &s.allServiceSpecs[i]
		}
	}
	return nil
}

// filterByResourceBudget removes machines from the set that cannot fit the given resource budget.
func (s *VolumeScheduler) filterByResourceBudget(
	machines mapset.Set[string],
	budget *volumeResourceBudget,
) mapset.Set[string] {
	if budget == nil || (budget.TotalCPU == 0 && budget.TotalMemory == 0) {
		return machines
	}

	result := mapset.NewSet[string]()
	for _, machineID := range machines.ToSlice() {
		machine := s.machineByID(machineID)
		if machine == nil {
			continue
		}

		// Check if machine has sufficient resources for the combined budget.
		availableCPU := machine.AvailableCPU()
		availableMemory := machine.AvailableMemory()

		if availableCPU >= budget.TotalCPU && availableMemory >= budget.TotalMemory {
			result.Add(machineID)
		}
	}

	return result
}

// machineByID returns the machine with the given ID, or nil if not found.
func (s *VolumeScheduler) machineByID(id string) *Machine {
	for _, m := range s.state.Machines {
		if m.Info.Id == id {
			return m
		}
	}
	return nil
}

// formatResourceBudgetError creates a detailed error message for resource budget failures.
func (s *VolumeScheduler) formatResourceBudgetError(volumeName string, budget *volumeResourceBudget) string {
	cpuCores := float64(budget.TotalCPU) / 1e9
	memoryGB := float64(budget.TotalMemory) / 1e9

	return fmt.Sprintf("insufficient resources for services %s sharing volume '%s': "+
		"need %.2f CPU cores and %.2f GB memory combined",
		strings.Join(quoteStrings(budget.Services), ", "), volumeName, cpuCores, memoryGB)
}

// sortVolumesByResourceBudget returns missing volume names sorted by resource requirements (descending).
// Volumes with larger CPU requirements are scheduled first, with memory as tiebreaker,
// then volume name for determinism. This ensures larger workloads get placed first,
// leaving flexibility for smaller ones.
func (s *VolumeScheduler) sortVolumesByResourceBudget() []string {
	var names []string
	for name := range s.volumeSpecs {
		// Skip volumes that already exist.
		if _, exists := s.existingVolumeMachines[name]; !exists {
			names = append(names, name)
		}
	}

	slices.SortFunc(names, func(a, b string) int {
		budgetA := s.volumeBudgets[a]
		budgetB := s.volumeBudgets[b]

		// Nil budgets (no resources) sort last.
		if budgetA == nil && budgetB == nil {
			return cmp.Compare(a, b)
		}
		if budgetA == nil {
			return 1
		}
		if budgetB == nil {
			return -1
		}

		// Sort by CPU descending, then memory descending, then name ascending for determinism.
		if c := cmp.Compare(budgetB.TotalCPU, budgetA.TotalCPU); c != 0 {
			return c
		}
		if c := cmp.Compare(budgetB.TotalMemory, budgetA.TotalMemory); c != 0 {
			return c
		}
		return cmp.Compare(a, b)
	})

	return names
}

// quoteStrings wraps each string in single quotes.
func quoteStrings(strs []string) []string {
	quoted := make([]string, len(strs))
	for i, s := range strs {
		quoted[i] = fmt.Sprintf("'%s'", s)
	}
	return quoted
}

// selectMachineForVolume picks the best machine for a volume by spreading volumes across machines.
// It prefers machines with fewer scheduled volumes to distribute load evenly.
// Uses machine ID as a tiebreaker for deterministic behavior.
func (s *VolumeScheduler) selectMachineForVolume(machines mapset.Set[string]) string {
	var bestMachine string
	bestVolumeCount := -1

	for _, machineID := range machines.ToSlice() {
		machine := s.machineByID(machineID)
		if machine == nil {
			continue
		}

		volumeCount := len(machine.ScheduledVolumes)

		// Pick machine with fewest volumes, using machine ID as tiebreaker for determinism.
		if bestMachine == "" || volumeCount < bestVolumeCount ||
			(volumeCount == bestVolumeCount && machineID < bestMachine) {
			bestVolumeCount = volumeCount
			bestMachine = machineID
		}
	}

	return bestMachine
}

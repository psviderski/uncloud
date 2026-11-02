package deploy

import (
	"maps"
	"sort"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/psviderski/uncloud/pkg/api"
)

type ContainerSpecStatus string

const (
	ContainerUpToDate ContainerSpecStatus = "up-to-date"
	// ContainerNeedsUpdate means the container can be updated in place without recreating it.
	ContainerNeedsUpdate   ContainerSpecStatus = "needs-update"
	ContainerNeedsRecreate ContainerSpecStatus = "needs-recreate"
)

func EvalContainerSpecChange(current api.ServiceSpec, new api.ServiceSpec) ContainerSpecStatus {
	current = current.SetDefaults()
	new = new.SetDefaults()

	if current.Mode != new.Mode {
		return ContainerNeedsRecreate
	}
	if current.Name != new.Name {
		return ContainerNeedsRecreate
	}

	// If pull policy is set to always, the container needs to be recreated.
	if new.Container.PullPolicy == api.PullPolicyAlways {
		return ContainerNeedsRecreate
	}
	new.Container.PullPolicy = current.Container.PullPolicy

	// Save mutable container resources that can be updated without recreation.
	newResources := new.Container.Resources
	// Temporarily set mutable container resources to current values to check if other properties changed.
	new.Container.Resources = current.Container.Resources

	// Check if immutable container properties changed.
	if !current.Container.Equals(new.Container) {
		return ContainerNeedsRecreate
	}

	if !cmp.Equal(current.Placement, new.Placement, cmpopts.EquateEmpty()) {
		// TODO: this could be just an in-place spec update when available.
		return ContainerNeedsRecreate
	}

	// TODO: change ports check to ContainerNeedsUpdate when ingress ports are stored only in the machine DB instead
	//  of as labels ans synced to the cluster store. Host ports changes should be handled as ContainerNeedsRecreate.
	if !api.PortsEqual(current.Ports, new.Ports) {
		return ContainerNeedsRecreate
	}

	// Compare volumes.
	if len(current.Volumes) != len(new.Volumes) {
		return ContainerNeedsRecreate
	}
	sortVolumes(current.Volumes)
	sortVolumes(new.Volumes)
	for i := range current.Volumes {
		if !current.Volumes[i].Equals(new.Volumes[i]) {
			// TODO: require only spec update for cases (see TODOs in tests):
			//  * bind volumes with different CreateHostPath
			//  * changed reference name in spec but preserved original volume name
			// TODO: should defined but not used (no corresponding mounts) volumes be simply ignored?
			return ContainerNeedsRecreate
		}
	}

	// Compare configs.
	if len(current.Configs) != len(new.Configs) {
		return ContainerNeedsRecreate
	}
	sortConfigs(current.Configs)
	sortConfigs(new.Configs)
	for i := range current.Configs {
		if !current.Configs[i].Equals(new.Configs[i]) {
			return ContainerNeedsRecreate
		}
	}

	if current.Container.Resources.ValidateUpdateInPlace(newResources) != nil {
		return ContainerNeedsRecreate
	}

	// Check if any mutable properties changed.
	if !current.Caddy.Equals(new.Caddy) {
		return ContainerNeedsRecreate
	}

	// CPU and memory limits are updated on the running container. Service labels are only stored in the service spec
	// and not applied to Docker containers.
	if !current.Container.Resources.Equals(newResources) ||
		!cmp.Equal(current.Labels, new.Labels, cmpopts.EquateEmpty()) {
		return ContainerNeedsUpdate
	}

	return ContainerUpToDate
}

func sortVolumes(volumes []api.VolumeSpec) {
	sort.Slice(volumes, func(i, j int) bool {
		return volumes[i].Name < volumes[j].Name
	})
}

func sortConfigs(configs []api.ConfigSpec) {
	sort.Slice(configs, func(i, j int) bool {
		return configs[i].Name < configs[j].Name
	})
}

// EvalContainerChange evaluates whether the running container matches the new spec, can be updated in place to
// match it, or needs to be recreated. Unlike EvalContainerSpecChange, it also compares the new spec with the CPU and
// memory limits that Docker applies to the container, so the container is updated when they drifted from its spec.
func EvalContainerChange(current api.ServiceContainer, new api.ServiceSpec) ContainerSpecStatus {
	status := EvalContainerSpecChange(current.ServiceSpec, new)
	if status == ContainerNeedsRecreate {
		return status
	}

	applied := current.AppliedResources()
	newResources := new.Container.Resources
	if applied.ValidateUpdateInPlace(newResources) != nil {
		return ContainerNeedsRecreate
	}
	// Only CPU and memory limits can differ after the in-place update check above.
	if !applied.Equals(newResources) {
		return ContainerNeedsUpdate
	}
	return status
}

// ContainerUpdateOptions returns the container properties that can be updated in place and differ between the new
// spec and either the container spec or the limits Docker applies to the container. It should only be used when
// EvalContainerChange returns ContainerNeedsUpdate.
func ContainerUpdateOptions(current api.ServiceContainer, new api.ServiceSpec) api.UpdateContainerOptions {
	var opts api.UpdateContainerOptions

	stored, applied := current.ServiceSpec.Container.Resources, current.AppliedResources()
	res := new.Container.Resources
	if res.CPU != stored.CPU || res.CPU != applied.CPU {
		opts.CPU = &res.CPU
	}
	if res.Memory != stored.Memory || res.Memory != applied.Memory {
		opts.Memory = &res.Memory
	}
	if res.MemoryReservation != stored.MemoryReservation || res.MemoryReservation != applied.MemoryReservation {
		opts.MemoryReservation = &res.MemoryReservation
	}

	if !cmp.Equal(current.ServiceSpec.Labels, new.Labels, cmpopts.EquateEmpty()) {
		labels := maps.Clone(new.Labels)
		if labels == nil {
			labels = map[string]string{}
		}
		opts.ServiceLabels = &labels
	}

	return opts
}

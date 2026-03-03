package deploy

import (
	"reflect"
	"sort"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/psviderski/uncloud/pkg/api"
)

type ContainerSpecStatus string

const (
	ContainerUpToDate      ContainerSpecStatus = "up-to-date"
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

	// Device reservations and mappings are immutable, so we'll need to recreate if any have changed
	if !reflect.DeepEqual(current.Container.Resources.DeviceReservations, newResources.DeviceReservations) {
		return ContainerNeedsRecreate
	}
	if !reflect.DeepEqual(current.Container.Resources.Devices, newResources.Devices) {
		return ContainerNeedsRecreate
	}
	// Ulimits are immutable, so we'll need to recreate if any have changed.
	if !reflect.DeepEqual(current.Container.Resources.Ulimits, newResources.Ulimits) {
		return ContainerNeedsRecreate
	}

	// Check if any mutable properties changed.
	if !current.Caddy.Equals(new.Caddy) {
		return ContainerNeedsRecreate
	}

	// Remaining resources are mutable.
	if !reflect.DeepEqual(current.Container.Resources, newResources) {
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

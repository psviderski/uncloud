package deploy

import (
	"sort"

	"github.com/psviderski/uncloud/pkg/api"
)

type ContainerSpecStatus string

const ContainerUpToDate ContainerSpecStatus = "up-to-date"
const ContainerNeedsUpdate ContainerSpecStatus = "needs-update"
const ContainerNeedsRecreate ContainerSpecStatus = "needs-recreate"

func EvalContainerSpecChange(current api.ServiceSpec, new api.ServiceSpec) ContainerSpecStatus {
	current = current.SetDefaults()
	new = new.SetDefaults()

	// Pull policy doesn't affect the container configuration.
	new.Container.PullPolicy = current.Container.PullPolicy
	if !current.Container.Equals(new.Container) {
		return ContainerNeedsRecreate
	}

	if current.Mode != new.Mode {
		return ContainerNeedsRecreate
	}
	if current.Name != new.Name {
		return ContainerNeedsRecreate
	}

	// TODO: compare mutable properties such as memory or CPU limits when they are implemented.

	// TODO: change ports check to ContainerNeedsUpdate when ingress ports are stored only the machine DB instead
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

	return ContainerUpToDate
}

func sortVolumes(volumes []api.VolumeSpec) {
	sort.Slice(volumes, func(i, j int) bool {
		return volumes[i].Name < volumes[j].Name
	})
}

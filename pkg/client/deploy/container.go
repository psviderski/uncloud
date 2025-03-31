package deploy

import (
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

	return ContainerUpToDate
}

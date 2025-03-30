package deploy

import (
	"fmt"

	"github.com/psviderski/uncloud/pkg/api"
)

type ContainerSpecStatus string

const ContainerUpToDate ContainerSpecStatus = "up-to-date"
const ContainerNeedsUpdate ContainerSpecStatus = "needs-update"
const ContainerNeedsRecreate ContainerSpecStatus = "needs-recreate"

func CompareContainerToSpec(ctr api.ServiceContainer, spec api.ServiceSpec) (ContainerSpecStatus, error) {
	// TODO: replace the hash comparison with a more detailed comparison of ctr.ServiceSpec and spec.
	specHash, err := spec.ImmutableHash()
	if err != nil {
		return "", fmt.Errorf("calculate immutable hash for service spec: %w", err)
	}

	// Is the hash label is unset, there is no easy way to compare its configuration with the spec,
	// so let's recreate as well.
	if ctr.Config.Labels[api.LabelServiceSpecHash] != specHash {
		return ContainerNeedsRecreate, nil
	}

	// TODO: compare mutable properties such as memory or CPU limits when they are implemented.

	// TODO: remove ports check when ports are stored in the local machine store instead of as labels.
	ports, err := ctr.ServicePorts()
	if err != nil {
		return "", fmt.Errorf("get service ports: %w", err)
	}

	if !api.PortsEqual(ports, spec.Ports) {
		return ContainerNeedsRecreate, nil
	}

	return ContainerUpToDate, nil
}

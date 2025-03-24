package deploy

import (
	"fmt"
	"github.com/psviderski/uncloud/pkg/api"
)

type ContainerSpecStatus string

const ContainerUpToDate ContainerSpecStatus = "up-to-date"
const ContainerNeedsUpdate ContainerSpecStatus = "needs-update"
const ContainerNeedsRecreate ContainerSpecStatus = "needs-recreate"

func CompareContainerToSpec(ctr api.Container, spec api.ServiceSpec) (ContainerSpecStatus, error) {
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
	
	// TODO: compare ports

	return ContainerUpToDate, nil
}

package e2e

import (
	"testing"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client/deploy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func assertServiceMatchesSpec(t *testing.T, svc api.Service, spec api.ServiceSpec) {
	assert.Equal(t, spec.Name, svc.Name)

	if svc.Mode == api.ServiceModeReplicated {
		assert.Contains(t, []string{"", api.ServiceModeReplicated}, spec.Mode)
		assert.Len(t, svc.Containers, int(spec.Replicas), "Expected %d replicas", spec.Replicas)
	} else {
		assert.Equal(t, spec.Mode, svc.Mode)
	}

	for _, mc := range svc.Containers {
		assertContainerMatchesSpec(t, mc.Container, spec)
	}
}

func assertContainerMatchesSpec(t *testing.T, ctr api.ServiceContainer, spec api.ServiceSpec) {
	status, err := deploy.CompareContainerToSpec(ctr, spec)
	require.NoError(t, err)
	assert.Equal(t, deploy.ContainerUpToDate, status)
}

// serviceContainersByMachine returns a map of machine ID to service containers on that machine.
func serviceContainersByMachine(t *testing.T, svc api.Service) map[string][]api.ServiceContainer {
	containers := make(map[string][]api.ServiceContainer)
	for _, c := range svc.Containers {
		containers[c.MachineID] = append(containers[c.MachineID], c.Container)
	}
	return containers
}

func serviceMachines(t *testing.T, svc api.Service) mapset.Set[string] {
	machines := mapset.NewSet[string]()
	for _, c := range svc.Containers {
		machines.Add(c.MachineID)
	}

	return machines
}

func serviceContainerIDs(t *testing.T, svc api.Service) mapset.Set[string] {
	ids := mapset.NewSet[string]()
	for _, c := range svc.Containers {
		ids.Add(c.Container.ID)
	}
	return ids
}

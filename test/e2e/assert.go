package e2e

import (
	"strconv"
	"testing"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	machinedocker "github.com/psviderski/uncloud/internal/machine/docker"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client/deploy"
	"github.com/stretchr/testify/assert"
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
	status := deploy.EvalContainerSpecChange(ctr.ServiceSpec, spec)
	assert.Equal(t, deploy.ContainerUpToDate, status)

	spec = spec.SetDefaults()
	// Verify labels.
	assert.Equal(t, spec.Name, ctr.Config.Labels[api.LabelServiceName])
	assert.Equal(t, spec.Mode, ctr.Config.Labels[api.LabelServiceMode])
	assert.Contains(t, ctr.Config.Labels, api.LabelManaged)

	// Command and Entrypoint can only be compared if they are set in the spec.
	// Otherwise, the container takes them from the image.
	if spec.Container.Command != nil {
		assert.EqualValues(t, spec.Container.Command, ctr.Config.Cmd)
	}
	if spec.Container.Entrypoint != nil {
		assert.EqualValues(t, spec.Container.Entrypoint, ctr.Config.Entrypoint)
	}

	assert.Equal(t, spec.Container.Image, ctr.Config.Image)
	assert.Equal(t, spec.Container.Init, ctr.HostConfig.Init)
	assert.ElementsMatch(t, spec.Container.Volumes, ctr.HostConfig.Binds)

	// Compare host ports.
	portBindings := make(nat.PortMap)
	for _, p := range spec.Ports {
		if p.Mode != api.PortModeHost {
			continue
		}

		port, err := nat.NewPort(p.Protocol, strconv.Itoa(int(p.ContainerPort)))
		assert.NoError(t, err)

		binding := nat.PortBinding{HostPort: strconv.Itoa(int(p.PublishedPort))}
		if p.HostIP.IsValid() {
			binding.HostIP = p.HostIP.String()
		}
		portBindings[port] = append(portBindings[port], binding)
	}
	assert.Equal(t, portBindings, ctr.HostConfig.PortBindings)

	assert.Equal(t, container.RestartPolicy{
		Name:              container.RestartPolicyAlways,
		MaximumRetryCount: 0,
	}, ctr.HostConfig.RestartPolicy)

	// Verify network settings.
	assert.Len(t, ctr.NetworkSettings.Networks, 1)
	assert.Contains(t, ctr.NetworkSettings.Networks, machinedocker.NetworkName)
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

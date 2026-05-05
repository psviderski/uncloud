package client

import (
	"bytes"
	"testing"

	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	machinedocker "github.com/psviderski/uncloud/internal/machine/docker"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServicesFromMachineContainers(t *testing.T) {
	var warnings bytes.Buffer
	servicesByID, err := servicesFromMachineContainers(
		[]machinedocker.MachineServiceContainers{
			{
				Metadata: &pb.Metadata{Machine: "10.0.0.1"},
				Containers: []api.ServiceContainer{
					testServiceContainer("web-1", "svc-web", "web", api.ServiceModeReplicated, false),
					testServiceContainer("api-1", "svc-api", "api", api.ServiceModeGlobal, false),
				},
				HookContainers: []api.ServiceContainer{
					testServiceContainer("web-hook-1", "svc-web", "web", api.ServiceModeReplicated, true),
				},
			},
			{
				Metadata: &pb.Metadata{Machine: "10.0.0.2"},
				Containers: []api.ServiceContainer{
					testServiceContainer("web-2", "svc-web", "web", api.ServiceModeReplicated, false),
					testServiceContainer("web-alt-1", "svc-web-alt", "web", api.ServiceModeReplicated, false),
				},
			},
			{
				Metadata: &pb.Metadata{Machine: "10.0.0.3", Error: "unavailable"},
			},
		},
		map[string]string{
			"10.0.0.1": "machine-1",
			"10.0.0.2": "machine-2",
			"10.0.0.3": "machine-3",
		},
		&warnings,
	)
	require.NoError(t, err)

	require.Len(t, servicesByID, 3)

	web := servicesByID["svc-web"]
	assert.Equal(t, "web", web.Name)
	assert.Equal(t, api.ServiceModeReplicated, web.Mode)
	require.Len(t, web.Containers, 2)
	assert.Equal(t, "machine-1", web.Containers[0].MachineID)
	assert.Equal(t, "machine-2", web.Containers[1].MachineID)
	require.Len(t, web.HookContainers, 1)
	assert.Equal(t, "web-hook-1", web.HookContainers[0].Container.ID)

	apiSvc := servicesByID["svc-api"]
	assert.Equal(t, "api", apiSvc.Name)
	assert.Equal(t, api.ServiceModeGlobal, apiSvc.Mode)
	require.Len(t, apiSvc.Containers, 1)
	assert.Equal(t, "machine-1", apiSvc.Containers[0].MachineID)

	duplicateName := servicesByID["svc-web-alt"]
	assert.Equal(t, "web", duplicateName.Name)
	require.Len(t, duplicateName.Containers, 1)

	assert.Contains(t, warnings.String(), "WARNING: failed to list containers on machine '10.0.0.3': unavailable")
}

func TestServicesFromMachineContainersNilMetadataWithMultipleMachines(t *testing.T) {
	_, err := servicesFromMachineContainers(
		[]machinedocker.MachineServiceContainers{
			{Metadata: nil},
			{Metadata: &pb.Metadata{Machine: "10.0.0.2"}},
		},
		map[string]string{"10.0.0.2": "machine-2"},
		nil,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "metadata is missing")
}

func testServiceContainer(id, serviceID, serviceName, mode string, hook bool) api.ServiceContainer {
	labels := map[string]string{
		api.LabelServiceID:   serviceID,
		api.LabelServiceName: serviceName,
	}
	if hook {
		labels[api.LabelHook] = api.LabelHookPreDeploy
	}

	return api.ServiceContainer{
		Container: api.Container{
			InspectResponse: dockercontainer.InspectResponse{
				ContainerJSONBase: &dockercontainer.ContainerJSONBase{
					ID:   id,
					Name: id,
				},
				Config: &dockercontainer.Config{
					Image:  "nginx:latest",
					Labels: labels,
				},
			},
		},
		ServiceSpec: api.ServiceSpec{
			Name: serviceName,
			Mode: mode,
			Container: api.ContainerSpec{
				Image: "nginx:latest",
			},
		},
	}
}

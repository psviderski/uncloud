package e2e

import (
	"context"
	"errors"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/netip"
	"strings"
	"testing"
	"uncloud/internal/api"
	"uncloud/internal/cli/client"
	"uncloud/internal/machine/api/pb"
	machinedocker "uncloud/internal/machine/docker"
	"uncloud/internal/secret"
	"uncloud/internal/ucind"
)

func newServiceID() string {
	id, err := secret.NewID()
	if err != nil {
		panic(err)
	}
	return id
}

func TestDeployment(t *testing.T) {
	t.Parallel()

	clusterName := "ucind-test.deployment"
	ctx := context.Background()
	c, _ := createTestCluster(t, clusterName, ucind.CreateClusterOptions{Machines: 3}, true)

	cli, err := c.Machines[0].Connect(ctx)
	require.NoError(t, err)

	t.Run("global", func(t *testing.T) {
		t.Parallel()

		name := "global-deployment"
		t.Cleanup(func() {
			err := cli.RemoveService(ctx, name)
			if !errors.Is(err, client.ErrNotFound) {
				require.NoError(t, err)
			}

			_, err = cli.InspectService(ctx, name)
			require.ErrorIs(t, err, client.ErrNotFound)
		})

		spec := api.ServiceSpec{
			Name: name,
			Mode: api.ServiceModeGlobal,
			Container: api.ContainerSpec{
				Image: "portainer/pause:latest",
			},
		}
		deploy, err := cli.NewDeployment(spec, nil)
		require.NoError(t, err)

		err = deploy.Validate(ctx)
		require.NoError(t, err)

		plan, err := deploy.Plan(ctx)
		require.NoError(t, err)
		assert.Len(t, plan.SequenceOperation.Operations, 3) // 3 run

		svcID, err := deploy.Run(ctx)
		require.NoError(t, err)
		assert.NotEmpty(t, svcID)

		svc, err := cli.InspectService(ctx, name)
		require.NoError(t, err)
		assert.Equal(t, name, svc.Name)
		assert.Equal(t, api.ServiceModeGlobal, svc.Mode)
		assert.Len(t, svc.Containers, 3)

		svcSpec, err := svc.Containers[0].Container.ServiceSpec()
		require.NoError(t, err)
		assert.True(t, svcSpec.Equals(spec))

		machines := make(map[string]struct{})
		for _, ctr := range svc.Containers {
			machines[ctr.MachineID] = struct{}{}
		}
		assert.Len(t, machines, 3, "expected 1 container on each machine")

		// Deploy a published port.
		specWithPort := api.ServiceSpec{
			Name: name,
			Mode: api.ServiceModeGlobal,
			Container: api.ContainerSpec{
				Image: "portainer/pause:latest",
			},
			Ports: []api.PortSpec{
				{
					PublishedPort: 8000,
					ContainerPort: 8000,
					Protocol:      api.ProtocolTCP,
					Mode:          api.PortModeHost,
				},
			},
		}
		deploy, err = cli.NewDeployment(specWithPort, nil)
		require.NoError(t, err)

		plan, err = deploy.Plan(ctx)
		require.NoError(t, err)
		assert.Len(t, plan.SequenceOperation.Operations, 6) // 3 run + 3 remove

		svcID, err = deploy.Run(ctx)
		require.NoError(t, err)
		assert.NotEmpty(t, svcID)

		svc, err = cli.InspectService(ctx, name)
		require.NoError(t, err)
		assert.Equal(t, name, svc.Name)
		assert.Equal(t, api.ServiceModeGlobal, svc.Mode)
		assert.Len(t, svc.Containers, 3)

		svcSpec, err = svc.Containers[0].Container.ServiceSpec()
		require.NoError(t, err)
		assert.True(t, svcSpec.Equals(specWithPort))

		// Deploy the same conflicting port but with container spec changes
		init := true
		specWithPortAndInit := api.ServiceSpec{
			Name: name,
			Mode: api.ServiceModeGlobal,
			Container: api.ContainerSpec{
				Image: "portainer/pause:latest",
				Init:  &init,
			},
			Ports: []api.PortSpec{
				{
					PublishedPort: 8000,
					ContainerPort: 8000,
					Protocol:      api.ProtocolTCP,
					Mode:          api.PortModeHost,
				},
			},
		}
		deploy, err = cli.NewDeployment(specWithPortAndInit, nil)
		require.NoError(t, err)

		plan, err = deploy.Plan(ctx)
		require.NoError(t, err)
		assert.Len(t, plan.SequenceOperation.Operations, 9) // 3 stop + 3 run + 3 remove

		svcID, err = deploy.Run(ctx)
		require.NoError(t, err)
		assert.NotEmpty(t, svcID)

		svc, err = cli.InspectService(ctx, name)
		require.NoError(t, err)
		assert.Equal(t, name, svc.Name)
		assert.Equal(t, api.ServiceModeGlobal, svc.Mode)
		assert.Len(t, svc.Containers, 3)

		svcSpec, err = svc.Containers[0].Container.ServiceSpec()
		require.NoError(t, err)
		assert.True(t, svcSpec.Equals(specWithPortAndInit))

		// Deploying the same spec should be a no-op.
		deploy, err = cli.NewDeployment(specWithPortAndInit, nil)
		require.NoError(t, err)

		plan, err = deploy.Plan(ctx)
		require.NoError(t, err)
		assert.Len(t, plan.SequenceOperation.Operations, 0) // no-op

		svcID, err = deploy.Run(ctx)
		require.NoError(t, err)
		assert.NotEmpty(t, svcID)

		svc, err = cli.InspectService(ctx, name)
		require.NoError(t, err)
		assert.Equal(t, name, svc.Name)
		assert.Equal(t, api.ServiceModeGlobal, svc.Mode)
		assert.Len(t, svc.Containers, 3)
	})

	t.Run("global with machine filter", func(t *testing.T) {
		t.Parallel()

		name := "global-deployment-filtered"
		t.Cleanup(func() {
			err := cli.RemoveService(ctx, name)
			if !errors.Is(err, client.ErrNotFound) {
				require.NoError(t, err)
			}
		})

		// First deploy globally without filter to get containers on all machines.
		spec := api.ServiceSpec{
			Name: name,
			Mode: api.ServiceModeGlobal,
			Container: api.ContainerSpec{
				Image: "portainer/pause:latest",
			},
		}

		deploy, err := cli.NewDeployment(spec, nil)
		require.NoError(t, err)

		_, err = deploy.Run(ctx)
		require.NoError(t, err)

		svc, err := cli.InspectService(ctx, name)
		require.NoError(t, err)
		assert.Len(t, svc.Containers, 3, "expected 1 container on each machine")

		// Store initial container IDs by machine.
		initialContainers := make(map[string]string) // machineID -> containerID
		for _, ctr := range svc.Containers {
			initialContainers[ctr.MachineID] = ctr.Container.ID
		}

		// Update spec with Init=true, but only deploy to machines #0 and #2.
		init := true
		specWithInit := spec
		specWithInit.Container.Init = &init

		filter := func(m *pb.MachineInfo) bool {
			return m.Name == c.Machines[0].Name || m.Name == c.Machines[2].Name
		}
		strategy := &client.RollingStrategy{MachineFilter: filter}

		deploy, err = cli.NewDeployment(specWithInit, strategy)
		require.NoError(t, err)

		_, err = deploy.Run(ctx)
		require.NoError(t, err)

		svc, err = cli.InspectService(ctx, name)
		require.NoError(t, err)
		assert.Len(t, svc.Containers, 3, "still 1 container on each machine")

		// Verify:
		// 1. Containers on machines #0 and #2 were updated (new IDs, init enabled)
		// 2. Container on machine #1 remains unchanged (same ID, no init)
		for _, ctr := range svc.Containers {
			machine, err := cli.InspectMachine(ctx, ctr.MachineID)
			require.NoError(t, err)

			oldContainerID := initialContainers[ctr.MachineID]
			switch machine.Machine.Name {
			case c.Machines[0].Name, c.Machines[2].Name:
				// These containers should be updated.
				assert.NotEqual(t, oldContainerID, ctr.Container.ID,
					"Container on machine %s should have been updated", machine.Machine.Name)

				svcSpec, err := ctr.Container.ServiceSpec()
				require.NoError(t, err)
				assert.NotNil(t, svcSpec.Container.Init)
				assert.True(t, *svcSpec.Container.Init,
					"Container on machine %s should have init enabled", machine.Machine.Name)
			case c.Machines[1].Name:
				// This container should remain unchanged.
				assert.Equal(t, oldContainerID, ctr.Container.ID,
					"Container on machine %s should not have been updated", machine.Machine.Name)
			}
		}

		// Now deploy another update without filter - should affect all machines.
		init = false
		specWithPort := spec
		specWithPort.Ports = []api.PortSpec{
			{
				PublishedPort: 8001,
				ContainerPort: 8001,
				Protocol:      api.ProtocolTCP,
				Mode:          api.PortModeHost,
			},
		}

		deploy, err = cli.NewDeployment(specWithPort, nil)
		require.NoError(t, err)

		_, err = deploy.Run(ctx)
		require.NoError(t, err)

		svc, err = cli.InspectService(ctx, name)
		require.NoError(t, err)
		assert.Len(t, svc.Containers, 3)

		// Verify all containers are updated with a published port.
		for _, ctr := range svc.Containers {
			svcSpec, err := ctr.Container.ServiceSpec()
			require.NoError(t, err)
			assert.Nil(t, svcSpec.Container.Init,
				"Container on machine %s should have init disabled", ctr.MachineID)

			ports, err := ctr.Container.ServicePorts()
			require.NoError(t, err)
			assert.Equal(t, specWithPort.Ports, ports,
				"Container on machine %s should have updated port", ctr.MachineID)
		}
	})

	t.Run("caddy", func(t *testing.T) {
		t.Cleanup(func() {
			err := cli.RemoveService(ctx, client.CaddyServiceName)
			if !errors.Is(err, client.ErrNotFound) {
				require.NoError(t, err)
			}
		})

		deploy, err := cli.NewCaddyDeployment("", nil)
		require.NoError(t, err)

		_, err = deploy.Run(ctx)
		require.NoError(t, err)

		svc, err := cli.InspectService(ctx, client.CaddyServiceName)
		require.NoError(t, err)
		assert.Equal(t, client.CaddyServiceName, svc.Name)
		assert.Equal(t, api.ServiceModeGlobal, svc.Mode)
		assert.Len(t, svc.Containers, 3)

		ctr := svc.Containers[0].Container
		assert.Regexp(t, `^caddy:2\.\d+\.\d+$`, ctr.Config.Image)

		ports, err := ctr.ServicePorts()
		require.NoError(t, err)
		expectedPorts := []api.PortSpec{
			{
				PublishedPort: 80,
				ContainerPort: 80,
				Protocol:      api.ProtocolTCP,
				Mode:          api.PortModeHost,
			},
			{
				PublishedPort: 443,
				ContainerPort: 443,
				Protocol:      api.ProtocolTCP,
				Mode:          api.PortModeHost,
			},
		}
		assert.Equal(t, expectedPorts, ports)

		assert.Equal(t, container.RestartPolicy{
			Name:              container.RestartPolicyAlways,
			MaximumRetryCount: 0,
		}, ctr.HostConfig.RestartPolicy)
	})

	t.Run("caddy with machine filter", func(t *testing.T) {
		t.Cleanup(func() {
			err := cli.RemoveService(ctx, client.CaddyServiceName)
			if !errors.Is(err, client.ErrNotFound) {
				require.NoError(t, err)
			}
		})

		// Deploy to machine #0
		filter := func(m *pb.MachineInfo) bool {
			return m.Name == c.Machines[0].Name
		}

		deploy, err := cli.NewCaddyDeployment("", filter)
		require.NoError(t, err)

		_, err = deploy.Run(ctx)
		require.NoError(t, err)

		svc, err := cli.InspectService(ctx, client.CaddyServiceName)
		require.NoError(t, err)
		assert.Len(t, svc.Containers, 1)
		ctr0 := svc.Containers[0]

		machine0, err := cli.InspectMachine(ctx, ctr0.MachineID)
		require.NoError(t, err)
		assert.Equal(t, c.Machines[0].Name, machine0.Machine.Name)

		// Deploy to machines #0 and #2
		filter = func(m *pb.MachineInfo) bool {
			return m.Name == c.Machines[0].Name || m.Name == c.Machines[2].Name
		}

		deploy, err = cli.NewCaddyDeployment("", filter)
		require.NoError(t, err)

		_, err = deploy.Run(ctx)
		require.NoError(t, err)

		svc, err = cli.InspectService(ctx, client.CaddyServiceName)
		require.NoError(t, err)
		assert.Len(t, svc.Containers, 2)

		// Existing container ctr0 on machine #0 should be left unchanged.
		var ctr2 api.MachineContainer
		if ctr0.Container.ID == svc.Containers[0].Container.ID {
			ctr2 = svc.Containers[1]
		} else {
			assert.Equal(t, ctr0.Container.ID, svc.Containers[1].Container.ID)
			ctr2 = svc.Containers[0]
		}

		machine2, err := cli.InspectMachine(ctx, ctr2.MachineID)
		require.NoError(t, err)
		assert.Equal(t, c.Machines[2].Name, machine2.Machine.Name)
	})

	t.Run("replicated", func(t *testing.T) {
		t.Parallel()

		name := "replicated-deployment"
		t.Cleanup(func() {
			err := cli.RemoveService(ctx, name)
			if !errors.Is(err, client.ErrNotFound) {
				require.NoError(t, err)
			}
		})

		// 1. Create a basic replicated service with 2 replicas.
		spec := api.ServiceSpec{
			Name: name,
			Mode: api.ServiceModeReplicated,
			Container: api.ContainerSpec{
				Image: "portainer/pause:latest",
			},
			Replicas: 2,
		}

		deploy, err := cli.NewDeployment(spec, nil)
		require.NoError(t, err)

		err = deploy.Validate(ctx)
		require.NoError(t, err)

		plan, err := deploy.Plan(ctx)
		require.NoError(t, err)
		assert.Len(t, plan.SequenceOperation.Operations, 2) // 2 run operations for 2 replicas

		svcID, err := deploy.Run(ctx)
		require.NoError(t, err)
		assert.NotEmpty(t, svcID)

		// Verify service was created with correct settings.
		svc, err := cli.InspectService(ctx, name)
		require.NoError(t, err)
		assert.Equal(t, name, svc.Name)
		assert.Equal(t, api.ServiceModeReplicated, svc.Mode)
		assert.Len(t, svc.Containers, 2, "expected 2 replicas")

		// Verify containers are on different machines for balanced distribution.
		machines := make(map[string]struct{})
		for _, ctr := range svc.Containers {
			machines[ctr.MachineID] = struct{}{}

			// Verify container spec matches our deployment spec.
			svcSpec, err := ctr.Container.ServiceSpec()
			require.NoError(t, err)
			assert.True(t, svcSpec.Equals(spec))
		}
		assert.Len(t, machines, 2, "containers should be on different machines")

		// Store the initial container IDs.
		initialContainers := make(map[string]string) // machineID -> containerID
		for _, ctr := range svc.Containers {
			initialContainers[ctr.MachineID] = ctr.Container.ID
		}

		// 2. Update the service with a new configuration.
		init := true
		updatedSpec := spec
		updatedSpec.Container.Init = &init

		deploy, err = cli.NewDeployment(updatedSpec, nil)
		require.NoError(t, err)

		plan, err = deploy.Plan(ctx)
		require.NoError(t, err)
		assert.Len(t, plan.Operations, 4, "expected 2 run + 2 remove operations")

		_, err = deploy.Run(ctx)
		require.NoError(t, err)

		// Verify service was updated.
		svc, err = cli.InspectService(ctx, name)
		require.NoError(t, err)
		assert.Equal(t, name, svc.Name)
		assert.Len(t, svc.Containers, 2)

		// Verify initial containers were updated.
		for _, ctr := range svc.Containers {
			initialCtr, ok := initialContainers[ctr.MachineID]
			require.True(t, ok, "Updated container should have replaced one of the initial containers")

			assert.NotEqual(t, initialCtr, ctr.Container.ID,
				"Container on machine %s should have been updated", ctr.MachineID)

			svcSpec, err := ctr.Container.ServiceSpec()
			require.NoError(t, err)
			assert.True(t, svcSpec.Equals(updatedSpec))
		}

		// 3. Update to 4 replicas with a different configuration.
		initialContainers = make(map[string]string) // Reset container tracking.
		for _, ctr := range svc.Containers {
			initialContainers[ctr.MachineID] = ctr.Container.ID
		}

		fourReplicaSpec := updatedSpec
		fourReplicaSpec.Container.Command = []string{"updated"}
		fourReplicaSpec.Replicas = 4

		deploy, err = cli.NewDeployment(fourReplicaSpec, nil)
		require.NoError(t, err)

		plan, err = deploy.Plan(ctx)
		require.NoError(t, err)
		assert.Len(t, plan.Operations, 6, "Expected 4 run + 2 remove operations")

		_, err = deploy.Run(ctx)
		require.NoError(t, err)

		// Verify service now has 4 containers.
		svc, err = cli.InspectService(ctx, name)
		require.NoError(t, err)
		assert.Equal(t, name, svc.Name)
		assert.Len(t, svc.Containers, 4, "Expected 4 replicas")

		// Count containers per machine.
		machineContainerCount := make(map[string]int)
		for _, ctr := range svc.Containers {
			machineContainerCount[ctr.MachineID]++

			// Verify all containers match the new spec
			svcSpec, err := ctr.Container.ServiceSpec()
			require.NoError(t, err)
			assert.True(t, svcSpec.Equals(fourReplicaSpec))

			// For existing machines, verify containers were replaced
			if initialID, ok := initialContainers[ctr.MachineID]; ok {
				assert.NotEqual(t, initialID, ctr.Container.ID,
					"Container on machine %s should have been updated", ctr.MachineID)
			}
		}

		// Verify even distributions across machines.
		assert.Len(t, machineContainerCount, 3, "Expected containers on all 3 machines")
		for _, count := range machineContainerCount {
			assert.GreaterOrEqual(t, count, 1, "Expected at least 1 container on each machine")
		}

		// 4. Redeploy the exact same spec and verify it's a noop.
		deploy, err = cli.NewDeployment(fourReplicaSpec, nil)
		require.NoError(t, err)

		plan, err = deploy.Plan(ctx)
		require.NoError(t, err)

		svc, err = cli.InspectService(ctx, name)
		require.NoError(t, err)

		assert.Empty(t, plan.Operations, "Redeploying the same spec should be a no-op")
	})

	// TODO: test deployments with unreachable machines. See https://github.com/psviderski/uncloud/issues/29.
}

func TestRunService(t *testing.T) {
	t.Parallel()

	clusterName := "ucind-test.run-service"
	ctx := context.Background()
	c, _ := createTestCluster(t, clusterName, ucind.CreateClusterOptions{Machines: 3}, true)

	cli, err := c.Machines[0].Connect(ctx)
	require.NoError(t, err)

	t.Run("create container with spec defaults", func(t *testing.T) {
		t.Parallel()

		serviceID := newServiceID()
		// Only required fields.
		spec := api.ServiceSpec{
			Name: "container-spec-defaults",
			Container: api.ContainerSpec{
				Image: "portainer/pause:latest",
			},
		}

		resp, err := cli.CreateContainer(ctx, serviceID, spec, c.Machines[0].Name)
		require.NoError(t, err)
		assert.NotEmpty(t, resp.ID)

		t.Cleanup(func() {
			err := cli.RemoveContainer(ctx, serviceID, resp.ID, container.RemoveOptions{Force: true})
			if !errors.Is(err, client.ErrNotFound) {
				require.NoError(t, err)
			}
		})

		// Verify container configuration.
		mc, err := cli.InspectContainer(ctx, serviceID, resp.ID)
		require.NoError(t, err)
		ctr := mc.Container

		assert.True(t, strings.HasPrefix(ctr.Name, "/container-spec-defaults-"))
		assert.Equal(t, "portainer/pause:latest", ctr.Config.Image)

		// Verify default settings.
		assert.Empty(t, ctr.Config.Cmd)
		assert.Nil(t, ctr.HostConfig.Init)
		assert.Empty(t, ctr.HostConfig.Binds)
		assert.Empty(t, ctr.HostConfig.PortBindings)
		assert.Equal(t, container.RestartPolicy{
			Name:              container.RestartPolicyAlways,
			MaximumRetryCount: 0,
		}, ctr.HostConfig.RestartPolicy)

		// Verify labels.
		assert.Equal(t, serviceID, ctr.Config.Labels[api.LabelServiceID])
		assert.Equal(t, spec.Name, ctr.Config.Labels[api.LabelServiceName])
		assert.Equal(t, api.ServiceModeReplicated, ctr.Config.Labels[api.LabelServiceMode])
		assert.NotContains(t, ctr.Config.Labels, api.LabelServicePorts) // No ports set.
		assert.Contains(t, ctr.Config.Labels, api.LabelManaged)

		// Verify network settings.
		assert.Len(t, ctr.NetworkSettings.Networks, 1)
		assert.Contains(t, ctr.NetworkSettings.Networks, machinedocker.NetworkName)
	})

	t.Run("create container with full spec", func(t *testing.T) {
		t.Parallel()

		serviceID := newServiceID()
		init := true
		spec := api.ServiceSpec{
			Name: "container-spec-full",
			Mode: api.ServiceModeGlobal,
			Container: api.ContainerSpec{
				Command: []string{"sleep", "infinity"},
				Image:   "portainer/pause:latest",
				Init:    &init,
				Volumes: []string{"/host/path:/container/path:ro"},
			},
			Ports: []api.PortSpec{
				{
					HostIP:        netip.MustParseAddr("127.0.0.1"),
					PublishedPort: 80,
					ContainerPort: 8080,
					Protocol:      api.ProtocolTCP,
					Mode:          api.PortModeHost,
				},
				{
					Hostname:      "app.example.com",
					ContainerPort: 8000,
					Protocol:      api.ProtocolHTTPS,
					Mode:          api.PortModeIngress,
				},
			},
		}

		resp, err := cli.CreateContainer(ctx, serviceID, spec, c.Machines[0].Name)
		require.NoError(t, err)
		assert.NotEmpty(t, resp.ID)

		t.Cleanup(func() {
			err := cli.RemoveContainer(ctx, serviceID, resp.ID, container.RemoveOptions{Force: true})
			if !errors.Is(err, client.ErrNotFound) {
				require.NoError(t, err)
			}
		})

		// Verify container configuration.
		mc, err := cli.InspectContainer(ctx, serviceID, resp.ID)
		require.NoError(t, err)
		ctr := mc.Container

		assert.True(t, strings.HasPrefix(ctr.Name, "/container-spec-full-"))
		assert.Equal(t, "portainer/pause:latest", ctr.Config.Image)

		assert.EqualValues(t, spec.Container.Command, ctr.Config.Cmd)
		assert.True(t, *ctr.HostConfig.Init)
		assert.Len(t, ctr.HostConfig.Binds, 1)
		assert.Contains(t, ctr.HostConfig.Binds, spec.Container.Volumes[0])

		assert.Len(t, ctr.HostConfig.PortBindings, 1)
		expectedPort := []nat.PortBinding{
			{
				HostIP:   "127.0.0.1",
				HostPort: "80",
			},
		}
		assert.Equal(t, expectedPort, ctr.HostConfig.PortBindings[nat.Port("8080/tcp")])

		assert.Equal(t, container.RestartPolicy{
			Name:              container.RestartPolicyAlways,
			MaximumRetryCount: 0,
		}, ctr.HostConfig.RestartPolicy)

		// Verify labels.
		assert.Equal(t, serviceID, ctr.Config.Labels[api.LabelServiceID])
		assert.Equal(t, spec.Name, ctr.Config.Labels[api.LabelServiceName])
		assert.Equal(t, api.ServiceModeGlobal, ctr.Config.Labels[api.LabelServiceMode])
		assert.Equal(t, "127.0.0.1:80:8080/tcp@host,app.example.com:8000/https", ctr.Config.Labels[api.LabelServicePorts])
		assert.Contains(t, ctr.Config.Labels, api.LabelManaged)

		// Verify network settings.
		assert.Len(t, ctr.NetworkSettings.Networks, 1)
		assert.Contains(t, ctr.NetworkSettings.Networks, machinedocker.NetworkName)
	})

	// TODO: create container invalid spec.
	//  - service Name is required

	t.Run("container lifecycle", func(t *testing.T) {
		t.Parallel()

		serviceID := newServiceID()
		spec := api.ServiceSpec{
			Name: "container-lifecycle",
			Container: api.ContainerSpec{
				Image: "portainer/pause:latest",
			},
		}

		ctr, err := cli.CreateContainer(ctx, serviceID, spec, c.Machines[0].Name)
		require.NoError(t, err)
		assert.NotEmpty(t, ctr.ID)

		t.Cleanup(func() {
			err := cli.RemoveContainer(ctx, serviceID, ctr.ID, container.RemoveOptions{Force: true})
			if !errors.Is(err, client.ErrNotFound) {
				require.NoError(t, err)
			}
		})

		err = cli.StartContainer(ctx, serviceID, ctr.ID)
		require.NoError(t, err)

		timeout := 1
		err = cli.StopContainer(ctx, serviceID, ctr.ID, container.StopOptions{Timeout: &timeout})
		require.NoError(t, err)

		err = cli.RemoveContainer(ctx, serviceID, ctr.ID, container.RemoveOptions{})
		require.NoError(t, err)

		err = cli.RemoveContainer(ctx, serviceID, ctr.ID, container.RemoveOptions{})
		require.ErrorIs(t, err, client.ErrNotFound)
	})

	t.Run("1 replica", func(t *testing.T) {
		t.Parallel()

		name := "1-replica"
		t.Cleanup(func() {
			err := cli.RemoveService(ctx, name)
			if !errors.Is(err, client.ErrNotFound) {
				require.NoError(t, err)
			}

			_, err = cli.InspectService(ctx, name)
			require.ErrorIs(t, err, client.ErrNotFound)
		})

		resp, err := cli.RunService(ctx, api.ServiceSpec{
			Name: name,
			Mode: api.ServiceModeReplicated,
			Container: api.ContainerSpec{
				Image: "portainer/pause:latest",
			},
		}, nil)
		require.NoError(t, err)

		assert.NotEmpty(t, resp.ID)
		assert.Equal(t, name, resp.Name)

		svc, err := cli.InspectService(ctx, name)
		require.NoError(t, err)

		assert.Equal(t, resp.ID, svc.ID)
		assert.Equal(t, name, svc.Name)
		assert.Equal(t, api.ServiceModeReplicated, svc.Mode)
		assert.Len(t, svc.Containers, 1)

		services, err := cli.ListServices(ctx)
		require.NoError(t, err)

		assert.GreaterOrEqual(t, len(services), 1)
		found := false
		for _, s := range services {
			if s.ID == svc.ID {
				assert.Equal(t, name, s.Name)
				assert.Equal(t, api.ServiceModeReplicated, s.Mode)
				assert.Len(t, s.Containers, 1)
				found = true
			}
		}
		assert.True(t, found)
	})

	t.Run("1 replica with ports", func(t *testing.T) {
		t.Parallel()

		name := "1-replica-ports"
		t.Cleanup(func() {
			err := cli.RemoveService(ctx, name)
			if !errors.Is(err, client.ErrNotFound) {
				require.NoError(t, err)
			}
		})

		spec := api.ServiceSpec{
			Name: name,
			Mode: api.ServiceModeReplicated,
			Container: api.ContainerSpec{
				Image: "portainer/pause:latest",
			},
			Ports: []api.PortSpec{
				{
					Hostname:      "https.example.com",
					ContainerPort: 8080,
					Protocol:      api.ProtocolHTTPS,
					Mode:          api.PortModeIngress,
				},
				{
					PublishedPort: 8000,
					ContainerPort: 8080,
					Protocol:      api.ProtocolTCP,
					Mode:          api.PortModeIngress,
				},
				{
					PublishedPort: 8000,
					ContainerPort: 8000,
					Protocol:      api.ProtocolUDP,
					Mode:          api.PortModeHost,
				},
			},
		}
		resp, err := cli.RunService(ctx, spec, nil)
		require.NoError(t, err)

		svc, err := cli.InspectService(ctx, resp.ID)
		require.NoError(t, err)
		require.Len(t, svc.Containers, 1)
		ctr := svc.Containers[0].Container

		ports, err := ctr.ServicePorts()
		require.NoError(t, err)
		assert.Equal(t, spec.Ports, ports)
	})

	t.Run("global mode", func(t *testing.T) {
		t.Parallel()

		name := "global"
		t.Cleanup(func() {
			err := cli.RemoveService(ctx, name)
			if !errors.Is(err, client.ErrNotFound) {
				require.NoError(t, err)
			}
		})

		resp, err := cli.RunService(ctx, api.ServiceSpec{
			Name: name,
			Mode: api.ServiceModeGlobal,
			Container: api.ContainerSpec{
				Image: "portainer/pause:latest",
			},
		}, nil)
		require.NoError(t, err)

		assert.NotEmpty(t, resp.ID)
		assert.Equal(t, name, resp.Name)

		svc, err := cli.InspectService(ctx, name)
		require.NoError(t, err)

		assert.Equal(t, resp.ID, svc.ID)
		assert.Equal(t, name, svc.Name)
		assert.Equal(t, api.ServiceModeGlobal, svc.Mode)
		assert.Len(t, svc.Containers, 3, "expected 1 container on each machine")
	})
}

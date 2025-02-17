package e2e

import (
	"context"
	"errors"
	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"uncloud/internal/api"
	"uncloud/internal/cli/client"
	"uncloud/internal/machine/api/pb"
	"uncloud/internal/ucind"
)

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
			if errors.Is(err, client.ErrNotFound) {
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
			if errors.Is(err, client.ErrNotFound) {
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
			if errors.Is(err, client.ErrNotFound) {
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
	})

	t.Run("caddy with machine filter", func(t *testing.T) {
		t.Cleanup(func() {
			err := cli.RemoveService(ctx, client.CaddyServiceName)
			if errors.Is(err, client.ErrNotFound) {
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
}

func TestRunService(t *testing.T) {
	t.Parallel()

	clusterName := "ucind-test.run-service"
	ctx := context.Background()
	c, _ := createTestCluster(t, clusterName, ucind.CreateClusterOptions{Machines: 3}, true)

	cli, err := c.Machines[0].Connect(ctx)
	require.NoError(t, err)

	t.Run("container lifecycle", func(t *testing.T) {
		t.Parallel()

		svcName := "pause-container-lifecycle"
		spec := api.ServiceSpec{
			Name: svcName,
			Container: api.ContainerSpec{
				Image: "portainer/pause:latest",
			},
		}
		machineID := c.Machines[0].Name

		ctr, err := cli.CreateContainer(ctx, svcName, spec, machineID)
		require.NoError(t, err)
		assert.NotEmpty(t, ctr.ID)

		t.Cleanup(func() {
			err := cli.RemoveContainer(ctx, svcName, ctr.ID, container.RemoveOptions{Force: true})
			if !errors.Is(err, client.ErrNotFound) {
				require.NoError(t, err)
			}
		})

		err = cli.StartContainer(ctx, svcName, ctr.ID)
		require.NoError(t, err)

		timeout := 1
		err = cli.StopContainer(ctx, svcName, ctr.ID, container.StopOptions{Timeout: &timeout})
		require.NoError(t, err)

		err = cli.RemoveContainer(ctx, svcName, ctr.ID, container.RemoveOptions{})
		require.NoError(t, err)

		err = cli.RemoveContainer(ctx, svcName, ctr.ID, container.RemoveOptions{})
		require.ErrorIs(t, err, client.ErrNotFound)
	})

	t.Run("1 replica", func(t *testing.T) {
		t.Parallel()

		name := "pause-1-replica"
		t.Cleanup(func() {
			err := cli.RemoveService(ctx, name)
			if errors.Is(err, client.ErrNotFound) {
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
		})
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

		name := "pause-1-replica-ports"
		t.Cleanup(func() {
			err := cli.RemoveService(ctx, name)
			if errors.Is(err, client.ErrNotFound) {
				require.NoError(t, err)
			}

			_, err = cli.InspectService(ctx, name)
			require.ErrorIs(t, err, client.ErrNotFound)
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
		resp, err := cli.RunService(ctx, spec)
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

		name := "pause-global"
		t.Cleanup(func() {
			err := cli.RemoveService(ctx, name)
			if errors.Is(err, client.ErrNotFound) {
				require.NoError(t, err)
			}

			_, err = cli.InspectService(ctx, name)
			require.ErrorIs(t, err, client.ErrNotFound)
		})

		resp, err := cli.RunService(ctx, api.ServiceSpec{
			Name: name,
			Mode: api.ServiceModeGlobal,
			Container: api.ContainerSpec{
				Image: "portainer/pause:latest",
			},
		})
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

package e2e

import (
	"context"
	"errors"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	machinedocker "github.com/psviderski/uncloud/internal/machine/docker"
	"github.com/psviderski/uncloud/internal/secret"
	"github.com/psviderski/uncloud/internal/ucind"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client"
	"github.com/psviderski/uncloud/pkg/client/deploy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/netip"
	"strings"
	"testing"
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

	t.Run("global auto-generated name", func(t *testing.T) {
		t.Parallel()

		name := "" // auto-generated and updated
		t.Cleanup(func() {
			err := cli.RemoveService(ctx, name)
			if !errors.Is(err, api.ErrNotFound) {
				require.NoError(t, err)
			}

			_, err = cli.InspectService(ctx, name)
			require.ErrorIs(t, err, api.ErrNotFound)
		})

		spec := api.ServiceSpec{
			Mode: api.ServiceModeGlobal,
			Container: api.ContainerSpec{
				Image: "portainer/pause:latest",
			},
		}
		deployment := cli.NewDeployment(spec, nil)

		err = deployment.Validate(ctx)
		require.NoError(t, err)

		plan, err := deployment.Plan(ctx)
		require.NoError(t, err)
		assert.NotEmpty(t, plan.ServiceID)
		assert.NotEmpty(t, plan.ServiceName)
		assert.Len(t, plan.SequenceOperation.Operations, 3) // 3 run

		runPlan, err := deployment.Run(ctx)
		require.NoError(t, err)
		assert.Equal(t, plan, runPlan)

		name = plan.ServiceName
		spec.Name = name // update spec to match the service and assert with assertServiceMatchesSpec

		svc, err := cli.InspectService(ctx, name)
		require.NoError(t, err)
		assertServiceMatchesSpec(t, svc, spec)

		assert.Len(t, svc.Containers, 3)
		machines := serviceMachines(t, svc)
		assert.Len(t, machines.ToSlice(), 3, "Expected 1 container on each machine")

		// Deploy a published port.
		initialContainers := serviceContainerIDs(t, svc)

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
		deployment = cli.NewDeployment(specWithPort, nil)

		plan, err = deployment.Plan(ctx)
		require.NoError(t, err)
		assert.Len(t, plan.SequenceOperation.Operations, 6) // 3 run + 3 remove

		_, err = deployment.Run(ctx)
		require.NoError(t, err)

		svc, err = cli.InspectService(ctx, name)
		require.NoError(t, err)
		assertServiceMatchesSpec(t, svc, specWithPort)

		assert.Len(t, svc.Containers, 3)
		machines = serviceMachines(t, svc)
		assert.Len(t, machines.ToSlice(), 3, "Expected 1 container on each machine")
		containers := serviceContainerIDs(t, svc)
		assert.Empty(t, initialContainers.Intersect(containers).ToSlice(),
			"All existing containers should be replaced")

		// Deploy the same conflicting port but with container spec changes
		initialContainers = containers

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
		deployment = cli.NewDeployment(specWithPortAndInit, nil)

		plan, err = deployment.Plan(ctx)
		require.NoError(t, err)
		assert.Len(t, plan.SequenceOperation.Operations, 9) // 3 stop + 3 run + 3 remove

		_, err = deployment.Run(ctx)
		require.NoError(t, err)

		svc, err = cli.InspectService(ctx, name)
		require.NoError(t, err)
		assertServiceMatchesSpec(t, svc, specWithPortAndInit)

		assert.Len(t, svc.Containers, 3)
		machines = serviceMachines(t, svc)
		assert.Len(t, machines.ToSlice(), 3, "Expected 1 container on each machine")
		containers = serviceContainerIDs(t, svc)
		assert.Empty(t, initialContainers.Intersect(containers).ToSlice(),
			"All existing containers should be replaced")

		// Deploying the same spec should be a no-op.
		initialContainers = containers

		deployment = cli.NewDeployment(specWithPortAndInit, nil)

		plan, err = deployment.Plan(ctx)
		require.NoError(t, err)
		assert.Len(t, plan.SequenceOperation.Operations, 0) // no-op

		_, err = deployment.Run(ctx)
		require.NoError(t, err)

		svc, err = cli.InspectService(ctx, name)
		require.NoError(t, err)

		containers = serviceContainerIDs(t, svc)
		assert.ElementsMatch(t, initialContainers.ToSlice(), containers.ToSlice())
	})

	t.Run("global with machine filter", func(t *testing.T) {
		t.Parallel()

		name := "global-deployment-filtered"
		t.Cleanup(func() {
			err := cli.RemoveService(ctx, name)
			if !errors.Is(err, api.ErrNotFound) {
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
		deployment := cli.NewDeployment(spec, nil)

		_, err = deployment.Run(ctx)
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
		strategy := &deploy.RollingStrategy{MachineFilter: filter}
		deployment = cli.NewDeployment(specWithInit, strategy)

		_, err = deployment.Run(ctx)
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
		deployment = cli.NewDeployment(specWithPort, nil)
		_, err = deployment.Run(ctx)
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
			if !errors.Is(err, api.ErrNotFound) {
				require.NoError(t, err)
			}
		})

		deployment, err := cli.NewCaddyDeployment("", nil)
		require.NoError(t, err)

		_, err = deployment.Run(ctx)
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
			if !errors.Is(err, api.ErrNotFound) {
				require.NoError(t, err)
			}
		})

		// Deploy to machine #0
		filter := func(m *pb.MachineInfo) bool {
			return m.Name == c.Machines[0].Name
		}

		deployment, err := cli.NewCaddyDeployment("", filter)
		require.NoError(t, err)
		image := deployment.Spec.Container.Image

		_, err = deployment.Run(ctx)
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
		deployment, err = cli.NewCaddyDeployment(image, filter)
		require.NoError(t, err)

		_, err = deployment.Run(ctx)
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
			if !errors.Is(err, api.ErrNotFound) {
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
		deployment := cli.NewDeployment(spec, nil)

		err = deployment.Validate(ctx)
		require.NoError(t, err)

		plan, err := deployment.Plan(ctx)
		require.NoError(t, err)
		assert.NotEmpty(t, plan.ServiceID)
		assert.Equal(t, name, plan.ServiceName)
		assert.Len(t, plan.SequenceOperation.Operations, 2) // 2 run operations for 2 replicas

		runPlan, err := deployment.Run(ctx)
		require.NoError(t, err)
		assert.Equal(t, plan, runPlan)

		// Verify service was created with correct settings.
		svc, err := cli.InspectService(ctx, name)
		require.NoError(t, err)
		assertServiceMatchesSpec(t, svc, spec)

		// Verify containers are on different machines for balanced distribution.
		initialMachines := serviceMachines(t, svc)
		assert.Len(t, initialMachines.ToSlice(), 2, "Expected 2 containers on 2 different machines")
		initialContainers := serviceContainerIDs(t, svc)

		// 2. Update the service with a new configuration.
		init := true
		updatedSpec := spec
		updatedSpec.Container.Init = &init
		deployment = cli.NewDeployment(updatedSpec, nil)

		plan, err = deployment.Plan(ctx)
		require.NoError(t, err)
		assert.Len(t, plan.Operations, 4, "Expected 2 run + 2 remove operations")

		_, err = deployment.Run(ctx)
		require.NoError(t, err)

		svc, err = cli.InspectService(ctx, name)
		require.NoError(t, err)
		assertServiceMatchesSpec(t, svc, updatedSpec)

		// Verify containers are on the same machines as before but the initial containers were replaced.
		machines := serviceMachines(t, svc)
		assert.ElementsMatch(t, initialMachines.ToSlice(), machines.ToSlice(),
			"Expected containers on the same machines")
		containers := serviceContainerIDs(t, svc)
		assert.Empty(t, initialContainers.Intersect(containers).ToSlice(),
			"All existing containers should be replaced")

		// 3. Scale to 3 replicas.
		initialMachines = machines
		initialContainers = containers // Reset container tracking.

		threeReplicaSpec := updatedSpec
		threeReplicaSpec.Replicas = 3
		deployment = cli.NewDeployment(threeReplicaSpec, nil)

		plan, err = deployment.Plan(ctx)
		require.NoError(t, err)
		assert.Len(t, plan.Operations, 1, "Expected 1 run operation")

		_, err = deployment.Run(ctx)
		require.NoError(t, err)

		svc, err = cli.InspectService(ctx, name)
		require.NoError(t, err)
		assertServiceMatchesSpec(t, svc, threeReplicaSpec)

		// Verify existing containers remain and a new one was added on a different machine.
		machines = serviceMachines(t, svc)
		assert.Len(t, machines.ToSlice(), 3, "Expected 3 containers on 3 different machines")
		containers = serviceContainerIDs(t, svc)
		assert.Len(t, containers.Intersect(initialContainers).ToSlice(), 2, "Expected 2 initial containers to remain")

		// 4. Update to 5 replicas with a different configuration.
		initialContainers = containers // Reset container tracking.

		fourReplicaSpec := updatedSpec
		fourReplicaSpec.Container.Command = []string{"updated"}
		fourReplicaSpec.Replicas = 5
		deployment = cli.NewDeployment(fourReplicaSpec, nil)

		plan, err = deployment.Plan(ctx)
		require.NoError(t, err)
		assert.Len(t, plan.Operations, 8, "Expected 5 run + 3 remove operations")

		_, err = deployment.Run(ctx)
		require.NoError(t, err)

		svc, err = cli.InspectService(ctx, name)
		require.NoError(t, err)
		assertServiceMatchesSpec(t, svc, fourReplicaSpec)

		// Verify all existing containers were replaced and new ones are evenly distributed.
		machines = serviceMachines(t, svc)
		assert.Len(t, machines.ToSlice(), 3, "Expected containers on 3 different machines")
		containers = serviceContainerIDs(t, svc)
		assert.Empty(t, containers.Intersect(initialContainers).ToSlice(),
			"All existing containers should be replaced")

		machineContainers := serviceContainersByMachine(t, svc)
		for _, ctrs := range machineContainers {
			assert.LessOrEqual(t, len(ctrs), 2, "Expected at most 2 containers on each machine")
		}

		// 5. Redeploy the exact same spec and verify it's a noop.
		initialContainers = containers // Reset container tracking.

		deployment = cli.NewDeployment(fourReplicaSpec, nil)

		plan, err = deployment.Plan(ctx)
		require.NoError(t, err)
		assert.Empty(t, plan.Operations, "Redeploying the same spec should be a no-op")

		_, err = deployment.Run(ctx)
		require.NoError(t, err)

		svc, err = cli.InspectService(ctx, name)
		require.NoError(t, err)

		containers = serviceContainerIDs(t, svc)
		assert.ElementsMatch(t, initialContainers.ToSlice(), containers.ToSlice())
	})

	t.Run("replicated with machine filter", func(t *testing.T) {
		t.Parallel()

		name := "replicated-deployment-filtered"
		t.Cleanup(func() {
			err := cli.RemoveService(ctx, name)
			if !errors.Is(err, api.ErrNotFound) {
				require.NoError(t, err)
			}
		})

		// Create a replicated service with 2 replicas but limit to machines 0 and 1
		spec := api.ServiceSpec{
			Name: name,
			Mode: api.ServiceModeReplicated,
			Container: api.ContainerSpec{
				Image: "portainer/pause:latest",
			},
			Replicas: 2,
		}

		machine01Filter := func(m *pb.MachineInfo) bool {
			return m.Name == c.Machines[0].Name || m.Name == c.Machines[1].Name
		}
		strategy := &deploy.RollingStrategy{MachineFilter: machine01Filter}
		deployment := cli.NewDeployment(spec, strategy)

		_, err = deployment.Run(ctx)
		require.NoError(t, err)

		// Verify service has 2 containers on machines 0 and 1.
		svc, err := cli.InspectService(ctx, name)
		require.NoError(t, err)

		assert.Len(t, svc.Containers, 2)
		assert.NotEqual(t, svc.Containers[0].MachineID, svc.Containers[1].MachineID,
			"Expected containers on different machines")

		machineNames := make(map[string]string)
		for _, ctr := range svc.Containers {
			machine, err := cli.InspectMachine(ctx, ctr.MachineID)
			require.NoError(t, err)
			machineNames[ctr.MachineID] = machine.Machine.Name

			// Should only be on machines 0 or 1.
			assert.Contains(t, []string{c.Machines[0].Name, c.Machines[1].Name}, machine.Machine.Name)
		}

		// Now update the filter to only allow machine 2
		machine2Filter := func(m *pb.MachineInfo) bool {
			return m.Name == c.Machines[2].Name
		}
		strategy = &deploy.RollingStrategy{MachineFilter: machine2Filter}
		deployment = cli.NewDeployment(spec, strategy)

		_, err = deployment.Run(ctx)
		require.NoError(t, err)

		// Verify service now has containers only on machine 2.
		svc, err = cli.InspectService(ctx, name)
		require.NoError(t, err)

		assert.Len(t, svc.Containers, 2) // Still 2 replicas.
		assert.Equal(t, svc.Containers[0].MachineID, svc.Containers[1].MachineID,
			"Expected containers on the same machine")

		machine, err := cli.InspectMachine(ctx, svc.Containers[0].MachineID)
		require.NoError(t, err)
		assert.Equal(t, c.Machines[2].Name, machine.Machine.Name, "Containers should only be on machine #2")
	})

	// TODO: test deployments with unreachable machines. See https://github.com/psviderski/uncloud/issues/29.
}

func TestServiceLifecycle(t *testing.T) {
	t.Parallel()

	clusterName := "ucind-test.service"
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
			if !errors.Is(err, api.ErrNotFound) {
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
		assert.EqualValues(t, []string{"/pause"}, ctr.Config.Entrypoint) // Populated by the image.
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
				// Extra slashes is not a typo, it changes the spec but Linux ignores them and uses the default /pause.
				Entrypoint: []string{"///pause"},
				Image:      "portainer/pause:latest",
				Init:       &init,
				Volumes:    []string{"/host/path:/container/path:ro"},
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
			if !errors.Is(err, api.ErrNotFound) {
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
		assert.EqualValues(t, spec.Container.Entrypoint, ctr.Config.Entrypoint)
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

	t.Run("create container invalid service", func(t *testing.T) {
		t.Parallel()

		invalidIDs := []string{"", "invalid", "651aef23ae90"}
		spec := api.ServiceSpec{
			Name: "invalid-service-id",
			Container: api.ContainerSpec{
				Image: "portainer/pause:latest",
			},
		}

		for _, invalidID := range invalidIDs {
			_, err := cli.CreateContainer(ctx, invalidID, spec, c.Machines[0].Name)
			require.ErrorContains(t, err, "invalid service ID")
		}
	})

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
			if !errors.Is(err, api.ErrNotFound) {
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
		require.ErrorIs(t, err, api.ErrNotFound)
	})

	t.Run("1 replica", func(t *testing.T) {
		t.Parallel()

		name := "1-replica"
		t.Cleanup(func() {
			err := cli.RemoveService(ctx, name)
			if !errors.Is(err, api.ErrNotFound) {
				require.NoError(t, err)
			}

			_, err = cli.InspectService(ctx, name)
			require.ErrorIs(t, err, api.ErrNotFound)
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
			if !errors.Is(err, api.ErrNotFound) {
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
				// Not supported yet.
				//{
				//	PublishedPort: 8000,
				//	ContainerPort: 8080,
				//	Protocol:      api.ProtocolTCP,
				//	Mode:          api.PortModeIngress,
				//},
				{
					PublishedPort: 8000,
					ContainerPort: 8000,
					Protocol:      api.ProtocolTCP,
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
			if !errors.Is(err, api.ErrNotFound) {
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

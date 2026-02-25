package e2e

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/go-units"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/internal/secret"
	"github.com/psviderski/uncloud/internal/ucind"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client"
	"github.com/psviderski/uncloud/pkg/client/deploy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		machines := serviceMachines(svc)
		assert.Len(t, machines.ToSlice(), 3, "Expected 1 container on each machine")

		// Deploy a published port.
		initialContainers := serviceContainerIDs(svc)

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
		assert.Len(t, plan.SequenceOperation.Operations, 3) // 3 replace

		_, err = deployment.Run(ctx)
		require.NoError(t, err)

		svc, err = cli.InspectService(ctx, name)
		require.NoError(t, err)
		assertServiceMatchesSpec(t, svc, specWithPort)

		assert.Len(t, svc.Containers, 3)
		machines = serviceMachines(svc)
		assert.Len(t, machines.ToSlice(), 3, "Expected 1 container on each machine")
		containers := serviceContainerIDs(svc)
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
		assert.Len(t, plan.SequenceOperation.Operations, 3) // 3 replace (stop-first due to port conflicts)

		_, err = deployment.Run(ctx)
		require.NoError(t, err)

		svc, err = cli.InspectService(ctx, name)
		require.NoError(t, err)
		assertServiceMatchesSpec(t, svc, specWithPortAndInit)

		assert.Len(t, svc.Containers, 3)
		machines = serviceMachines(svc)
		assert.Len(t, machines.ToSlice(), 3, "Expected 1 container on each machine")
		containers = serviceContainerIDs(svc)
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

		containers = serviceContainerIDs(svc)
		assert.ElementsMatch(t, initialContainers.ToSlice(), containers.ToSlice())
	})

	t.Run("global with machine placement", func(t *testing.T) {
		t.Parallel()

		name := "test-global-deployment-machine-placement"
		t.Cleanup(func() {
			err := cli.RemoveService(ctx, name)
			if !errors.Is(err, api.ErrNotFound) {
				require.NoError(t, err)
			}
		})

		// First deploy globally to machines #0 and #1.
		spec := api.ServiceSpec{
			Name: name,
			Mode: api.ServiceModeGlobal,
			Container: api.ContainerSpec{
				Image: "portainer/pause:latest",
			},
			Placement: api.Placement{
				Machines: []string{c.Machines[0].Name, c.Machines[1].Name},
			},
		}
		deployment := cli.NewDeployment(spec, nil)

		_, err = deployment.Run(ctx)
		require.NoError(t, err)

		svc, err := cli.InspectService(ctx, name)
		require.NoError(t, err)
		assertServiceMatchesSpec(t, svc, spec)

		assert.Len(t, svc.Containers, 2, "Expected 1 container on machines %s and %s",
			c.Machines[0].Name, c.Machines[1].Name)
		initialMachines := serviceMachines(svc)
		assert.ElementsMatch(t, initialMachines.ToSlice(), []string{c.Machines[0].ID, c.Machines[1].ID})

		initialContainers := serviceContainerIDs(svc)

		// Update spec with Init=true, but only deploy to machines #0 and #2.
		init := true
		specWithInit := spec
		specWithInit.Container.Init = &init
		specWithInit.Placement.Machines = []string{c.Machines[0].Name, c.Machines[2].Name}

		deployment = cli.NewDeployment(specWithInit, nil)

		_, err = deployment.Run(ctx)
		require.NoError(t, err)

		svc, err = cli.InspectService(ctx, name)
		require.NoError(t, err)
		assertServiceMatchesSpec(t, svc, specWithInit)

		assert.Len(t, svc.Containers, 2, "Expected 1 container on machines %s and %s",
			c.Machines[0].Name, c.Machines[2].Name)
		machines := serviceMachines(svc)
		assert.ElementsMatch(t, machines.ToSlice(), []string{c.Machines[0].ID, c.Machines[2].ID})

		containers := serviceContainerIDs(svc)
		assert.Empty(t, initialContainers.Intersect(containers).ToSlice(),
			"All initial containers should be replaced")

		// Now deploy the same spec without a placement constraint.
		initialContainers = containers // Reset container tracking.
		specWithInit.Placement = api.Placement{}

		deployment = cli.NewDeployment(specWithInit, nil)
		_, err = deployment.Run(ctx)
		require.NoError(t, err)

		svc, err = cli.InspectService(ctx, name)
		require.NoError(t, err)
		assertServiceMatchesSpec(t, svc, specWithInit)
		assert.Len(t, svc.Containers, 3)

		machines = serviceMachines(svc)
		assert.Len(t, machines.ToSlice(), 3, "Expected 1 container on each machine")

		// TODO: update the container spec in-place if only the placement constraint has changed.
		// containers = serviceContainerIDs(svc)
		// assert.True(t, initialContainers.IsSubset(containers), "Expected all initial containers to remain")
	})

	t.Run("caddy", func(t *testing.T) {
		t.Cleanup(func() {
			err := cli.RemoveService(ctx, client.CaddyServiceName)
			if !errors.Is(err, api.ErrNotFound) {
				require.NoError(t, err)
			}
		})

		deployment, err := cli.NewCaddyDeployment("", "", api.Placement{})
		require.NoError(t, err)

		_, err = deployment.Run(ctx)
		require.NoError(t, err)

		svc, err := cli.InspectService(ctx, client.CaddyServiceName)
		require.NoError(t, err)
		assert.Len(t, svc.Containers, 3)
		assertServiceMatchesSpec(t, svc, deployment.Spec)

		ctr := svc.Containers[0].Container
		assert.Regexp(t, `^caddy:2\.\d+\.\d+$`, ctr.Config.Image)

		config, err := cli.Caddy.GetConfig(ctx, nil)
		require.NoError(t, err)

		assert.Contains(t, config.Caddyfile, "# Caddyfile autogenerated by Uncloud")
		assert.Contains(t, config.Caddyfile, "handle /.uncloud-verify")
	})

	t.Run("caddy with machine placement", func(t *testing.T) {
		t.Cleanup(func() {
			err := cli.RemoveService(ctx, client.CaddyServiceName)
			if !errors.Is(err, api.ErrNotFound) {
				require.NoError(t, err)
			}
		})

		// Deploy to machine #0.
		deployment, err := cli.NewCaddyDeployment("", "", api.Placement{
			Machines: []string{c.Machines[0].Name},
		})
		require.NoError(t, err)
		image := deployment.Spec.Container.Image

		_, err = deployment.Run(ctx)
		require.NoError(t, err)

		svc, err := cli.InspectService(ctx, client.CaddyServiceName)
		require.NoError(t, err)
		assert.Len(t, svc.Containers, 1)
		assertServiceMatchesSpec(t, svc, deployment.Spec)

		assert.Equal(t, c.Machines[0].ID, svc.Containers[0].MachineID)
		// initialContainerID := svc.Containers[0].Container.ID

		// Deploy to all machines without a placement constraint.
		deployment, err = cli.NewCaddyDeployment(image, "", api.Placement{})
		require.NoError(t, err)

		_, err = deployment.Run(ctx)
		require.NoError(t, err)

		svc, err = cli.InspectService(ctx, client.CaddyServiceName)
		require.NoError(t, err)
		assert.Len(t, svc.Containers, 3)
		assertServiceMatchesSpec(t, svc, deployment.Spec)

		// Initial container on machine #0 should be left unchanged.
		machines := serviceMachines(svc)
		assert.Len(t, machines.ToSlice(), 3, "Expected 1 container on each machine")
		// TODO: update the container spec in-place if only the placement constraint has changed.
		// containers := serviceContainerIDs(svc)
		// assert.True(t, containers.Contains(initialContainerID), "Expected initial container to remain")
	})

	t.Run("caddy and service with custom configs", func(t *testing.T) {
		name := "test-custom-caddy-config"
		t.Cleanup(func() {
			err := cli.RemoveService(ctx, name)
			if !errors.Is(err, api.ErrNotFound) {
				require.NoError(t, err)
			}
			err = cli.RemoveService(ctx, client.CaddyServiceName)
			if !errors.Is(err, api.ErrNotFound) {
				require.NoError(t, err)
			}
		})

		// First deploy a service with custom caddy config before caddy is deployed.
		serviceCaddyfile := `test-custom-caddy-config.example.com {
	reverse_proxy {{upstreams}} {
		import common_proxy
	}
	log
}`
		spec := api.ServiceSpec{
			Name: name,
			Container: api.ContainerSpec{
				Image: "portainer/pause:latest",
			},
			Caddy: &api.CaddySpec{
				Config: serviceCaddyfile,
			},
		}

		deployment := cli.NewDeployment(spec, nil)
		_, err := deployment.Run(ctx)
		require.NoError(t, err)

		svc, err := cli.InspectService(ctx, name)
		require.NoError(t, err)
		assertServiceMatchesSpec(t, svc, spec)

		// Check that the generated Caddyfile contains a comment that user-define configs were skipped.
		var config *pb.GetCaddyConfigResponse
		require.Eventually(t, func() bool {
			config, err = cli.Caddy.GetConfig(ctx, nil)
			if err != nil {
				return false
			}
			return strings.Contains(config.Caddyfile, "# NOTE: User-defined configs for services were skipped")
		}, 5*time.Second, 100*time.Millisecond)

		assert.NotContains(t, config.Caddyfile, "test-custom-caddy-config.example.com {")

		// Now deploy caddy with custom config.
		caddyCaddyfile := `{
	debug
}

myapp.example.com {
	reverse_proxy 1.2.3.4:8000
}`
		caddyDeployment, err := cli.NewCaddyDeployment("", caddyCaddyfile, api.Placement{})
		require.NoError(t, err)

		_, err = caddyDeployment.Run(ctx)
		require.NoError(t, err)

		caddySvc, err := cli.InspectService(ctx, client.CaddyServiceName)
		require.NoError(t, err)
		assertServiceMatchesSpec(t, caddySvc, caddyDeployment.Spec)

		// Wait for the Caddyfile to be regenerated with both custom configs.
		require.Eventually(t, func() bool {
			config, err = cli.Caddy.GetConfig(ctx, nil)
			if err != nil {
				return false
			}
			// Both configs should be present.
			return strings.Contains(config.Caddyfile, caddyCaddyfile) &&
				strings.Contains(config.Caddyfile, "test-custom-caddy-config.example.com")
		}, 5*time.Second, 100*time.Millisecond,
			"Expected both custom configs to be included in the Caddyfile")

		assert.Contains(t, config.Caddyfile, "# Caddyfile autogenerated by Uncloud")
		assert.Contains(t, config.Caddyfile, "handle /.uncloud-verify")
		assert.Contains(t, config.Caddyfile, caddyCaddyfile,
			"Expected user-defined global Caddy config to be included in the Caddyfile")

		ctrIP := svc.Containers[0].Container.UncloudNetworkIP().String()
		renderedServiceCaddyfile := `test-custom-caddy-config.example.com {
	reverse_proxy ` + ctrIP + ` {
		import common_proxy
	}
	log
}`
		assert.Contains(t, config.Caddyfile, renderedServiceCaddyfile,
			"Expected rendered user-defined Caddy config for test service to be included in the Caddyfile")

		assert.NotContains(t, config.Caddyfile, "invalid user-defined configs",
			"Should not have validation failure comments after caddy is deployed")

		// Store the current valid config for later comparison.
		validConfig := config.Caddyfile

		// Now deploy a service with invalid Caddyfile that references missing cert files and check it isn't included.
		invalidServiceName := "test-invalid-caddy-config"
		t.Cleanup(func() {
			err := cli.RemoveService(ctx, invalidServiceName)
			if !errors.Is(err, api.ErrNotFound) {
				require.NoError(t, err)
			}
		})

		invalidCaddyfile := `test-invalid.example.com {
	tls cert.pem key.pem
}`
		invalidSpec := api.ServiceSpec{
			Name: invalidServiceName,
			Container: api.ContainerSpec{
				Image: "portainer/pause:latest",
			},
			Caddy: &api.CaddySpec{
				Config: invalidCaddyfile,
			},
		}

		invalidDeployment := cli.NewDeployment(invalidSpec, nil)
		_, err = invalidDeployment.Run(ctx)
		require.NoError(t, err)

		invalidSvc, err := cli.InspectService(ctx, invalidServiceName)
		require.NoError(t, err)
		assertServiceMatchesSpec(t, invalidSvc, invalidSpec)

		// Wait a bit for any config updates to potentially happen.
		time.Sleep(2 * time.Second)

		// Check that the Caddy config hasn't changed.
		newConfig, err := cli.Caddy.GetConfig(ctx, nil)
		require.NoError(t, err)

		// Compare stable parts of the Caddyfile only (skip autogenerated comment with timestamp).
		_, stableValidConfig, _ := strings.Cut(validConfig, "\n")
		_, stableNewConfig, _ := strings.Cut(newConfig.Caddyfile, "\n")
		assert.Equal(t, stableValidConfig, stableNewConfig,
			"Caddy config should not change when an invalid user-defined Caddy config is deployed")
	})

	t.Run("replicated", func(t *testing.T) {
		t.Parallel()

		name := "test-replicated-deployment"
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
		initialMachines := serviceMachines(svc)
		assert.Len(t, initialMachines.ToSlice(), 2, "Expected 2 containers on 2 different machines")
		initialContainers := serviceContainerIDs(svc)

		// 2. Update the service with a new configuration.
		init := true
		updatedSpec := spec
		updatedSpec.Container.Init = &init
		deployment = cli.NewDeployment(updatedSpec, nil)

		plan, err = deployment.Plan(ctx)
		require.NoError(t, err)
		assert.Len(t, plan.Operations, 2, "Expected 2 replace operations")

		_, err = deployment.Run(ctx)
		require.NoError(t, err)

		svc, err = cli.InspectService(ctx, name)
		require.NoError(t, err)
		assertServiceMatchesSpec(t, svc, updatedSpec)

		// Verify containers are on the same machines as before but the initial containers were replaced.
		machines := serviceMachines(svc)
		assert.ElementsMatch(t, initialMachines.ToSlice(), machines.ToSlice(),
			"Expected containers on the same machines")
		containers := serviceContainerIDs(svc)
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
		machines = serviceMachines(svc)
		assert.Len(t, machines.ToSlice(), 3, "Expected 3 containers on 3 different machines")
		containers = serviceContainerIDs(svc)
		assert.Len(t, containers.Intersect(initialContainers).ToSlice(), 2, "Expected 2 initial containers to remain")

		// 4. Update to 5 replicas with a different configuration.
		initialContainers = containers // Reset container tracking.

		fourReplicaSpec := updatedSpec
		fourReplicaSpec.Container.Command = []string{"updated"}
		fourReplicaSpec.Replicas = 5
		deployment = cli.NewDeployment(fourReplicaSpec, nil)

		plan, err = deployment.Plan(ctx)
		require.NoError(t, err)
		assert.Len(t, plan.Operations, 5, "Expected 3 replace + 2 run operations")

		_, err = deployment.Run(ctx)
		require.NoError(t, err)

		svc, err = cli.InspectService(ctx, name)
		require.NoError(t, err)
		assertServiceMatchesSpec(t, svc, fourReplicaSpec)

		// Verify all existing containers were replaced and new ones are evenly distributed.
		machines = serviceMachines(svc)
		assert.Len(t, machines.ToSlice(), 3, "Expected containers on 3 different machines")
		containers = serviceContainerIDs(svc)
		assert.Empty(t, containers.Intersect(initialContainers).ToSlice(),
			"All existing containers should be replaced")

		machineContainers := serviceContainersByMachine(svc)
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

		containers = serviceContainerIDs(svc)
		assert.ElementsMatch(t, initialContainers.ToSlice(), containers.ToSlice())
	})

	t.Run("replicated with machine placement", func(t *testing.T) {
		t.Parallel()

		name := "test-replicated-deployment-machine-placement"
		t.Cleanup(func() {
			err := cli.RemoveService(ctx, name)
			if !errors.Is(err, api.ErrNotFound) {
				require.NoError(t, err)
			}
		})

		// Create a replicated service with 2 replicas but limit to machines 0 and 1.
		spec := api.ServiceSpec{
			Name: name,
			Mode: api.ServiceModeReplicated,
			Container: api.ContainerSpec{
				Image: "portainer/pause:latest",
			},
			Placement: api.Placement{
				Machines: []string{c.Machines[0].Name, c.Machines[1].Name},
			},
			Replicas: 2,
		}

		deployment := cli.NewDeployment(spec, nil)

		_, err = deployment.Run(ctx)
		require.NoError(t, err)

		// Verify service has 2 containers on machines 0 and 1.
		svc, err := cli.InspectService(ctx, name)
		require.NoError(t, err)
		assertServiceMatchesSpec(t, svc, spec)

		assert.Len(t, svc.Containers, 2)
		machines := serviceMachines(svc)
		assert.ElementsMatch(t, machines.ToSlice(), []string{c.Machines[0].ID, c.Machines[1].ID})

		// Now update the filter to only allow machine 2
		spec.Placement = api.Placement{
			Machines: []string{c.Machines[2].Name},
		}
		deployment = cli.NewDeployment(spec, nil)

		_, err = deployment.Run(ctx)
		require.NoError(t, err)

		// Verify service now has containers only on machine 2.
		svc, err = cli.InspectService(ctx, name)
		require.NoError(t, err)
		assertServiceMatchesSpec(t, svc, spec)

		assert.Len(t, svc.Containers, 2) // Still 2 replicas.
		machines = serviceMachines(svc)
		assert.Equal(t, machines.ToSlice(), []string{c.Machines[2].ID}, "Expected containers on machine 2 only")
	})

	// Deployments with volumes.
	t.Run("replicated with missing volume fails", func(t *testing.T) {
		t.Parallel()

		name := "test-replicated-with-missing-volume"
		spec := api.ServiceSpec{
			Name: name,
			Container: api.ContainerSpec{
				Image: "portainer/pause:latest",
				VolumeMounts: []api.VolumeMount{
					{
						VolumeName:    "non-existent-volume",
						ContainerPath: "/data",
					},
				},
			},
			Volumes: []api.VolumeSpec{
				{
					Name: "non-existent-volume",
					Type: api.VolumeTypeVolume,
				},
			},
		}

		d := deploy.NewDeployment(cli, spec, nil)
		_, err := d.Run(ctx)
		require.Error(t, err, "Deployment should fail when volume doesn't exist")
		require.Contains(t, err.Error(), "no machines available")
		// TODO: implement and check for more details about the failed constraints.
		// require.Contains(t, err.Error(), "volume 'non-existent-volume' not found")
	})

	// Tests that when a volume exists on a single machine, all requested replicas will be deployed to that machine,
	// regardless of how many replicas are requested.
	t.Run("replicated with volume on single machine", func(t *testing.T) {
		t.Parallel()

		serviceName := "test-replicated-with-volume-single-machine"
		volumeName := serviceName
		t.Cleanup(func() {
			removeServices(t, cli, serviceName)
			removeVolumes(t, cli, volumeName)
		})

		vol, err := cli.CreateVolume(ctx, c.Machines[1].Name, volume.CreateOptions{Name: volumeName})
		require.NoError(t, err, "Failed to create test volume")

		spec := api.ServiceSpec{
			Name: serviceName,
			Container: api.ContainerSpec{
				Image: "portainer/pause:latest",
				VolumeMounts: []api.VolumeMount{
					{
						VolumeName:    volumeName,
						ContainerPath: "/data",
					},
				},
			},
			Volumes: []api.VolumeSpec{
				{
					Name: volumeName,
					Type: api.VolumeTypeVolume,
				},
			},
			Replicas: 3,
		}

		d := deploy.NewDeployment(cli, spec, nil)
		_, err = d.Run(ctx)
		require.NoError(t, err)

		svc, err := cli.InspectService(ctx, serviceName)
		require.NoError(t, err)
		assertServiceMatchesSpec(t, svc, spec)

		for _, ctr := range svc.Containers {
			assert.Equal(t, vol.MachineID, ctr.MachineID,
				"All containers should be on the machine where the volume is located")
		}
	})

	// Tests replica distribution across machines that have the same volume.
	t.Run("replicated with volume distributed", func(t *testing.T) {
		t.Parallel()

		serviceName := "test-replicated-with-volume-distributed"
		volumeName := serviceName
		t.Cleanup(func() {
			err := cli.RemoveService(ctx, serviceName)
			if !errors.Is(err, api.ErrNotFound) {
				assert.NoError(t, err)
			}

			err = cli.RemoveVolume(ctx, c.Machines[0].Name, volumeName, false)
			if !errors.Is(err, api.ErrNotFound) {
				assert.NoError(t, err)
			}
			err = cli.RemoveVolume(ctx, c.Machines[1].Name, volumeName, false)
			if !errors.Is(err, api.ErrNotFound) {
				assert.NoError(t, err)
			}
		})

		vol1, err := cli.CreateVolume(ctx, c.Machines[0].Name, volume.CreateOptions{Name: volumeName})
		require.NoError(t, err, "Failed to create volume on first machine")
		vol2, err := cli.CreateVolume(ctx, c.Machines[1].Name, volume.CreateOptions{Name: volumeName})
		require.NoError(t, err, "Failed to create volume on second machine")

		spec := api.ServiceSpec{
			Name: serviceName,
			Container: api.ContainerSpec{
				Image: "portainer/pause:latest",
				VolumeMounts: []api.VolumeMount{
					{
						VolumeName:    volumeName,
						ContainerPath: "/data",
					},
				},
			},
			Volumes: []api.VolumeSpec{
				{
					Name: volumeName,
					Type: api.VolumeTypeVolume,
				},
			},
			Replicas: 3,
		}

		d := deploy.NewDeployment(cli, spec, nil)
		_, err = d.Run(ctx)
		require.NoError(t, err)

		svc, err := cli.InspectService(ctx, serviceName)
		require.NoError(t, err)
		assertServiceMatchesSpec(t, svc, spec)

		machines := serviceMachines(svc)
		assert.ElementsMatch(t, machines.ToSlice(), []string{vol1.MachineID, vol2.MachineID},
			"Containers should be distributed across machines with the same volume")
	})

	// Tests that a service requiring multiple volumes is correctly deployed to a machine that has all of those volumes.
	t.Run("replicated with multiple volumes on single machine", func(t *testing.T) {
		t.Parallel()

		serviceName := "test-replicated-with-multi-volume-single-machine"
		vol1Name := serviceName + "1"
		vol2Name := serviceName + "2"

		t.Cleanup(func() {
			err := cli.RemoveService(ctx, serviceName)
			if !errors.Is(err, api.ErrNotFound) {
				assert.NoError(t, err)
			}

			err = cli.RemoveVolume(ctx, c.Machines[0].Name, vol1Name, false)
			if !errors.Is(err, api.ErrNotFound) {
				assert.NoError(t, err)
			}
			err = cli.RemoveVolume(ctx, c.Machines[1].Name, vol1Name, false)
			if !errors.Is(err, api.ErrNotFound) {
				assert.NoError(t, err)
			}
			err = cli.RemoveVolume(ctx, c.Machines[1].Name, vol2Name, false)
			if !errors.Is(err, api.ErrNotFound) {
				assert.NoError(t, err)
			}
			err = cli.RemoveVolume(ctx, c.Machines[2].Name, vol2Name, false)
			if !errors.Is(err, api.ErrNotFound) {
				assert.NoError(t, err)
			}
		})

		_, err := cli.CreateVolume(ctx, c.Machines[0].Name, volume.CreateOptions{Name: vol1Name})
		require.NoError(t, err)
		_, err = cli.CreateVolume(ctx, c.Machines[1].Name, volume.CreateOptions{Name: vol1Name})
		require.NoError(t, err)
		_, err = cli.CreateVolume(ctx, c.Machines[1].Name, volume.CreateOptions{Name: vol2Name})
		require.NoError(t, err)
		_, err = cli.CreateVolume(ctx, c.Machines[2].Name, volume.CreateOptions{Name: vol2Name})
		require.NoError(t, err)

		spec := api.ServiceSpec{
			Name: serviceName,
			Container: api.ContainerSpec{
				Image: "portainer/pause:latest",
				VolumeMounts: []api.VolumeMount{
					{
						VolumeName:    vol1Name,
						ContainerPath: "/data1",
					},
					{
						VolumeName:    vol2Name,
						ContainerPath: "/data2",
					},
				},
			},
			Volumes: []api.VolumeSpec{
				{
					Name: vol1Name,
					Type: api.VolumeTypeVolume,
				},
				{
					Name: vol2Name,
					Type: api.VolumeTypeVolume,
				},
			},
			Replicas: 3,
		}

		d := deploy.NewDeployment(cli, spec, nil)
		_, err = d.Run(ctx)
		require.NoError(t, err)

		svc, err := cli.InspectService(ctx, serviceName)
		require.NoError(t, err)
		assertServiceMatchesSpec(t, svc, spec)

		machines := serviceMachines(svc)
		assert.ElementsMatch(t, machines.ToSlice(), []string{c.Machines[1].ID},
			"Containers should be deployed to the machine that has both volumes")
	})

	// Tests that a deployment fails when one of multiple required volumes doesn't exist on any machine.
	t.Run("replicated with multiple volumes one missing fails", func(t *testing.T) {
		t.Parallel()

		serviceName := "test-replicated-with-multi-volume-one-missing"
		vol1Name := serviceName + "1"
		vol2Name := serviceName + "2"

		t.Cleanup(func() {
			err := cli.RemoveService(ctx, serviceName)
			if !errors.Is(err, api.ErrNotFound) {
				assert.NoError(t, err)
			}

			err = cli.RemoveVolume(ctx, c.Machines[0].Name, vol1Name, false)
			if !errors.Is(err, api.ErrNotFound) {
				assert.NoError(t, err)
			}
			err = cli.RemoveVolume(ctx, c.Machines[1].Name, vol2Name, false)
			if !errors.Is(err, api.ErrNotFound) {
				assert.NoError(t, err)
			}
		})

		_, err := cli.CreateVolume(ctx, c.Machines[0].Name, volume.CreateOptions{Name: vol1Name})
		require.NoError(t, err, "Failed to create test volume")
		_, err = cli.CreateVolume(ctx, c.Machines[1].Name, volume.CreateOptions{Name: vol2Name})
		require.NoError(t, err, "Failed to create test volume")

		spec := api.ServiceSpec{
			Name: serviceName,
			Container: api.ContainerSpec{
				Image: "portainer/pause:latest",
				VolumeMounts: []api.VolumeMount{
					{
						VolumeName:    vol1Name,
						ContainerPath: "/data1",
					},
					{
						VolumeName:    vol2Name,
						ContainerPath: "/data2",
					},
				},
			},
			Volumes: []api.VolumeSpec{
				{
					Name: vol1Name,
					Type: api.VolumeTypeVolume,
				},
				{
					Name: vol2Name,
					Type: api.VolumeTypeVolume,
				},
			},
		}

		d := deploy.NewDeployment(cli, spec, nil)
		_, err = d.Run(ctx)
		require.Error(t, err, "Deployment should fail when all required volumes don't exist on any machine")
		require.Contains(t, err.Error(), "no machines available")
	})

	// Tests deployment with multiple volumes spread across different machines.
	t.Run("replicated with multiple volumes distributed", func(t *testing.T) {
		t.Parallel()

		serviceName := "test-replicated-with-multi-volume-distributed"
		vol1Name := serviceName + "1"
		vol2Name := serviceName + "2"

		t.Cleanup(func() {
			err := cli.RemoveService(ctx, serviceName)
			if !errors.Is(err, api.ErrNotFound) {
				assert.NoError(t, err)
			}

			for _, machine := range []string{c.Machines[0].Name, c.Machines[1].Name, c.Machines[2].Name} {
				for _, volName := range []string{vol1Name, vol2Name} {
					err = cli.RemoveVolume(ctx, machine, volName, false)
					if !errors.Is(err, api.ErrNotFound) {
						assert.NoError(t, err)
					}
				}
			}
		})

		// Machine 0: vol1, vol2
		// Machine 1: vol1, vol2
		// Machine 2: vol1
		for i := 0; i < 3; i++ {
			_, err := cli.CreateVolume(ctx, c.Machines[i].Name, volume.CreateOptions{Name: vol1Name})
			require.NoError(t, err)
			if i < 2 {
				_, err = cli.CreateVolume(ctx, c.Machines[i].Name, volume.CreateOptions{Name: vol2Name})
				require.NoError(t, err)
			}
		}

		spec := api.ServiceSpec{
			Name: serviceName,
			Container: api.ContainerSpec{
				Image: "portainer/pause:latest",
				VolumeMounts: []api.VolumeMount{
					{
						VolumeName:    vol1Name,
						ContainerPath: "/data1",
					},
					{
						VolumeName:    vol2Name,
						ContainerPath: "/data2",
					},
				},
			},
			Volumes: []api.VolumeSpec{
				{
					Name: vol1Name,
					Type: api.VolumeTypeVolume,
				},
				{
					Name: vol2Name,
					Type: api.VolumeTypeVolume,
				},
			},
			Replicas: 3, // Request more replicas than machines with both volumes.
		}

		d := deploy.NewDeployment(cli, spec, nil)
		_, err = d.Run(ctx)
		require.NoError(t, err)

		svc, err := cli.InspectService(ctx, serviceName)
		require.NoError(t, err)
		assertServiceMatchesSpec(t, svc, spec)

		machines := serviceMachines(svc)
		assert.ElementsMatch(t, machines.ToSlice(), []string{c.Machines[0].ID, c.Machines[1].ID},
			"All containers should be distributed across machines with both volumes (#0 and #1)")
	})

	// Tests that a global deployment fails when the required volume doesn't exist.
	t.Run("global with missing volume fails", func(t *testing.T) {
		t.Parallel()

		serviceName := "test-global-with-missing-volume"
		spec := api.ServiceSpec{
			Name: serviceName,
			Mode: api.ServiceModeGlobal,
			Container: api.ContainerSpec{
				Image: "portainer/pause:latest",
				VolumeMounts: []api.VolumeMount{
					{
						VolumeName:    "non-existent-volume",
						ContainerPath: "/data",
					},
				},
			},
			Volumes: []api.VolumeSpec{
				{
					Name: "non-existent-volume",
					Type: api.VolumeTypeVolume,
				},
			},
		}

		d := deploy.NewDeployment(cli, spec, nil)
		_, err = d.Run(ctx)
		require.Error(t, err, "Global deployment should fail when required volume doesn't exist")
		require.Contains(t, err.Error(), "no machines available")
	})

	// Tests that a global deployment with a volume on a single machine only deploys to that machine.
	t.Run("global with volume on single machine", func(t *testing.T) {
		t.Parallel()

		serviceName := "test-global-with-volume-single-machine"
		volumeName := serviceName
		t.Cleanup(func() {
			err := cli.RemoveService(ctx, serviceName)
			if !errors.Is(err, api.ErrNotFound) {
				assert.NoError(t, err)
			}

			err = cli.RemoveVolume(ctx, c.Machines[1].Name, volumeName, false)
			if !errors.Is(err, api.ErrNotFound) {
				assert.NoError(t, err)
			}
		})

		vol, err := cli.CreateVolume(ctx, c.Machines[1].Name, volume.CreateOptions{Name: volumeName})
		require.NoError(t, err)

		spec := api.ServiceSpec{
			Name: serviceName,
			Mode: api.ServiceModeGlobal,
			Container: api.ContainerSpec{
				Image: "portainer/pause:latest",
				VolumeMounts: []api.VolumeMount{
					{
						VolumeName:    volumeName,
						ContainerPath: "/data",
					},
				},
			},
			Volumes: []api.VolumeSpec{
				{
					Name: volumeName,
					Type: api.VolumeTypeVolume,
				},
			},
		}

		d := deploy.NewDeployment(cli, spec, nil)
		_, err = d.Run(ctx)
		require.NoError(t, err)

		svc, err := cli.InspectService(ctx, serviceName)
		require.NoError(t, err)
		assertServiceMatchesSpec(t, svc, spec)

		assert.Equal(t, vol.MachineID, svc.Containers[0].MachineID,
			"Container should be on the machine where the volume exists")
	})

	// Tests global deployment when the required volume exists on multiple machines.
	t.Run("global with volume on multiple machines", func(t *testing.T) {
		t.Parallel()

		serviceName := "test-global-with-volume-multi-machine"
		volumeName := serviceName
		t.Cleanup(func() {
			err := cli.RemoveService(ctx, serviceName)
			if !errors.Is(err, api.ErrNotFound) {
				assert.NoError(t, err)
			}

			for _, machineName := range []string{c.Machines[0].Name, c.Machines[2].Name} {
				err = cli.RemoveVolume(ctx, machineName, volumeName, false)
				if !errors.Is(err, api.ErrNotFound) {
					assert.NoError(t, err)
				}
			}
		})

		_, err := cli.CreateVolume(ctx, c.Machines[0].Name, volume.CreateOptions{Name: volumeName})
		require.NoError(t, err)
		_, err = cli.CreateVolume(ctx, c.Machines[2].Name, volume.CreateOptions{Name: volumeName})
		require.NoError(t, err)

		spec := api.ServiceSpec{
			Name: serviceName,
			Mode: api.ServiceModeGlobal,
			Container: api.ContainerSpec{
				Image: "portainer/pause:latest",
				VolumeMounts: []api.VolumeMount{
					{
						VolumeName:    volumeName,
						ContainerPath: "/data",
					},
				},
			},
			Volumes: []api.VolumeSpec{
				{
					Name: volumeName,
					Type: api.VolumeTypeVolume,
				},
			},
		}

		d := deploy.NewDeployment(cli, spec, nil)
		_, err = d.Run(ctx)
		require.NoError(t, err)

		svc, err := cli.InspectService(ctx, serviceName)
		require.NoError(t, err)
		assertServiceMatchesSpec(t, svc, spec)

		assert.Len(t, svc.Containers, 2, "Should have created one container for each machine with the volume")
		machines := serviceMachines(svc)
		assert.ElementsMatch(t, machines.ToSlice(), []string{c.Machines[0].ID, c.Machines[2].ID},
			"Containers should be on machines where the volume exists")
	})

	t.Run("list images for replicated service", func(t *testing.T) {
		t.Parallel()

		serviceName := "test-list-images-replicated"
		uniqueImage := "portainer/pause:3.9" // Unique image not used by other tests.
		t.Cleanup(func() {
			err := cli.RemoveService(ctx, serviceName)
			if !errors.Is(err, api.ErrNotFound) {
				require.NoError(t, err)
			}
		})

		spec := api.ServiceSpec{
			Name: serviceName,
			Mode: api.ServiceModeReplicated,
			Container: api.ContainerSpec{
				Image: uniqueImage,
			},
			Replicas: 2,
		}

		deployment := cli.NewDeployment(spec, nil)
		_, err = deployment.Run(ctx)
		require.NoError(t, err)

		svc, err := cli.InspectService(ctx, serviceName)
		require.NoError(t, err)
		assertServiceMatchesSpec(t, svc, spec)

		// Get the machine IDs where containers are running.
		machinesWithContainers := serviceMachines(svc)
		assert.Len(t, machinesWithContainers.ToSlice(), 2, "Containers should be on 2 different machines")

		var machineWithoutContainer string
		for _, m := range c.Machines {
			if !machinesWithContainers.Contains(m.ID) {
				machineWithoutContainer = m.ID
				break
			}
		}
		assert.NotEmpty(t, machineWithoutContainer, "Should have found a machine without container")

		machineImages, err := cli.ListImages(ctx, api.ImageFilter{})
		require.NoError(t, err)
		assert.Len(t, machineImages, 3, "Should get images from all 3 machines")

		for _, mi := range machineImages {
			// Checking only DockerImages because the machines in ucind cluster don't use the containerd image store.
			if !machinesWithContainers.Contains(mi.Metadata.Machine) {
				// This is the machine without service containers, it should not have the unique image.
				for _, img := range mi.Images {
					assert.NotContains(t, img.RepoTags, uniqueImage)
				}
				continue
			}

			// Check if the unique image is present on the machine where a service container is running.
			hasImage := false
			for _, img := range mi.Images {
				if slices.Contains(img.RepoTags, uniqueImage) {
					hasImage = true
					break
				}
			}
			assert.True(t, hasImage, "Machine %s with container should have image %s",
				mi.Metadata.Machine, uniqueImage)
		}
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
		assertContainerMatchesSpec(t, mc.Container, spec)
	})

	t.Run("create container with full spec", func(t *testing.T) {
		t.Parallel()

		serviceID := newServiceID()
		init := true
		spec := api.ServiceSpec{
			Name: "container-spec-full",
			Mode: api.ServiceModeGlobal,
			Container: api.ContainerSpec{
				// TODO: Add the latest implemented fields to this spec and update assertContainerMatchesSpec.
				Command: []string{"sleep", "infinity"},
				// Extra slashes is not a typo, it changes the spec but Linux ignores them and uses the default /pause.
				Entrypoint: []string{"///pause"},
				Env: map[string]string{
					"VAR":   "value",
					"EMPTY": "",
					"BOOL":  "true",
					"":      "ignored",
				},
				Healthcheck: &api.HealthcheckSpec{
					Test:          []string{"CMD-SHELL", "exit 0"},
					Interval:      1*time.Minute + 30*time.Second,
					Timeout:       10 * time.Second,
					Retries:       5,
					StartPeriod:   15 * time.Second,
					StartInterval: 2 * time.Second,
				},
				Image: "portainer/pause:latest",
				Init:  &init,
				LogDriver: &api.LogDriver{
					Name: "json-file",
					Options: map[string]string{
						"max-size": "1m",
					},
				},
				Resources: api.ContainerResources{
					CPU:               100 * api.MilliCore,
					Memory:            20 * units.MiB,
					MemoryReservation: 10 * 1024 * 1024,
				},
				User: "nobody:nobody",
				VolumeMounts: []api.VolumeMount{
					{
						VolumeName:    "hostpath",
						ContainerPath: "/volumes/hostpath",
						ReadOnly:      true,
					},
					{
						VolumeName:    "container-spec-full-default-volume",
						ContainerPath: "/volumes/default-volume",
					},
					{
						VolumeName:    "custom-volume",
						ContainerPath: "/volumes/custom-volume",
						ReadOnly:      true,
					},
					{
						VolumeName:    "tmpfs",
						ContainerPath: "/volumes/tmpfs",
					},
				},
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
			Volumes: []api.VolumeSpec{
				{
					Name: "hostpath",
					Type: api.VolumeTypeBind,
					BindOptions: &api.BindOptions{
						HostPath:       "/tmp/container-spec-full/host/path",
						CreateHostPath: true,
					},
				},
				{
					Name: "container-spec-full-default-volume",
					Type: api.VolumeTypeVolume,
				},
				{
					Name: "custom-volume",
					Type: api.VolumeTypeVolume,
					VolumeOptions: &api.VolumeOptions{
						Driver: &mount.Driver{
							Name: "local",
						},
						Labels: map[string]string{
							"key": "value",
						},
						Name:   "container-spec-full-custom-volume",
						NoCopy: true,
					},
				},
				{
					Name: "tmpfs",
					Type: api.VolumeTypeTmpfs,
				},
			},
		}

		// Create the volumes before creating the container as they must be managed externally.
		volumeNames := []string{
			"container-spec-full-default-volume",
			"container-spec-full-custom-volume",
		}
		for _, name := range volumeNames {
			_, err = cli.CreateVolume(ctx, c.Machines[0].Name, volume.CreateOptions{Name: name})
			require.NoError(t, err)

			t.Cleanup(func() {
				err := cli.RemoveVolume(ctx, c.Machines[0].Name, name, false)
				if !errors.Is(err, api.ErrNotFound) {
					require.NoError(t, err)
				}
			})
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
		assertContainerMatchesSpec(t, mc.Container, spec)
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

	t.Run("3 replicas with volume auto-created", func(t *testing.T) {
		t.Parallel()

		name := "test-3-replicas-volume-auto-created"
		volumeName := name
		t.Cleanup(func() {
			err := cli.RemoveService(ctx, name)
			if !errors.Is(err, api.ErrNotFound) {
				assert.NoError(t, err)
			}

			volumes, err := cli.ListVolumes(ctx, &api.VolumeFilter{Names: []string{volumeName}})
			require.NoError(t, err)
			for _, v := range volumes {
				err = cli.RemoveVolume(ctx, v.MachineID, v.Volume.Name, false)
				assert.NoError(t, err)
			}
		})

		volumes, err := cli.ListVolumes(ctx, &api.VolumeFilter{Names: []string{volumeName}})
		require.NoError(t, err)
		assert.Len(t, volumes, 0, "Volume should not exist before service creation")

		spec := api.ServiceSpec{
			Name: name,
			Container: api.ContainerSpec{
				Image: "portainer/pause:latest",
				VolumeMounts: []api.VolumeMount{
					{
						VolumeName:    volumeName,
						ContainerPath: "/data",
					},
				},
			},
			Volumes: []api.VolumeSpec{
				{
					Name: volumeName,
					Type: api.VolumeTypeVolume,
				},
			},
			Replicas: 3,
		}
		resp, err := cli.RunService(ctx, spec)
		require.NoError(t, err)

		svc, err := cli.InspectService(ctx, resp.ID)
		require.NoError(t, err)
		assertServiceMatchesSpec(t, svc, spec)

		volumes, err = cli.ListVolumes(ctx, &api.VolumeFilter{Names: []string{volumeName}})
		require.NoError(t, err)
		assert.Len(t, volumes, 1, "Volume should be created automatically")
		assert.Equal(t, volumeName, volumes[0].Volume.Name)

		machines := serviceMachines(svc)
		assert.Equal(t, []string{volumes[0].MachineID}, machines.ToSlice(),
			"Replicas should be on the same machine as the volume")
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

	t.Run("service logs", func(t *testing.T) {
		t.Parallel()

		name := "test-service-logs"
		t.Cleanup(func() {
			err := cli.RemoveService(ctx, name)
			if !errors.Is(err, api.ErrNotFound) {
				require.NoError(t, err)
			}
		})

		spec := api.ServiceSpec{
			Name: name,
			Container: api.ContainerSpec{
				Image: "busybox:1.37.0-musl",
				Command: []string{
					"sh",
					"-c",
					"trap 'exit 0' TERM INT; " +
						"echo \"Hello from $HOSTNAME\"; sleep 0.5;" +
						"echo \"Hello stderr from $HOSTNAME\" >&2; " +
						"while true; do sleep 1; done",
				},
			},
			Replicas: 3,
		}
		deployment := cli.NewDeployment(spec, nil)
		_, err := deployment.Run(ctx)
		require.NoError(t, err)

		svc, err := cli.InspectService(ctx, name)
		require.NoError(t, err)
		assertServiceMatchesSpec(t, svc, spec)

		machines := serviceMachines(svc)
		assert.Len(t, machines.ToSlice(), 3, "should have containers on all 3 machines")

		time.Sleep(1 * time.Second) // Wait for containers to produce the logs.

		logsSvc, stream, err := cli.ServiceLogs(ctx, name, api.ServiceLogsOptions{})
		require.NoError(t, err)
		assertServiceMatchesSpec(t, logsSvc, spec)

		collectLogs := func(stream <-chan api.ServiceLogEntry) []string {
			var entries []string
			timeout := time.After(5 * time.Second)

			for {
				select {
				case entry, ok := <-stream:
					if !ok {
						return entries
					}

					require.NoError(t, entry.Err)
					if entry.Stream == api.LogStreamStdout || entry.Stream == api.LogStreamStderr {
						entries = append(entries, string(entry.Message))
					}
				case <-timeout:
					require.FailNow(t, "timed out waiting for logs")
				}
			}
		}

		logs := collectLogs(stream)
		require.Len(t, logs, 6, "should have 6 log entries (1 stdout and 1 stderr from each of 3 replicas)")

		for _, ctr := range svc.Containers {
			stdoutMsg := fmt.Sprintf("Hello from %s\n", ctr.Container.Name)
			stderrMsg := fmt.Sprintf("Hello stderr from %s\n", ctr.Container.Name)

			assert.Contains(t, logs, stdoutMsg)
			assert.Contains(t, logs, stderrMsg)

			outIdx := slices.Index(logs, stdoutMsg)
			errIdx := slices.Index(logs, stderrMsg)
			assert.Less(t, outIdx, errIdx, "stdout log should appear before stderr log")
		}

		// Test filter logs by machine ID.
		machineID := c.Machines[0].ID
		_, machineStream, err := cli.ServiceLogs(ctx, name, api.ServiceLogsOptions{
			Machines: []string{machineID},
		})
		require.NoError(t, err)

		machineLogs := collectLogs(machineStream)
		require.Len(t, machineLogs, 2, "should have 2 log entries (1 stdout and 1 stderr) from the filtered machine")

		ctrName := ""
		for _, ctr := range svc.Containers {
			if ctr.MachineID == machineID {
				ctrName = ctr.Container.Name
				break
			}
		}
		require.NotEmpty(t, ctrName, "should have found container on the target machine")

		stdoutMsg := fmt.Sprintf("Hello from %s\n", ctrName)
		stderrMsg := fmt.Sprintf("Hello stderr from %s\n", ctrName)

		assert.Equal(t, []string{stdoutMsg, stderrMsg}, machineLogs)

		// Test non-existent machine filter returns error.
		_, _, err = cli.ServiceLogs(ctx, name, api.ServiceLogsOptions{
			Machines: []string{"non-existent-machine"},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "machines not found")
	})
}

package e2e

import (
	"context"
	"testing"

	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/volume"
	"github.com/psviderski/uncloud/internal/ucind"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client/compose"
	"github.com/psviderski/uncloud/pkg/client/deploy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComposeDeployment(t *testing.T) {
	t.Parallel()

	clusterName := "ucind-test.compose-deploy"
	ctx := context.Background()
	c, _ := createTestCluster(t, clusterName, ucind.CreateClusterOptions{Machines: 3}, true)

	cli, err := c.Machines[0].Connect(ctx)
	require.NoError(t, err)

	t.Run("basic with published ports", func(t *testing.T) {
		t.Parallel()

		name := "test-compose-basic"
		t.Cleanup(func() {
			removeServices(t, cli, name)
		})

		project, err := compose.LoadProject(ctx, []string{"fixtures/compose-basic.yaml"})
		require.NoError(t, err)

		deployment, err := compose.NewDeployment(ctx, cli, project)
		require.NoError(t, err)

		plan, err := deployment.Plan(ctx)
		require.NoError(t, err)
		assert.Len(t, plan.Operations, 1, "Expected 1 service to deploy")

		err = deployment.Run(ctx)
		require.NoError(t, err)

		svc, err := cli.InspectService(ctx, name)
		require.NoError(t, err)

		expectedSpec := api.ServiceSpec{
			Name: name,
			Mode: api.ServiceModeReplicated,
			Container: api.ContainerSpec{
				Env: map[string]string{
					"VAR":   "value",
					"BOOL":  "true",
					"EMPTY": "",
				},
				Image: "portainer/pause:3.9",
			},
			Ports: []api.PortSpec{
				{
					Hostname:      "basic.example.com",
					ContainerPort: 80,
					Protocol:      api.ProtocolHTTPS,
					Mode:          api.PortModeIngress,
				},
			},
			Replicas: 1,
		}
		assertServiceMatchesSpec(t, svc, expectedSpec)
	})

	t.Run("multi-service deployment with redeploy and recreate", func(t *testing.T) {
		t.Parallel()

		serviceNames := []string{
			"test-compose-multi-web",
			"test-compose-multi-api",
			"test-compose-multi-worker",
		}
		t.Cleanup(func() {
			removeServices(t, cli, serviceNames...)
		})

		// Initial deployment.
		project, err := compose.LoadProject(ctx, []string{"fixtures/compose-multi-service.yaml"})
		require.NoError(t, err)

		deployment, err := compose.NewDeployment(ctx, cli, project)
		require.NoError(t, err)

		plan, err := deployment.Plan(ctx)
		require.NoError(t, err)
		assert.Len(t, plan.Operations, 3, "Expected 3 services to deploy")

		err = deployment.Run(ctx)
		require.NoError(t, err)

		// Verify web service.
		webSvc, err := cli.InspectService(ctx, "test-compose-multi-web")
		require.NoError(t, err)
		expectedWebSpec := api.ServiceSpec{
			Name: "test-compose-multi-web",
			Mode: api.ServiceModeReplicated,
			Container: api.ContainerSpec{
				Env: map[string]string{
					"SERVICE": "web",
					"VERSION": "1.0",
				},
				Image: "portainer/pause:3.9",
			},
			Ports: []api.PortSpec{
				{
					Hostname:      "multi.example.com",
					ContainerPort: 80,
					Protocol:      api.ProtocolHTTPS,
					Mode:          api.PortModeIngress,
				},
			},
			Replicas: 2,
		}
		assertServiceMatchesSpec(t, webSvc, expectedWebSpec)

		// Verify api service.
		apiSvc, err := cli.InspectService(ctx, "test-compose-multi-api")
		require.NoError(t, err)
		expectedApiSpec := api.ServiceSpec{
			Name: "test-compose-multi-api",
			Mode: api.ServiceModeReplicated,
			Container: api.ContainerSpec{
				Env: map[string]string{
					"SERVICE": "api",
					"PORT":    "8080",
				},
				Image: "portainer/pause:3.9",
			},
			Replicas: 3,
		}
		assertServiceMatchesSpec(t, apiSvc, expectedApiSpec)

		// Verify worker service.
		workerSvc, err := cli.InspectService(ctx, "test-compose-multi-worker")
		require.NoError(t, err)
		expectedWorkerSpec := api.ServiceSpec{
			Name: "test-compose-multi-worker",
			Mode: api.ServiceModeReplicated,
			Container: api.ContainerSpec{
				Env: map[string]string{
					"SERVICE":     "worker",
					"CONCURRENCY": "5",
				},
				Image: "portainer/pause:3.9",
			},
			Replicas: 1,
		}
		assertServiceMatchesSpec(t, workerSvc, expectedWorkerSpec)

		// Save container IDs for later verification.
		containers := serviceContainerIDs(webSvc).
			Union(serviceContainerIDs(apiSvc)).
			Union(serviceContainerIDs(workerSvc))

		// Redeploy without changes - should be up to date.
		redeploy, err := compose.NewDeployment(ctx, cli, project)
		require.NoError(t, err)

		redeployPlan, err := redeploy.Plan(ctx)
		require.NoError(t, err)
		assert.Len(t, redeployPlan.Operations, 0, "Expected no operations - deployment should be up to date")

		// Deploy with ForceRecreate - should recreate all service containers.
		strategy := &deploy.RollingStrategy{ForceRecreate: true}
		recreateDeploy, err := compose.NewDeploymentWithStrategy(ctx, cli, project, strategy)
		require.NoError(t, err)

		recreatePlan, err := recreateDeploy.Plan(ctx)
		require.NoError(t, err)
		assert.Len(t, recreatePlan.Operations, 3, "Expected 3 services to be recreated")

		err = recreateDeploy.Run(ctx)
		require.NoError(t, err)

		// Verify services match the expected specs after recreate.
		webSvcAfter, err := cli.InspectService(ctx, "test-compose-multi-web")
		require.NoError(t, err)
		assertServiceMatchesSpec(t, webSvcAfter, expectedWebSpec)

		apiSvcAfter, err := cli.InspectService(ctx, "test-compose-multi-api")
		require.NoError(t, err)
		assertServiceMatchesSpec(t, apiSvcAfter, expectedApiSpec)

		workerSvcAfter, err := cli.InspectService(ctx, "test-compose-multi-worker")
		require.NoError(t, err)
		assertServiceMatchesSpec(t, workerSvcAfter, expectedWorkerSpec)

		// Verify that all containers have been recreated.
		afterContainers := serviceContainerIDs(webSvcAfter).
			Union(serviceContainerIDs(apiSvcAfter)).
			Union(serviceContainerIDs(workerSvcAfter))
		assert.NotEqual(t, containers.ToSlice(), afterContainers.ToSlice(),
			"Expected containers to be recreated after deployment with ForceRecreate strategy")
	})

	t.Run("multiple services with volumes", func(t *testing.T) {
		t.Parallel()

		t.Cleanup(func() {
			removeServices(
				t,
				cli,
				"test-compose-volumes-service1",
				"test-compose-volumes-service2",
				"test-compose-volumes-service3",
			)
			removeVolumes(
				t,
				cli,
				"test-compose-volumes-data1",
				"test-compose-volumes-data2",
				"test-compose-volumes-external",
			)
		})

		project, err := compose.LoadProject(ctx, []string{"fixtures/compose-volumes.yaml"})
		require.NoError(t, err)

		deployment, err := compose.NewDeployment(ctx, cli, project)
		require.NoError(t, err)

		_, err = deployment.Plan(ctx)
		require.ErrorContains(t, err, "external volumes not found: 'test-compose-volumes-external'")

		externalVolumeOpts := volume.CreateOptions{Name: "test-compose-volumes-external"}
		externalVolume, err := cli.CreateVolume(ctx, c.Machines[2].Name, externalVolumeOpts)
		require.NoError(t, err)

		// Recreate the deployment as it caches the cluster state.
		deployment, err = compose.NewDeployment(ctx, cli, project)
		require.NoError(t, err)

		plan, err := deployment.Plan(ctx)
		require.NoError(t, err)
		assert.Len(t, plan.Operations, 5, "Expected 2 volumes creation and 3 services to deploy")

		err = deployment.Run(ctx)
		require.NoError(t, err)

		// Verify data1 and data2 volumes have been created.
		volumes, err := cli.ListVolumes(ctx, &api.VolumeFilter{
			Names: []string{"test-compose-volumes-data1", "test-compose-volumes-data2"},
		})
		require.NoError(t, err)
		assert.Len(t, volumes, 2, "Expected 2 volumes to be created")
		// Check that the volumes have the correct labels
		assert.Equal(t, map[string]string{"uncloud.managed": ""}, volumes[0].Volume.Labels)
		assert.Equal(t, map[string]string{"uncloud.managed": ""}, volumes[1].Volume.Labels)

		data1Volume, data2Volume := volumes[0], volumes[1]
		if volumes[0].Volume.Name == "test-compose-volumes-data2" {
			data1Volume, data2Volume = volumes[1], volumes[0]
		}
		assert.Equal(t, c.Machines[2].ID, data2Volume.MachineID,
			"data2 volume must be on machine #2 as service3 shares both data2 and external volumes")

		service1, err := cli.InspectService(ctx, "test-compose-volumes-service1")
		require.NoError(t, err)

		expectedSpec1 := api.ServiceSpec{
			Name: "test-compose-volumes-service1",
			Container: api.ContainerSpec{
				Image: "portainer/pause:3.9",
				VolumeMounts: []api.VolumeMount{
					{
						VolumeName:    "test-compose-volumes-data1",
						ContainerPath: "/data1",
					},
					{
						VolumeName:    "bind-bb6aed1683cea1e0a1ae5cd227aacd0734f2f87f7a78fcf1baeff978ce300b90",
						ContainerPath: "/host/etc/passwd",
						ReadOnly:      true,
					},
				},
			},
			Volumes: []api.VolumeSpec{
				{
					Name: "test-compose-volumes-data1",
					Type: api.VolumeTypeVolume,
				},
				{
					Name: "bind-bb6aed1683cea1e0a1ae5cd227aacd0734f2f87f7a78fcf1baeff978ce300b90",
					Type: api.VolumeTypeBind,
					BindOptions: &api.BindOptions{
						HostPath:       "/etc/passwd",
						CreateHostPath: true,
					},
				},
			},
			Replicas: 3,
		}
		assertServiceMatchesSpec(t, service1, expectedSpec1)
		service1Machines := serviceMachines(service1)
		assert.Equal(t, service1Machines.ToSlice(), []string{data1Volume.MachineID},
			"service1 should be on the same machine as data1 volume")

		service2, err := cli.InspectService(ctx, "test-compose-volumes-service2")
		require.NoError(t, err)

		expectedSpec2 := api.ServiceSpec{
			Name: "test-compose-volumes-service2",
			Container: api.ContainerSpec{
				Image: "portainer/pause:3.9",
				VolumeMounts: []api.VolumeMount{
					{
						VolumeName:    "test-compose-volumes-data2-alias",
						ContainerPath: "/data2/long/syntax",
					},
				},
			},
			Volumes: []api.VolumeSpec{
				{
					Name: "test-compose-volumes-data2-alias",
					Type: api.VolumeTypeVolume,
					VolumeOptions: &api.VolumeOptions{
						Name: "test-compose-volumes-data2",
						Driver: &mount.Driver{
							Name: "local",
						},
					},
				},
			},
			Replicas: 2,
		}
		assertServiceMatchesSpec(t, service2, expectedSpec2)
		service2Machines := serviceMachines(service2)
		assert.Equal(t, service2Machines.ToSlice(), []string{data2Volume.MachineID},
			"service2 replicas should be on the same machine as data2 volume")

		service3, err := cli.InspectService(ctx, "test-compose-volumes-service3")
		require.NoError(t, err)

		expectedSpec3 := api.ServiceSpec{
			Name: "test-compose-volumes-service3",
			Container: api.ContainerSpec{
				Image: "portainer/pause:3.9",
				VolumeMounts: []api.VolumeMount{
					{
						VolumeName:    "test-compose-volumes-data2-alias",
						ContainerPath: "/data2",
					},
					{
						VolumeName:    "test-compose-volumes-external",
						ContainerPath: "/external",
						ReadOnly:      true,
					},
				},
			},
			Volumes: []api.VolumeSpec{
				{
					Name: "test-compose-volumes-data2-alias",
					Type: api.VolumeTypeVolume,
					VolumeOptions: &api.VolumeOptions{
						Name: "test-compose-volumes-data2",
						Driver: &mount.Driver{
							Name: "local",
						},
					},
				},
				{
					Name: "test-compose-volumes-external",
					Type: api.VolumeTypeVolume,
				},
			},
			Replicas: 1,
		}
		assertServiceMatchesSpec(t, service3, expectedSpec3)
		assert.Equal(t, externalVolume.MachineID, service3.Containers[0].MachineID,
			"service3 should be on the same machine as external volume")

		// Verify deployment is up-to-date.
		deployment, err = compose.NewDeployment(ctx, cli, project)
		require.NoError(t, err)

		plan, err = deployment.Plan(ctx)
		require.NoError(t, err)
		assert.Len(t, plan.Operations, 0, "Expected no new operations after deployment")
	})

	t.Run("x-machines placement constraint", func(t *testing.T) {
		t.Parallel()

		name := "test-compose-placement"
		t.Cleanup(func() {
			removeServices(t, cli, name)
		})

		project, err := compose.LoadProject(ctx, []string{"fixtures/compose-placement.yaml"})
		require.NoError(t, err)

		deployment, err := compose.NewDeployment(ctx, cli, project)
		require.NoError(t, err)

		plan, err := deployment.Plan(ctx)
		require.NoError(t, err)
		assert.Len(t, plan.Operations, 1, "Expected 1 service to deploy")

		err = deployment.Run(ctx)
		require.NoError(t, err)

		svc, err := cli.InspectService(ctx, name)
		require.NoError(t, err)

		expectedSpec := api.ServiceSpec{
			Name: name,
			Mode: api.ServiceModeReplicated,
			Container: api.ContainerSpec{
				Env: map[string]string{
					"VAR":   "value",
					"BOOL":  "true",
					"EMPTY": "",
				},
				Image: "portainer/pause:3.9",
			},
			Placement: api.Placement{
				Machines: []string{"machine-2", "machine-3"},
			},
			Replicas: 2,
		}
		assertServiceMatchesSpec(t, svc, expectedSpec)

		// Verify that containers are only deployed on specified machines
		// Since we only specified 2 machines in x-machines and have 2 replicas,
		// and the cluster has 3 machines, the third machine should have no containers
		serviceMachines := serviceMachines(svc)
		assert.Len(t, serviceMachines.ToSlice(), 2, "Service should only be on 2 machines")

		// Verify machines match the expected machine IDs (machine-2 = c.Machines[1], machine-3 = c.Machines[2])
		assert.ElementsMatch(t, serviceMachines.ToSlice(), []string{c.Machines[1].ID, c.Machines[2].ID},
			"Service containers should only be on machines 2 and 3")
	})

	t.Run("x-machines placement constraint with non-existing machine", func(t *testing.T) {
		t.Parallel()

		name := "test-compose-placement-nonexistent"
		t.Cleanup(func() {
			removeServices(t, cli, name)
		})

		project, err := compose.LoadProject(ctx, []string{"fixtures/compose-placement-nonexistent.yaml"})
		require.NoError(t, err)

		deployment, err := compose.NewDeployment(ctx, cli, project)
		require.NoError(t, err)

		plan, err := deployment.Plan(ctx)
		require.NoError(t, err)
		assert.Len(t, plan.Operations, 1, "Expected 1 service to deploy")

		err = deployment.Run(ctx)
		require.NoError(t, err)

		svc, err := cli.InspectService(ctx, name)
		require.NoError(t, err)

		expectedSpec := api.ServiceSpec{
			Name: name,
			Mode: api.ServiceModeReplicated,
			Container: api.ContainerSpec{
				Image: "portainer/pause:3.9",
			},
			Placement: api.Placement{
				Machines: []string{"machine-2", "nonexistent-machine"},
			},
			Replicas: 2,
		}
		assertServiceMatchesSpec(t, svc, expectedSpec)

		// Verify that containers are deployed only on existing machines
		// Non-existent machine names should be ignored by the scheduler
		serviceMachines := serviceMachines(svc)

		// Should only deploy on machine-2 since nonexistent-machine doesn't exist
		// The scheduler should intersect placement constraints with available machines
		assert.Len(t, serviceMachines.ToSlice(), 1, "Service should only be on 1 existing machine")
		assert.ElementsMatch(t, serviceMachines.ToSlice(), []string{c.Machines[1].ID},
			"Service containers should only be on machine-2 (existing machine)")
	})

	t.Run("x-machines placement constraint with comma-separated string", func(t *testing.T) {
		t.Parallel()

		name := "test-compose-placement-comma"
		t.Cleanup(func() {
			removeServices(t, cli, name)
		})

		project, err := compose.LoadProject(ctx, []string{"fixtures/compose-placement-comma.yaml"})
		require.NoError(t, err)

		deployment, err := compose.NewDeployment(ctx, cli, project)
		require.NoError(t, err)

		plan, err := deployment.Plan(ctx)
		require.NoError(t, err)
		assert.Len(t, plan.Operations, 1, "Expected 1 service to deploy")

		err = deployment.Run(ctx)
		require.NoError(t, err)

		svc, err := cli.InspectService(ctx, name)
		require.NoError(t, err)

		expectedSpec := api.ServiceSpec{
			Name: name,
			Mode: api.ServiceModeReplicated,
			Container: api.ContainerSpec{
				Image: "portainer/pause:3.9",
			},
			Placement: api.Placement{
				Machines: []string{"machine-1", "machine-3"},
			},
			Replicas: 2,
		}
		assertServiceMatchesSpec(t, svc, expectedSpec)

		// Verify that containers are deployed on specified machines from comma-separated list
		serviceMachines := serviceMachines(svc)
		assert.Len(t, serviceMachines.ToSlice(), 2, "Service should be on 2 machines")

		// Verify machines match the expected machine IDs (machine-1 = c.Machines[0], machine-3 = c.Machines[2])
		assert.ElementsMatch(t, serviceMachines.ToSlice(), []string{c.Machines[0].ID, c.Machines[2].ID},
			"Service containers should be on machines 1 and 3 from comma-separated list")
	})

	// Catches regression: https://github.com/psviderski/uncloud/issues/176
	t.Run("plan new deployment with volumes and recreate strategy", func(t *testing.T) {
		t.Parallel()

		project, err := compose.LoadProjectFromContent(ctx, `
services:
  redis:
    image: redis:7-alpine
    volumes:
      - redis-data:/data
volumes:
  redis-data:
`)
		require.NoError(t, err)

		deployment, err := compose.NewDeploymentWithStrategy(ctx, cli, project,
			&deploy.RollingStrategy{ForceRecreate: true})
		require.NoError(t, err)

		plan, err := deployment.Plan(ctx)
		require.NoError(t, err)

		assert.Len(t, plan.Operations, 2, "Expected 1 volume creation and 1 service to deploy")
	})

	t.Run("global service auto-creates volumes on all machines", func(t *testing.T) {
		t.Parallel()

		serviceName := "test-compose-global-volume"
		volumeName := serviceName
		t.Cleanup(func() {
			removeServices(t, cli, serviceName)
			for _, machine := range c.Machines {
				_ = cli.RemoveVolume(ctx, machine.Name, volumeName, false)
			}
		})

		project, err := compose.LoadProject(ctx, []string{"fixtures/compose-global-volume.yaml"})
		require.NoError(t, err)

		deployment, err := compose.NewDeployment(ctx, cli, project)
		require.NoError(t, err)

		err = deployment.Run(ctx)
		require.NoError(t, err, "Global deployment should auto-create volumes on all machines")

		// Verify volumes were created on all machines.
		volumes, err := cli.ListVolumes(ctx, &api.VolumeFilter{Names: []string{volumeName}})
		require.NoError(t, err)
		assert.Len(t, volumes, len(c.Machines), "Volume should be created on all machines")

		// Verify containers are running on all machines.
		svc, err := cli.InspectService(ctx, serviceName)
		require.NoError(t, err)
		assert.Equal(t, api.ServiceModeGlobal, svc.Mode)
		assert.Len(t, svc.Containers, len(c.Machines), "Container should be running on all machines")

		machines := serviceMachines(svc)
		expectedMachines := make([]string, len(c.Machines))
		for i, m := range c.Machines {
			expectedMachines[i] = m.ID
		}
		assert.ElementsMatch(t, machines.ToSlice(), expectedMachines,
			"Containers should be distributed across all machines")
	})
}

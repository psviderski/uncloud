package e2e

import (
	"context"
	"testing"

	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/volume"
	"github.com/psviderski/uncloud/internal/ucind"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client/compose"
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

		deploy, err := compose.NewDeployment(ctx, cli, project)
		require.NoError(t, err)

		plan, err := deploy.Plan(ctx)
		require.NoError(t, err)
		assert.Len(t, plan.Operations, 1, "Expected 1 service to deploy")

		err = deploy.Run(ctx)
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

		deploy, err := compose.NewDeployment(ctx, cli, project)
		require.NoError(t, err)

		plan, err := deploy.Plan(ctx)
		require.ErrorContains(t, err, "external volumes not found: 'test-compose-volumes-external'")

		externalVolumeOpts := volume.CreateOptions{Name: "test-compose-volumes-external"}
		externalVolume, err := cli.CreateVolume(ctx, c.Machines[2].Name, externalVolumeOpts)
		require.NoError(t, err)

		// Recreate the deployment as it caches the cluster state.
		deploy, err = compose.NewDeployment(ctx, cli, project)
		require.NoError(t, err)

		plan, err = deploy.Plan(ctx)
		require.NoError(t, err)
		assert.Len(t, plan.Operations, 5, "Expected 2 volumes creation and 3 services to deploy")

		err = deploy.Run(ctx)
		require.NoError(t, err)

		// Verify data1 and data2 volumes have been created.
		volumes, err := cli.ListVolumes(ctx, &api.VolumeFilter{
			Names: []string{"test-compose-volumes-data1", "test-compose-volumes-data2"},
		})
		require.NoError(t, err)
		assert.Len(t, volumes, 2, "Expected 2 volumes to be created")

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
		deploy, err = compose.NewDeployment(ctx, cli, project)
		require.NoError(t, err)

		plan, err = deploy.Plan(ctx)
		require.NoError(t, err)
		assert.Len(t, plan.Operations, 0, "Expected no new operations after deployment")
	})
}

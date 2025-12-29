package e2e

import (
	"context"
	"os"
	"testing"

	"github.com/psviderski/uncloud/internal/ucind"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client/compose"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComposeConfigs(t *testing.T) {
	t.Parallel()

	clusterName := "ucind-test.compose-configs"
	ctx := context.Background()
	c, _ := createTestCluster(t, clusterName, ucind.CreateClusterOptions{Machines: 1}, true)

	machine := c.Machines[0]
	cli, err := machine.Connect(ctx)
	require.NoError(t, err)

	t.Run("basic configs", func(t *testing.T) {
		t.Parallel()

		name := "web"
		t.Cleanup(func() {
			removeServices(t, cli, name)
		})

		project, err := compose.LoadProject(ctx, []string{"fixtures/compose-configs.yaml"})
		require.NoError(t, err)

		deploy, err := compose.NewDeployment(ctx, cli, project)
		require.NoError(t, err)

		plan, err := deploy.Plan(ctx)
		require.NoError(t, err)
		assert.Len(t, plan.Operations, 1, "Expected 1 service deployment")

		err = deploy.Run(ctx)
		require.NoError(t, err)

		svc, err := cli.InspectService(ctx, name)
		require.NoError(t, err)

		expectedSpec := api.ServiceSpec{
			Name: name,
			Mode: api.ServiceModeReplicated,
			Container: api.ContainerSpec{
				Command: []string{"sleep", "600"},
				Image:   "busybox:1.37.0-uclibc",
				ConfigMounts: []api.ConfigMount{
					{
						ConfigName:    "from-file",
						ContainerPath: "/etc/config-from-file.conf",
						Mode:          func() *os.FileMode { m := os.FileMode(0o644); return &m }(),
					},
					{
						ConfigName:    "from-inline",
						ContainerPath: "/etc/config-inline.conf",
						Uid:           "1000",
						Gid:           "1000",
						Mode:          func() *os.FileMode { m := os.FileMode(0o600); return &m }(),
					},
					{
						ConfigName:    "from-file",
						ContainerPath: "/etc/new-dir/config-from-file.conf",
						Mode:          func() *os.FileMode { m := os.FileMode(0o644); return &m }(),
					},
				},
			},
			Configs: []api.ConfigSpec{
				{
					Name:    "from-file",
					Content: []byte("this is file config\n"),
				},
				{
					Name:    "from-inline",
					Content: []byte("this is inline config\n"),
				},
			},
			Replicas: 1,
		}
		assertServiceMatchesSpec(t, svc, expectedSpec)

		// Verify deployment is up-to-date after initial deployment
		deploy, err = compose.NewDeployment(ctx, cli, project)
		require.NoError(t, err)

		plan, err = deploy.Plan(ctx)
		require.NoError(t, err)
		assert.Len(t, plan.Operations, 0, "Expected no new operations after configs deployment")

		// Verify the config files are actually created in the container and contain expected content
		containerName := svc.Containers[0].Container.Name

		configContentFirst, err := readFileInfoInContainer(t, cli, name, containerName, "/etc/config-from-file.conf")
		require.NoError(t, err)
		assert.Equal(t, fileInfo{
			permissions: 0o644,
			content:     "this is file config\n",
			userId:      0,
			groupId:     0,
		}, configContentFirst)

		configContentSecond, err := readFileInfoInContainer(t, cli, name, containerName, "/etc/config-inline.conf")
		require.NoError(t, err)
		assert.Equal(t, fileInfo{
			permissions: 0o600,
			content:     "this is inline config\n",
			userId:      1000,
			groupId:     1000,
		}, configContentSecond)

		configContentThird, err := readFileInfoInContainer(t, cli, name, containerName, "/etc/new-dir/config-from-file.conf")
		require.NoError(t, err)
		assert.Equal(t, fileInfo{
			permissions: 0o644,
			content:     "this is file config\n",
			userId:      0,
			groupId:     0,
		}, configContentThird, "Same config should be mountable to multiple paths, including nested directories")
	})
}

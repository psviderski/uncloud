package e2e

import (
	"context"
	"path/filepath"
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

	cli, err := c.Machines[0].Connect(ctx)
	require.NoError(t, err)

	// Get absolute path to the current directory to be able to reference compose files relatively.
	currentDir, err := filepath.Abs(".")
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
		// Should plan to deploy 1 service (configs are parsed but not yet deployed by uncloud)
		assert.Len(t, plan.Operations, 1, "Expected 1 service deployment")

		err = deploy.Run(ctx)
		require.NoError(t, err)

		svc, err := cli.InspectService(ctx, name)
		require.NoError(t, err)

		expectedSpec := api.ServiceSpec{
			Name: name,
			Mode: api.ServiceModeReplicated,
			Container: api.ContainerSpec{
				Env: map[string]string{
					"APP_ENV": "production",
				},
				Image: "portainer/pause:3.9",
				ConfigMounts: []api.ConfigMount{
					{
						Source: "from-file",
						Target: "/etc/config-from-file.conf",
						Mode:   func() *uint32 { m := uint32(0o644); return &m }(),
					},
					{
						Source: "from-inline",
						Target: "/etc/config-inline.conf",
						UID:    "1000",
						GID:    "1000",
						Mode:   func() *uint32 { m := uint32(0o600); return &m }(),
					},
				},
			},
			Configs: []api.ConfigSpec{
				{
					Name:    "from-file",
					File:    filepath.Join(currentDir, "fixtures", "configs", "test-config.conf"),
					Content: "this is file config\n",
				},
				{
					Name:    "from-inline",
					Content: "this is inline config\n",
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
	})
}

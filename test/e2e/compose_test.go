package e2e

import (
	"context"
	"errors"
	"testing"

	"github.com/psviderski/uncloud/internal/ucind"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client/compose"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComposeDeployment(t *testing.T) {
	t.Parallel()

	clusterName := "ucind-test.compose"
	ctx := context.Background()
	c, _ := createTestCluster(t, clusterName, ucind.CreateClusterOptions{Machines: 3}, true)

	cli, err := c.Machines[0].Connect(ctx)
	require.NoError(t, err)

	t.Run("basic", func(t *testing.T) {
		t.Parallel()

		name := "basic"
		t.Cleanup(func() {
			err := cli.RemoveService(ctx, name)
			if !errors.Is(err, api.ErrNotFound) {
				require.NoError(t, err)
			}
		})

		project, err := compose.LoadProject(ctx, []string{"fixtures/basic-compose.yaml"})
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
				// TODO: resolve image digest and substitute the image with the image@digest.
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
}

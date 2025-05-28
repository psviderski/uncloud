package e2e

import (
	"context"
	"testing"

	"github.com/psviderski/uncloud/internal/ucind"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client/compose"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComposeBuild(t *testing.T) {
	t.Parallel()

	clusterName := "ucind-test.compose"
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
	})
}

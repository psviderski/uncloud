package e2e

import (
	"context"
	dockerclient "github.com/docker/docker/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"uncloud/internal/api"
	"uncloud/internal/cli/client"
	"uncloud/internal/ucind"
)

func TestRunService(t *testing.T) {
	t.Parallel()

	clusterName := "ucind-test.run-service"
	ctx := context.Background()
	c, _ := createTestCluster(t, clusterName, ucind.CreateClusterOptions{Machines: 3}, true)

	cli, err := c.Machines[0].Connect(ctx)
	require.NoError(t, err)

	t.Run("1 replica", func(t *testing.T) {
		t.Parallel()

		name := "busybox-1-replica"
		t.Cleanup(func() {
			err := cli.RemoveService(ctx, name)
			if !dockerclient.IsErrNotFound(err) {
				require.NoError(t, err)
			}

			_, err = cli.InspectService(ctx, name)
			require.ErrorIs(t, err, client.ErrNotFound)
		})

		resp, err := cli.RunService(ctx, api.ServiceSpec{
			Name: name,
			Mode: api.ServiceModeReplicated,
			Container: api.ContainerSpec{
				Command: []string{"sleep", "infinity"},
				Image:   "busybox:latest",
			},
		})
		require.NoError(t, err)

		assert.NotEmpty(t, resp.ID)
		assert.Equal(t, name, resp.Name)
		assert.Len(t, resp.Containers, 1)

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

	t.Run("global mode", func(t *testing.T) {
		t.Parallel()

		name := "busybox-global"
		t.Cleanup(func() {
			err := cli.RemoveService(ctx, name)
			if !dockerclient.IsErrNotFound(err) {
				require.NoError(t, err)
			}

			_, err = cli.InspectService(ctx, name)
			require.ErrorIs(t, err, client.ErrNotFound)
		})

		resp, err := cli.RunService(ctx, api.ServiceSpec{
			Name: name,
			Mode: api.ServiceModeGlobal,
			Container: api.ContainerSpec{
				Command: []string{"sleep", "infinity"},
				Image:   "busybox:latest",
			},
		})
		require.NoError(t, err)

		assert.NotEmpty(t, resp.ID)
		assert.Equal(t, name, resp.Name)
		assert.Len(t, resp.Containers, 3, "expected 1 container on each machine")

		svc, err := cli.InspectService(ctx, name)
		require.NoError(t, err)

		assert.Equal(t, resp.ID, svc.ID)
		assert.Equal(t, name, svc.Name)
		assert.Equal(t, api.ServiceModeGlobal, svc.Mode)
		assert.Len(t, svc.Containers, 3, "expected 1 container on each machine")
	})
}

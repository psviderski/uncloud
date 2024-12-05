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

	name := "ucind-test.run-service"
	ctx := context.Background()
	c, _ := createTestCluster(t, name, ucind.CreateClusterOptions{Machines: 3})

	cli, err := c.Machines[0].Connect(ctx)
	require.NoError(t, err)

	t.Run("global mode", func(t *testing.T) {
		t.Cleanup(func() {
			err := cli.RemoveService(ctx, "busybox-global")
			if !dockerclient.IsErrNotFound(err) {
				require.NoError(t, err)
			}

			_, err = cli.InspectService(ctx, "busybox-global")
			require.ErrorIs(t, err, client.ErrNotFound)
		})

		resp, err := cli.RunService(ctx, api.ServiceSpec{
			Name: "busybox-global",
			Mode: api.ServiceModeGlobal,
			Container: api.ContainerSpec{
				Command: []string{"sleep", "infinity"},
				Image:   "busybox:latest",
			},
		})
		require.NoError(t, err)

		assert.NotEmpty(t, resp.ID)
		assert.Equal(t, "busybox-global", resp.Name)
		assert.Len(t, resp.Containers, 3, "expected 1 container on each machine")

		svc, err := cli.InspectService(ctx, "busybox-global")
		require.NoError(t, err)

		assert.Equal(t, resp.ID, svc.ID)
		assert.Equal(t, "busybox-global", svc.Name)
		assert.Equal(t, api.ServiceModeGlobal, svc.Mode)
		assert.Len(t, svc.Containers, 3, "expected 1 container on each machine")
	})
}

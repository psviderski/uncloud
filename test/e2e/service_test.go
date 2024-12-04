package e2e

import (
	"context"
	dockerclient "github.com/docker/docker/client"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
	"uncloud/internal/api"
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

		require.NotEmpty(t, resp.ID)
		require.Equal(t, "busybox-global", resp.Name)
		require.Len(t, resp.Containers, 3, "expected 1 container on each machine")

		svc, err := cli.InspectService(ctx, "busybox-global")
		require.NoError(t, err)

		require.Equal(t, resp.ID, svc.ID)
		require.Equal(t, "busybox-global", svc.Name)
		require.Equal(t, api.ServiceModeGlobal, svc.Mode)

		require.Eventually(t, func() bool {
			svc, err = cli.InspectService(ctx, "busybox-global")
			require.NoError(t, err)
			if len(svc.Containers) != 3 {
				return false
			}
			return true
			//require.Len(t, svc.Containers, 3, "expected 1 container on each machine")
		}, 10*time.Second, 10*time.Millisecond)
	})
}

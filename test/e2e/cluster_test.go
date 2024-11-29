package e2e

import (
	"context"
	"github.com/docker/docker/client"
	"github.com/stretchr/testify/require"
	"testing"
	"uncloud/internal/ucind"
)

func createTestCluster(t *testing.T, name string, opts ucind.CreateClusterOptions) (*ucind.Provisioner, ucind.Cluster) {
	dockerCli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	require.NoError(t, err)

	p := ucind.NewProvisioner(dockerCli, nil)

	ctx := context.Background()
	c, err := p.CreateCluster(ctx, name, opts)
	require.NoError(t, err)

	t.Cleanup(func() {
		err = p.RemoveCluster(ctx, name)
		require.NoError(t, err)
	})

	return p, c
}

func TestClusterLifecycle(t *testing.T) {
	t.Parallel()

	var (
		name = "test-cluster-lifecycle"
		ctx  = context.Background()
		p    *ucind.Provisioner
		c    ucind.Cluster
	)

	t.Run("create", func(t *testing.T) {
		p, c = createTestCluster(t, name, ucind.CreateClusterOptions{Machines: 3})

		require.Equal(t, name, c.Name)
		require.Len(t, c.Machines, 3)
	})

	t.Run("remove", func(t *testing.T) {
		err := p.RemoveCluster(ctx, name)
		require.NoError(t, err)

		_, err = p.InspectCluster(ctx, name)
		require.ErrorIs(t, err, ucind.ErrNotFound)
	})
}

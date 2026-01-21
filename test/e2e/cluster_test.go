package e2e

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	dockerclient "github.com/docker/docker/client"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/internal/ucind"
	"github.com/psviderski/uncloud/pkg/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func createTestCluster(
	t *testing.T, name string, opts ucind.CreateClusterOptions, waitReady bool,
) (ucind.Cluster, *ucind.Provisioner) {
	dockerCli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	require.NoError(t, err)

	p := ucind.NewProvisioner(dockerCli, nil)
	ctx := context.Background()

	// Use the existing cluster if specified by the environment variable.
	envName := os.Getenv("TEST_CLUSTER_NAME")
	if envName != "" {
		c, err := p.InspectCluster(ctx, envName)
		if err == nil {
			return c, p
		}
		if !errors.Is(err, ucind.ErrNotFound) {
			require.NoError(t, err)
		}
	}

	// Remove the cluster if it already exists. It could be left from a previous interrupted test run.
	require.NoError(t, p.RemoveCluster(ctx, name))

	c, err := p.CreateCluster(ctx, name, opts)
	require.NoError(t, err)
	assert.Equal(t, name, c.Name)
	assert.Len(t, c.Machines, opts.Machines)
	for _, m := range c.Machines {
		assert.NotEmpty(t, m.ID)
	}

	t.Cleanup(func() {
		require.NoError(t, p.RemoveCluster(ctx, name))
	})

	if waitReady {
		require.NoError(t, p.WaitClusterReady(ctx, c, 30*time.Second))
	}

	return c, p
}

func TestClusterLifecycle(t *testing.T) {
	t.Parallel()

	name := "ucind-test.cluster-lifecycle"
	ctx := context.Background()
	c, p := createTestCluster(t, name, ucind.CreateClusterOptions{Machines: 3}, false)

	t.Run("each machine reconciled cluster store", func(t *testing.T) {
		var err error
		// Create a client for each machine and wait for it to be ready.
		clients := make([]*client.Client, len(c.Machines))
		for i, m := range c.Machines {
			clients[i], err = m.Connect(ctx)
			require.NoError(t, err)
			//goland:noinspection GoDeferInLoop
			defer clients[i].Close()
		}

		// Any machine should work as a cluster API endpoint, e.g. be able to list all machines in the cluster.
		for i, cli := range clients {
			// Wait for the machine to reconcile the cluster store.
			require.Eventually(t, func() bool {
				machines, err := cli.ListMachines(ctx, nil)
				if err != nil {
					// Unavailable "machine is not ready to serve cluster requests" is expected until
					// the store is reconciled.
					if s, ok := status.FromError(err); ok && s.Code() == codes.Unavailable {
						return false
					}
					require.NoError(t, err)
				}

				if len(machines) != 3 {
					return false
				}

				for _, m := range machines {
					if pb.MachineMember_UP != m.State {
						return false
					}
				}

				return true
			}, 15*time.Second, 50*time.Millisecond, "cluster store not reconciled on machine #%d", i+1)
		}
	})

	t.Run("inspect", func(t *testing.T) {
		cluster, err := p.InspectCluster(ctx, name)
		require.NoError(t, err)

		assert.Equal(t, name, cluster.Name)
		assert.Len(t, cluster.Machines, 3)
		for _, m := range cluster.Machines {
			assert.NotEmpty(t, m.ID)
			assert.True(t, strings.HasPrefix(m.Name, "machine-"))
		}
	})

	t.Run("remove", func(t *testing.T) {
		err := p.RemoveCluster(ctx, name)
		require.NoError(t, err)

		_, err = p.InspectCluster(ctx, name)
		require.ErrorIs(t, err, ucind.ErrNotFound)
	})
}

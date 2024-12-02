package e2e

import (
	"context"
	dockerclient "github.com/docker/docker/client"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"testing"
	"time"
	"uncloud/internal/cli/client"
	"uncloud/internal/cli/client/connector"
	"uncloud/internal/machine/api/pb"
	"uncloud/internal/ucind"
)

func createTestCluster(t *testing.T, name string, opts ucind.CreateClusterOptions) (*ucind.Provisioner, ucind.Cluster) {
	dockerCli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	require.NoError(t, err)

	p := ucind.NewProvisioner(dockerCli, nil)
	ctx := context.Background()
	// Remove the cluster if it already exists. It could be left from a previous interrupted test run.
	require.NoError(t, p.RemoveCluster(ctx, name))

	c, err := p.CreateCluster(ctx, name, opts)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, p.RemoveCluster(ctx, name))
	})

	return p, c
}

func TestClusterLifecycle(t *testing.T) {
	t.Parallel()

	var (
		name = "test-cluster-lifecycle"
		ctx  = context.Background()
	)

	p, c := createTestCluster(t, name, ucind.CreateClusterOptions{Machines: 3})
	require.Equal(t, name, c.Name)
	require.Len(t, c.Machines, 3)

	t.Run("each machine reconciled cluster store", func(t *testing.T) {
		var err error
		// Create a client for each machine and wait for it to be ready.
		clients := make([]*client.Client, len(c.Machines))
		for i, m := range c.Machines {
			require.NoError(t, p.WaitMachineReady(ctx, m))
			clients[i], err = client.New(ctx, connector.NewTCPConnector(m.APIAddress))
			require.NoError(t, err)
			//goland:noinspection GoDeferInLoop
			defer clients[i].Close()
		}

		// Any machine should work as a cluster API endpoint, e.g. be able to list all machines in the cluster.
		for _, cli := range clients {
			// Wait for the machine to reconcile the cluster store.
			require.Eventually(t, func() bool {
				machines, err := cli.ListMachines(ctx, &emptypb.Empty{})
				if err != nil {
					// FailedPrecondition "cluster is not initialised" is expected until the store is reconciled.
					if s, ok := status.FromError(err); ok {
						if s.Code() == codes.FailedPrecondition {
							return false
						}
					}
					require.NoError(t, err)
				}

				if len(machines.Machines) != 3 {
					return false
				}

				for _, m := range machines.Machines {
					if pb.MachineMember_UP != m.State {
						return false
					}
				}

				return true
			}, 15*time.Second, 50*time.Millisecond)
		}
	})

	t.Run("remove", func(t *testing.T) {
		err := p.RemoveCluster(ctx, name)
		require.NoError(t, err)

		_, err = p.InspectCluster(ctx, name)
		require.ErrorIs(t, err, ucind.ErrNotFound)
	})
}

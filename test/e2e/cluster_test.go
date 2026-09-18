package e2e

import (
	"context"
	"errors"
	"maps"
	"os"
	"strings"
	"testing"
	"time"

	dockerclient "github.com/docker/docker/client"
	"github.com/google/uuid"
	"github.com/psviderski/uncloud/api/pb"
	"github.com/psviderski/uncloud/internal/ucind"
	"github.com/psviderski/uncloud/pkg/client"
	"github.com/psviderski/uncloud/pkg/distlock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
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
		require.NoError(t, p.WaitClusterReady(ctx, c, 90*time.Second))
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
			}, 30*time.Second, 50*time.Millisecond, "cluster store not reconciled on machine #%d", i+1)
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

	t.Run("distributed lock", func(t *testing.T) {
		cli0, err := c.Machines[0].Connect(ctx)
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, cli0.Close())
		})

		cli1, err := c.Machines[1].Connect(ctx)
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, cli1.Close())
		})

		firstLocker, err := cli0.NewLocker(distlock.Config{})
		require.NoError(t, err)
		secondLocker, err := cli1.NewLocker(distlock.Config{})
		require.NoError(t, err)

		acquire := func(locker *distlock.Locker) (*distlock.Lease, error) {
			acquireCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()
			return locker.Acquire(acquireCtx, "e2e-lock")
		}
		release := func(lease *distlock.Lease) {
			t.Helper()
			if lease.Context().Err() != nil {
				return
			}
			releaseCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()
			require.NoError(t, lease.Release(releaseCtx))
		}

		firstLease, err := acquire(firstLocker)
		require.NoError(t, err)
		t.Cleanup(func() {
			release(firstLease)
		})

		contendingCtx, cancelContending := context.WithTimeout(ctx, 500*time.Millisecond)
		contendingLease, err := secondLocker.Acquire(contendingCtx, "e2e-lock")
		cancelContending()
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.Nil(t, contendingLease)

		release(firstLease)

		secondLease, err := acquire(secondLocker)
		require.NoError(t, err)
		t.Cleanup(func() {
			release(secondLease)
		})
	})

	t.Run("Caddy storage replication", func(t *testing.T) {
		clients := make([]*client.Client, len(c.Machines))
		for i, m := range c.Machines {
			cli, err := m.Connect(ctx)
			require.NoError(t, err)
			t.Cleanup(func() {
				require.NoError(t, cli.Close())
			})
			clients[i] = cli
		}

		storeVersion := func(clis ...*client.Client) map[string]uint64 {
			t.Helper()
			callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			version := make(map[string]uint64)
			for _, cli := range clis {
				resp, err := cli.MachineClient.InspectMachine(callCtx, &emptypb.Empty{})
				require.NoError(t, err)
				require.Len(t, resp.Machines, 1)
				m := resp.Machines[0]
				require.Len(t, m.StoreVersion, 3)
				for actor, v := range m.StoreVersion {
					version[actor] = max(version[actor], v)
				}
			}
			return version
		}
		waitForStoreVersion := func(cli *client.Client, version map[string]uint64) {
			t.Helper()
			waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			err := cli.WaitForStoreVersion(waitCtx, version)
			require.NoError(t, err)
		}

		prefix := "e2e/caddy-storage/" + uuid.NewString()
		key := prefix + "/key/path"
		otherKey := prefix + "/other-key"

		// Keep reused test clusters clean if an assertion stops the test before its explicit deletes.
		t.Cleanup(func() {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			_, _ = clients[0].CaddyStorage.Delete(cleanupCtx, &pb.DeleteCaddyStorageRequest{Key: prefix})
			_, _ = clients[1].CaddyStorage.Delete(cleanupCtx, &pb.DeleteCaddyStorageRequest{Key: prefix})
		})

		// Create and overwrite a key on the first machine before waiting for replication.
		var updatedAt time.Time
		for _, value := range [][]byte{[]byte("test-value"), []byte("replacement-value")} {
			_, err := clients[0].CaddyStorage.Store(ctx, &pb.StoreCaddyStorageRequest{Key: key, Value: value})
			require.NoError(t, err)

			// A Load through another machine must find the value on the machine that accepted the local write,
			// regardless of whether Corrosion has replicated it to the other machines yet.
			loadResp, err := clients[1].CaddyStorage.Load(client.ProxySingleMachineContext(ctx, c.Machines[0].ID),
				&pb.LoadCaddyStorageRequest{Key: key})
			require.NoError(t, err)
			require.Equal(t, value, loadResp.Value)
			require.NoError(t, loadResp.UpdatedAt.CheckValid())
			modified := loadResp.UpdatedAt.AsTime()
			require.False(t, modified.IsZero(), "Stored value should have a valid updated_at timestamp")
			if !updatedAt.IsZero() {
				// Back-to-back writes can have the same timestamp.
				require.False(t, modified.Before(updatedAt), "Overwriting a value should not move updated_at backwards")
			}
			updatedAt = modified
		}

		// Write a distinct key on the second machine, then capture the combined store version from both machines.
		otherValue := []byte("second-value")
		_, err := clients[1].CaddyStorage.Store(ctx, &pb.StoreCaddyStorageRequest{Key: otherKey, Value: otherValue})
		require.NoError(t, err)
		version := storeVersion(clients[0], clients[1])
		values := map[string][]byte{key: []byte("replacement-value"), otherKey: otherValue}

		// On each machine, wait for the store version to be reached, then verify that the final values are readable
		// and that the Stat and List endpoints return the expected results.
		for _, cli := range clients {
			waitForStoreVersion(cli, version)

			// Both final values must be readable locally as soon as the wait returns.
			for k, v := range values {
				resp, err := cli.CaddyStorage.Load(ctx, &pb.LoadCaddyStorageRequest{Key: k})
				require.NoError(t, err)
				assert.Equal(t, v, resp.Value)
				require.NoError(t, resp.UpdatedAt.CheckValid())
				if k == key {
					assert.True(t, resp.UpdatedAt.AsTime().Equal(updatedAt))
				}

				statResp, err := cli.CaddyStorage.Stat(ctx, &pb.StatCaddyStorageRequest{Key: k})
				require.NoError(t, err)
				assert.Equal(t, k, statResp.Key)
				assert.True(t, statResp.UpdatedAt.AsTime().Equal(resp.UpdatedAt.AsTime()))
				assert.EqualValues(t, len(v), statResp.Size)
				assert.True(t, statResp.IsTerminal)
			}

			// A path with descendants should exist as a directory even though no value is stored at that key.
			statResp, err := cli.CaddyStorage.Stat(ctx, &pb.StatCaddyStorageRequest{Key: prefix + "/key"})
			require.NoError(t, err)
			assert.Equal(t, prefix+"/key", statResp.Key)
			assert.Nil(t, statResp.UpdatedAt)
			assert.EqualValues(t, 0, statResp.Size)
			assert.False(t, statResp.IsTerminal)

			listResp, err := cli.CaddyStorage.List(ctx, &pb.ListCaddyStorageRequest{Prefix: prefix, Recursive: true})
			require.NoError(t, err)
			assert.Equal(t, []string{prefix + "/key", key, otherKey}, listResp.Keys)

			// A non-recursive list should only return the immediate child keys.
			listResp, err = cli.CaddyStorage.List(ctx, &pb.ListCaddyStorageRequest{Prefix: prefix, Recursive: false})
			require.NoError(t, err)
			assert.Equal(t, []string{prefix + "/key", otherKey}, listResp.Keys)
		}

		// Delete through one machine. The deletion must reach the other replicas through Corrosion.
		_, err = clients[2].CaddyStorage.Delete(ctx, &pb.DeleteCaddyStorageRequest{Key: prefix})
		require.NoError(t, err)
		version = storeVersion(clients[2])

		for _, cli := range clients {
			waitForStoreVersion(cli, version)

			for k := range values {
				_, err = cli.CaddyStorage.Load(ctx, &pb.LoadCaddyStorageRequest{Key: k})
				assert.Equal(t, codes.NotFound, status.Code(err))
				_, err = cli.CaddyStorage.Stat(ctx, &pb.StatCaddyStorageRequest{Key: k})
				assert.Equal(t, codes.NotFound, status.Code(err))
			}
			_, err = cli.CaddyStorage.List(ctx, &pb.ListCaddyStorageRequest{Prefix: prefix, Recursive: true})
			assert.Equal(t, codes.NotFound, status.Code(err))

			// Delete is idempotent on each machine.
			_, err = cli.CaddyStorage.Delete(ctx, &pb.DeleteCaddyStorageRequest{Key: prefix})
			require.NoError(t, err)
		}
	})

	t.Run("wait for store version edge cases", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()

		cli, err := c.Machines[0].Connect(ctx)
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, cli.Close())
		})

		resp, err := cli.MachineClient.InspectMachine(ctx, &emptypb.Empty{})
		require.NoError(t, err)
		version := resp.Machines[0].StoreVersion
		require.Len(t, version, 3)

		t.Run("already satisfied vector", func(t *testing.T) {
			err := cli.WaitForStoreVersion(ctx, version)
			require.NoError(t, err)
		})

		t.Run("empty vector", func(t *testing.T) {
			err := cli.WaitForStoreVersion(ctx, nil)
			require.NoError(t, err)
		})

		t.Run("zero version for unknown actor", func(t *testing.T) {
			err := cli.WaitForStoreVersion(ctx, map[string]uint64{uuid.NewString(): 0})
			require.NoError(t, err)
		})

		t.Run("invalid actor UUID", func(t *testing.T) {
			err := cli.WaitForStoreVersion(ctx, map[string]uint64{"not-a-uuid": 1})
			require.Equal(t, codes.InvalidArgument, status.Code(err))
		})

		t.Run("unknown actor times out", func(t *testing.T) {
			waitCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
			defer cancel()

			// Background writes cannot satisfy a target for an actor that does not exist.
			err := cli.WaitForStoreVersion(waitCtx, map[string]uint64{uuid.NewString(): 1})
			require.Equal(t, codes.DeadlineExceeded, status.Code(err))
		})

		t.Run("partially satisfied vector times out", func(t *testing.T) {
			waitCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
			defer cancel()

			minVersion := maps.Clone(version)
			minVersion[uuid.NewString()] = 1
			err := cli.WaitForStoreVersion(waitCtx, minVersion)
			require.Equal(t, codes.DeadlineExceeded, status.Code(err))
		})

		t.Run("cancel pending wait", func(t *testing.T) {
			waitCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			timer := time.AfterFunc(500*time.Millisecond, cancel)
			defer timer.Stop()

			err := cli.WaitForStoreVersion(waitCtx, map[string]uint64{uuid.NewString(): 1})
			require.Equal(t, codes.Canceled, status.Code(err))
		})
	})

	t.Run("remove", func(t *testing.T) {
		err := p.RemoveCluster(ctx, name)
		require.NoError(t, err)

		_, err = p.InspectCluster(ctx, name)
		require.ErrorIs(t, err, ucind.ErrNotFound)
	})
}

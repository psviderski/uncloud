package grpc_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/psviderski/uncloud/pkg/distlock"
	distlockgrpc "github.com/psviderski/uncloud/pkg/distlock/grpc"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/durationpb"
)

func newTestLeaseClient(t *testing.T) distlockgrpc.LeaseClient {
	t.Helper()

	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	distlockgrpc.RegisterLeaseServer(server, distlockgrpc.NewServer(distlock.NewMemoryStore()))
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(listener)
	}()

	conn, err := grpc.NewClient(
		"passthrough:///distlock",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, conn.Close())
		server.Stop()
		if err := <-serveErr; err != nil {
			require.ErrorIs(t, err, grpc.ErrServerStopped)
		}
	})

	return distlockgrpc.NewLeaseClient(conn)
}

func TestLeaseClientServerLifecycle(t *testing.T) {
	client := newTestLeaseClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ttl := durationpb.New(time.Minute)
	ownerToken := []byte("owner")
	otherToken := []byte("other")

	acquired, err := client.Acquire(ctx, &distlockgrpc.AcquireLeaseRequest{
		Resource: "resource",
		Token:    ownerToken,
		Ttl:      ttl,
	})
	require.NoError(t, err)
	require.True(t, acquired.Acquired)

	acquired, err = client.Acquire(ctx, &distlockgrpc.AcquireLeaseRequest{
		Resource: "resource",
		Token:    otherToken,
		Ttl:      ttl,
	})
	require.NoError(t, err)
	require.False(t, acquired.Acquired)

	renewed, err := client.Renew(ctx, &distlockgrpc.RenewLeaseRequest{
		Resource: "resource",
		Token:    otherToken,
		Ttl:      ttl,
	})
	require.NoError(t, err)
	require.False(t, renewed.Renewed)

	renewed, err = client.Renew(ctx, &distlockgrpc.RenewLeaseRequest{
		Resource: "resource",
		Token:    ownerToken,
		Ttl:      ttl,
	})
	require.NoError(t, err)
	require.True(t, renewed.Renewed)

	released, err := client.Release(ctx, &distlockgrpc.ReleaseLeaseRequest{
		Resource: "resource",
		Token:    otherToken,
	})
	require.NoError(t, err)
	require.False(t, released.Released)

	released, err = client.Release(ctx, &distlockgrpc.ReleaseLeaseRequest{
		Resource: "resource",
		Token:    ownerToken,
	})
	require.NoError(t, err)
	require.True(t, released.Released)

	acquired, err = client.Acquire(ctx, &distlockgrpc.AcquireLeaseRequest{
		Resource: "resource",
		Token:    otherToken,
		Ttl:      ttl,
	})
	require.NoError(t, err)
	require.True(t, acquired.Acquired)
}

func TestLeaseClientServerRejectsInvalidRequests(t *testing.T) {
	client := newTestLeaseClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tests := []struct {
		name string
		call func(context.Context) error
	}{
		{
			name: "acquire without resource",
			call: func(ctx context.Context) error {
				_, err := client.Acquire(ctx, &distlockgrpc.AcquireLeaseRequest{
					Token: []byte("owner"),
					Ttl:   durationpb.New(time.Minute),
				})
				return err
			},
		},
		{
			name: "acquire without TTL",
			call: func(ctx context.Context) error {
				_, err := client.Acquire(ctx, &distlockgrpc.AcquireLeaseRequest{
					Resource: "resource",
					Token:    []byte("owner"),
				})
				return err
			},
		},
		{
			name: "renew with non-positive TTL",
			call: func(ctx context.Context) error {
				_, err := client.Renew(ctx, &distlockgrpc.RenewLeaseRequest{
					Resource: "resource",
					Token:    []byte("owner"),
					Ttl:      durationpb.New(0),
				})
				return err
			},
		},
		{
			name: "release without token",
			call: func(ctx context.Context) error {
				_, err := client.Release(ctx, &distlockgrpc.ReleaseLeaseRequest{Resource: "resource"})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call(ctx)
			require.Error(t, err)
			require.Equal(t, codes.InvalidArgument, status.Code(err))
		})
	}
}

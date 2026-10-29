package connector

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

func TestUnixConnectorReconnectsAfterSocketReplacement(t *testing.T) {
	// Use a custom pattern instead of t.TempDir() to create a short path for the socket on macOS
	// because it has a 104 character limit.
	t.TempDir()
	dir, err := os.MkdirTemp("", "uncloud-")
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, os.RemoveAll(dir))
	})
	socketPath := filepath.Join(dir, "uncloud.sock")
	conn, err := NewUnixConnector(socketPath).Connect(context.Background())
	require.NoError(t, err, "creating a gRPC channel must not require the socket to exist")
	t.Cleanup(func() {
		require.NoError(t, conn.Close())
	})

	client := healthpb.NewHealthClient(conn)
	first := startHealthServer(t, socketPath)
	requireHealthCheck(t, client)

	first.Stop()

	second := startHealthServer(t, socketPath)
	t.Cleanup(second.Stop)
	requireHealthCheck(t, client)
}

func startHealthServer(t *testing.T, socketPath string) *grpc.Server {
	t.Helper()

	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err)

	healthServer := health.NewServer()
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	server := grpc.NewServer()
	healthpb.RegisterHealthServer(server, healthServer)

	go func() {
		_ = server.Serve(listener)
	}()

	return server
}

func requireHealthCheck(t *testing.T, client healthpb.HealthClient) {
	t.Helper()

	require.Eventually(t, func() bool {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		resp, err := client.Check(ctx, &healthpb.HealthCheckRequest{})
		return err == nil && resp.Status == healthpb.HealthCheckResponse_SERVING
	}, 15*time.Second, 100*time.Millisecond)
}

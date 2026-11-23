package hub

import (
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/psviderski/uncloud/api/pb"
	"github.com/psviderski/uncloud/hub/tunnel/tunneltest"
	apiproxy "github.com/psviderski/uncloud/internal/machine/api/proxy"
	"github.com/psviderski/uncloud/internal/machine/network"
	"github.com/psviderski/uncloud/internal/secret"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	clusterID = "cluster-1"
	machineID = "machine-1"
)

func TestParseEndpoint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		endpoint   string
		wantTarget string
		wantTLS    bool
		wantErr    bool
	}{
		{name: "https default port", endpoint: "https://hub.uncloud.run", wantTarget: "hub.uncloud.run:443", wantTLS: true},
		{name: "http default port", endpoint: "http://localhost", wantTarget: "localhost:80"},
		{name: "explicit port", endpoint: "http://127.0.0.1:8081", wantTarget: "127.0.0.1:8081"},
		{name: "uppercase scheme", endpoint: "HTTPS://hub.uncloud.run", wantTarget: "hub.uncloud.run:443", wantTLS: true},
		{name: "trailing slash", endpoint: "https://hub.uncloud.run/", wantTarget: "hub.uncloud.run:443", wantTLS: true},
		{name: "IPv6", endpoint: "https://[::1]:8443", wantTarget: "[::1]:8443", wantTLS: true},
		{name: "unsupported scheme", endpoint: "grpc://hub.uncloud.run", wantErr: true},
		{name: "no scheme", endpoint: "hub.uncloud.run", wantErr: true},
		{name: "empty host", endpoint: "https://", wantErr: true},
		{name: "path", endpoint: "https://hub.uncloud.run/api", wantErr: true},
		{name: "query", endpoint: "https://hub.uncloud.run?a=b", wantErr: true},
		{name: "user info", endpoint: "https://user@hub.uncloud.run", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, useTLS, err := parseEndpoint(tt.endpoint)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantTarget, target)
			assert.Equal(t, tt.wantTLS, useTLS)
		})
	}
}

func TestNewConnector_InvalidConfig(t *testing.T) {
	t.Parallel()

	priv, _, err := network.NewMachineKeys()
	require.NoError(t, err)
	valid := Config{Endpoint: "https://hub.uncloud.run", ClusterID: clusterID, MachineID: machineID, PrivateKey: priv}

	tests := []struct {
		name   string
		modify func(c *Config)
	}{
		{name: "invalid endpoint", modify: func(c *Config) { c.Endpoint = "hub.uncloud.run" }},
		{name: "empty cluster ID", modify: func(c *Config) { c.ClusterID = "" }},
		{name: "empty machine ID", modify: func(c *Config) { c.MachineID = "" }},
		{name: "invalid private key", modify: func(c *Config) { c.PrivateKey = secret.Secret("short") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := valid
			tt.modify(&c)
			_, err := NewConnector(c, grpc.NewServer())
			assert.Error(t, err)
		})
	}

	_, err = NewConnector(valid, grpc.NewServer())
	assert.NoError(t, err)
}

// TestConnector_HubReadOnlyAccess checks the whole path from Hub through a tunnel and the API server to the local
// machine API, and that the API server only allows read-only methods.
func TestStatus_Next(t *testing.T) {
	t.Parallel()

	t0 := time.Now()
	t1 := t0.Add(time.Second)
	t2 := t1.Add(time.Second)
	t3 := t2.Add(time.Second)
	t4 := t3.Add(time.Second)
	errClosed := assert.AnError

	s := Status{}.next(StateConnecting, t0, nil, time.Time{})
	assert.Equal(t, t0, s.Since)

	s = s.next(StateRetrying, t1, errClosed, t2)
	assert.Equal(t, t0, s.Since, "retrying must not reset the time since the machine started connecting")
	assert.Equal(t, t2, s.RetryAt)
	assert.Equal(t, t1, s.LastErrorAt)

	s = s.next(StateConnecting, t2, nil, time.Time{})
	assert.Equal(t, t0, s.Since)
	assert.True(t, s.RetryAt.IsZero())

	s = s.next(StateConnected, t3, nil, time.Time{})
	assert.Equal(t, t3, s.Since)
	assert.Equal(t, errClosed, s.LastError, "the last error must be kept after the tunnel opens")

	s = s.next(StateRetrying, t4, errClosed, t4)
	assert.Equal(t, t4, s.Since)
}

func TestConnector_HubReadOnlyAccess(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sockPath := serveLocalMachineAPI(t)
	director := apiproxy.NewDirector(sockPath, 0, nil)
	t.Cleanup(director.Close)
	apiServer := NewAPIServer(director.Director)
	t.Cleanup(apiServer.Stop)

	hub := tunneltest.NewHub(t)
	connector := newTestConnector(t, hub, true, apiServer)
	runConnector(t, ctx, connector)

	cc, err := hub.Client(ctx, machineID)
	require.NoError(t, err)

	resp, err := pb.NewClusterClient(cc).ListMachines(ctx, &emptypb.Empty{})
	require.NoError(t, err)
	require.Len(t, resp.Machines, 1)
	assert.Equal(t, "machine-name", resp.Machines[0].Machine.Name)

	_, err = pb.NewClusterClient(cc).RemoveMachine(ctx, &pb.RemoveMachineRequest{Id: "machine-2"})
	assert.Equal(t, codes.PermissionDenied, status.Code(err), err)

	_, err = pb.NewDockerClient(cc).CreateContainer(ctx, &pb.CreateContainerRequest{})
	assert.Equal(t, codes.PermissionDenied, status.Code(err), err)
}

func TestConnector_ReopensClosedTunnel(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	hub := tunneltest.NewHub(t)
	proxy := newCountingProxy(t, hub.Addr())
	connector := newTestConnector(t, hub, true, newClusterServer(t))
	connector.endpoint = "http://" + proxy.addr
	runConnector(t, ctx, connector)

	_, err := hub.Client(ctx, machineID)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		return connector.Status().State == StateConnected
	}, 5*time.Second, 10*time.Millisecond)
	opened := connector.Status().Since

	hub.Drop(machineID)

	require.Eventually(t, func() bool {
		s := connector.Status()
		return s.State == StateConnected && s.Since.After(opened) && s.LastError != nil
	}, 5*time.Second, 10*time.Millisecond, "connector didn't reopen the tunnel")
	// The connection of the closed tunnel could be dead, so the connector must not reuse it.
	assert.EqualValues(t, 2, proxy.conns.Load(), "connector didn't reopen the tunnel over a new connection")

	cc, err := hub.Client(ctx, machineID)
	require.NoError(t, err)
	_, err = pb.NewClusterClient(cc).ListMachines(ctx, &emptypb.Empty{})
	assert.NoError(t, err)
}

func TestConnector_Unauthenticated(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	hub := tunneltest.NewHub(t)
	connector := newTestConnector(t, hub, false, newClusterServer(t))
	runConnector(t, ctx, connector)

	require.Eventually(t, func() bool {
		return status.Code(connector.Status().LastError) == codes.Unauthenticated
	}, 5*time.Second, 10*time.Millisecond)
	s := connector.Status()
	assert.Contains(t, []State{StateConnecting, StateRetrying}, s.State)
	assert.False(t, s.LastErrorAt.IsZero())
	assert.False(t, hub.Connected(machineID))
}

func TestConnector_StopsWhenCancelled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hub := tunneltest.NewHub(t)
	connector := newTestConnector(t, hub, true, newClusterServer(t))

	runErr := make(chan error, 1)
	go func() { runErr <- connector.Run(ctx) }()
	require.Eventually(t, func() bool {
		return connector.Status().State == StateConnected
	}, 5*time.Second, 10*time.Millisecond)

	cancel()

	select {
	case err := <-runErr:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("connector didn't stop after the context was cancelled")
	}
	assert.Equal(t, StateInactive, connector.Status().State)
	assert.Eventually(t, func() bool { return !hub.Connected(machineID) }, 5*time.Second, 10*time.Millisecond)
}

// newTestConnector creates a connector to the hub with short retry intervals. It adds the machine to the hub if
// known is true.
func newTestConnector(t *testing.T, hub *tunneltest.Hub, known bool, server *grpc.Server) *Connector {
	t.Helper()

	priv, pub, err := network.NewMachineKeys()
	require.NoError(t, err)
	if known {
		hub.AddMachine(clusterID, machineID, pub)
	}

	connector, err := NewConnector(Config{
		Endpoint:   hub.Endpoint(),
		ClusterID:  clusterID,
		MachineID:  machineID,
		PrivateKey: priv,
	}, server)
	require.NoError(t, err)
	connector.minRetryInterval = 10 * time.Millisecond
	connector.maxRetryInterval = 50 * time.Millisecond

	return connector
}

// runConnector runs the connector until the test ends.
func runConnector(t *testing.T, ctx context.Context, connector *Connector) {
	t.Helper()

	ctx, cancel := context.WithCancel(ctx)
	runErr := make(chan error, 1)
	go func() { runErr <- connector.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		assert.NoError(t, <-runErr)
	})
}

// countingProxy forwards TCP connections to a target address and counts them.
type countingProxy struct {
	addr  string
	conns atomic.Int32
}

func newCountingProxy(t *testing.T, target string) *countingProxy {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { listener.Close() })

	p := &countingProxy{addr: listener.Addr().String()}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			p.conns.Add(1)

			go func() {
				defer conn.Close()
				upstream, err := net.Dial("tcp", target)
				if err != nil {
					return
				}
				defer upstream.Close()

				go func() {
					io.Copy(upstream, conn)
					upstream.Close()
				}()
				io.Copy(conn, upstream)
			}()
		}
	}()

	return p
}

func newClusterServer(t *testing.T) *grpc.Server {
	t.Helper()

	server := grpc.NewServer()
	pb.RegisterClusterServer(server, &fakeCluster{})
	t.Cleanup(server.Stop)
	return server
}

// serveLocalMachineAPI serves a fake machine API on a Unix socket and returns the socket path.
func serveLocalMachineAPI(t *testing.T) string {
	t.Helper()

	// Use a short directory because Unix socket paths are limited to about 100 characters.
	dir, err := os.MkdirTemp("", "uchub")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })

	sockPath := filepath.Join(dir, "machine.sock")
	listener, err := net.Listen("unix", sockPath)
	require.NoError(t, err)

	server := grpc.NewServer()
	pb.RegisterClusterServer(server, &fakeCluster{})
	go server.Serve(listener)
	t.Cleanup(server.Stop)

	return sockPath
}

type fakeCluster struct {
	pb.UnimplementedClusterServer
}

func (f *fakeCluster) ListMachines(context.Context, *emptypb.Empty) (*pb.ListMachinesResponse, error) {
	return &pb.ListMachinesResponse{Machines: []*pb.MachineMember{
		{Machine: &pb.MachineInfo{Id: machineID, Name: "machine-name"}},
	}}, nil
}

func (f *fakeCluster) RemoveMachine(context.Context, *pb.RemoveMachineRequest) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

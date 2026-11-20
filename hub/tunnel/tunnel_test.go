package tunnel_test

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/psviderski/uncloud/api/pb"
	"github.com/psviderski/uncloud/hub/tunnel"
	"github.com/psviderski/uncloud/hub/tunnel/tunneltest"
	"github.com/psviderski/uncloud/internal/machine/network"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	clusterID = "cluster-1"
	machineID = "machine-1"
)

func TestTunnel_HubCallsMachine(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	hub := tunneltest.NewHub(t)
	creds := addMachine(t, hub)

	conn, err := tunnel.Open(ctx, hubClient(t, hub.Addr()), creds)
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	// Serve enough machines for the response to span many data messages.
	cluster := &fakeCluster{}
	for i := range 5000 {
		cluster.machines = append(cluster.machines, &pb.MachineMember{
			Machine: &pb.MachineInfo{Id: fmt.Sprintf("machine-%04d", i), Name: fmt.Sprintf("name-%04d", i)},
		})
	}
	serveConn(t, conn, cluster)

	cc, err := hub.Client(ctx, machineID)
	require.NoError(t, err)
	resp, err := pb.NewClusterClient(cc).ListMachines(ctx, &emptypb.Empty{})
	require.NoError(t, err)

	require.Len(t, resp.Machines, 5000)
	assert.Equal(t, "name-0000", resp.Machines[0].Machine.Name)
	assert.Equal(t, "name-4999", resp.Machines[4999].Machine.Name)
}

func TestOpen_Rejected(t *testing.T) {
	t.Parallel()

	hub := tunneltest.NewHub(t)
	creds := addMachine(t, hub)
	otherPriv, _, err := network.NewMachineKeys()
	require.NoError(t, err)

	tests := []struct {
		name   string
		modify func(c *tunnel.Credentials)
	}{
		{name: "unknown machine", modify: func(c *tunnel.Credentials) { c.MachineID = "unknown" }},
		{name: "unknown cluster", modify: func(c *tunnel.Credentials) { c.ClusterID = "unknown" }},
		{name: "wrong key", modify: func(c *tunnel.Credentials) { c.PrivateKey = otherPriv }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			c := creds
			tt.modify(&c)
			_, err := tunnel.Open(ctx, hubClient(t, hub.Addr()), c)
			require.Error(t, err)
			assert.Equal(t, codes.Unauthenticated, status.Code(err), err)
		})
	}
}

func TestTunnel_HubDropEndsMachineConn(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	hub := tunneltest.NewHub(t)
	creds := addMachine(t, hub)

	conn, err := tunnel.Open(ctx, hubClient(t, hub.Addr()), creds)
	require.NoError(t, err)
	serveConn(t, conn, &fakeCluster{})
	_, err = hub.Client(ctx, machineID)
	require.NoError(t, err)

	hub.Drop(machineID)

	select {
	case <-conn.Done():
		assert.Error(t, conn.Err())
	case <-ctx.Done():
		t.Fatal("machine connection didn't end after Hub dropped the tunnel")
	}
}

func TestNewAPIConn_ConnectsBeforeFirstCall(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	hub := tunneltest.NewHub(t)
	creds := addMachine(t, hub)

	conn, err := tunnel.Open(ctx, hubClient(t, hub.Addr()), creds)
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })
	serveConn(t, conn, &fakeCluster{})

	cc, err := hub.Client(ctx, machineID)
	require.NoError(t, err)

	// The machine pings Hub only on an established connection, so it mustn't wait for the first call.
	assert.Eventually(t, func() bool {
		return cc.GetState() == connectivity.Ready
	}, 5*time.Second, 10*time.Millisecond)
}

func TestTunnel_MachineCloseEndsHubTunnel(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	hub := tunneltest.NewHub(t)
	creds := addMachine(t, hub)

	conn, err := tunnel.Open(ctx, hubClient(t, hub.Addr()), creds)
	require.NoError(t, err)
	serveConn(t, conn, &fakeCluster{})
	_, err = hub.Client(ctx, machineID)
	require.NoError(t, err)

	require.NoError(t, conn.Close())

	assert.Eventually(t, func() bool { return !hub.Connected(machineID) }, 5*time.Second, 10*time.Millisecond)
}

// addMachine generates a WireGuard key pair for a machine, adds it to the hub, and returns the machine
// credentials.
func addMachine(t *testing.T, hub *tunneltest.Hub) tunnel.Credentials {
	t.Helper()

	priv, pub, err := network.NewMachineKeys()
	require.NoError(t, err)
	hub.AddMachine(clusterID, machineID, pub)

	return tunnel.Credentials{
		ClusterID:  clusterID,
		MachineID:  machineID,
		PrivateKey: priv,
	}
}

func hubClient(t *testing.T, addr string) pb.HubClient {
	t.Helper()

	cc, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { cc.Close() })
	return pb.NewHubClient(cc)
}

// serveConn serves the fake cluster API on the machine end of the tunnel.
func serveConn(t *testing.T, conn net.Conn, cluster *fakeCluster) {
	t.Helper()

	server := grpc.NewServer()
	pb.RegisterClusterServer(server, cluster)
	go server.Serve(newOneConnListener(conn))

	t.Cleanup(server.Stop)
}

type fakeCluster struct {
	pb.UnimplementedClusterServer
	machines []*pb.MachineMember
}

func (f *fakeCluster) ListMachines(context.Context, *emptypb.Empty) (*pb.ListMachinesResponse, error) {
	return &pb.ListMachinesResponse{Machines: f.machines}, nil
}

// oneConnListener accepts a single connection and then blocks until it's closed.
type oneConnListener struct {
	conns  chan net.Conn
	closed chan struct{}
	once   sync.Once
}

func newOneConnListener(conn net.Conn) *oneConnListener {
	l := &oneConnListener{conns: make(chan net.Conn, 1), closed: make(chan struct{})}
	l.conns <- conn
	return l
}

func (l *oneConnListener) Accept() (net.Conn, error) {
	select {
	case conn := <-l.conns:
		return conn, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

func (l *oneConnListener) Close() error {
	l.once.Do(func() { close(l.closed) })
	return nil
}

func (l *oneConnListener) Addr() net.Addr {
	return tunnel.Addr("machine")
}

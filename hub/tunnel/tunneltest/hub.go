// Package tunneltest provides an in-process fake Hub for testing machine tunnels.
package tunneltest

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"

	"github.com/psviderski/uncloud/api/pb"
	"github.com/psviderski/uncloud/hub/tunnel"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Hub is a fake Hub that serves tunnels over plaintext gRPC on localhost. It authenticates machines added with
// AddMachine. ConnectCluster isn't implemented.
type Hub struct {
	pb.UnimplementedHubServer

	addr string

	mu      sync.Mutex
	keys    map[machineKey][]byte
	tunnels map[string]*machineTunnel
	// changed is closed and replaced every time a tunnel opens or closes.
	changed chan struct{}
}

type machineKey struct {
	clusterID string
	machineID string
}

type machineTunnel struct {
	conn *tunnel.Conn
	cc   *grpc.ClientConn
}

// NewHub starts a fake Hub that stops when the test ends.
func NewHub(t testing.TB) *Hub {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	h := &Hub{
		addr:    listener.Addr().String(),
		keys:    make(map[machineKey][]byte),
		tunnels: make(map[string]*machineTunnel),
		changed: make(chan struct{}),
	}
	server := grpc.NewServer()
	pb.RegisterHubServer(server, h)
	go server.Serve(listener)

	t.Cleanup(server.Stop)

	return h
}

// Addr returns the TCP address that the Hub listens on.
func (h *Hub) Addr() string {
	return h.addr
}

// Endpoint returns the Hub API endpoint that machines open tunnels to.
func (h *Hub) Endpoint() string {
	return "http://" + h.addr
}

// AddMachine adds a machine with its WireGuard public key to the machines the Hub knows so it can open tunnels.
func (h *Hub) AddMachine(clusterID, machineID string, publicKey []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.keys[machineKey{clusterID: clusterID, machineID: machineID}] = publicKey
}

// Client waits for the tunnel of the machine to open and returns a gRPC client connection to the Uncloud API
// through it. The connection closes when the tunnel closes.
func (h *Hub) Client(ctx context.Context, machineID string) (*grpc.ClientConn, error) {
	for {
		h.mu.Lock()
		t, ok := h.tunnels[machineID]
		changed := h.changed
		h.mu.Unlock()

		if ok {
			return t.cc, nil
		}
		select {
		case <-changed:
		case <-ctx.Done():
			return nil, fmt.Errorf("wait for tunnel of machine '%s': %w", machineID, ctx.Err())
		}
	}
}

// Connected reports whether the machine has an open tunnel.
func (h *Hub) Connected(machineID string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	_, ok := h.tunnels[machineID]
	return ok
}

// Drop closes the tunnel of the machine if it's open.
func (h *Hub) Drop(machineID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if t, ok := h.tunnels[machineID]; ok {
		t.conn.Close()
	}
}

// Tunnel implements pb.HubServer.
func (h *Hub) Tunnel(stream pb.Hub_TunnelServer) error {
	conn, m, err := tunnel.Accept(stream, h.resolveKey)
	if err != nil {
		return err
	}
	defer conn.Close()

	cc, err := tunnel.NewAPIConn(conn)
	if err != nil {
		return status.Errorf(codes.Internal, "create client connection: %v", err)
	}
	t := &machineTunnel{conn: conn, cc: cc}

	// A new tunnel replaces the previous one of the same machine.
	h.mu.Lock()
	if old, ok := h.tunnels[m.MachineID]; ok {
		old.conn.Close()
	}
	h.tunnels[m.MachineID] = t
	h.notifyLocked()
	h.mu.Unlock()

	select {
	case <-conn.Done():
	case <-stream.Context().Done():
	}

	h.mu.Lock()
	if h.tunnels[m.MachineID] == t {
		delete(h.tunnels, m.MachineID)
		h.notifyLocked()
	}
	h.mu.Unlock()

	return nil
}

func (h *Hub) resolveKey(_ context.Context, clusterID, machineID string) ([]byte, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	key, ok := h.keys[machineKey{clusterID: clusterID, machineID: machineID}]
	if !ok {
		return nil, tunnel.ErrUnknownMachine
	}
	return key, nil
}

func (h *Hub) notifyLocked() {
	close(h.changed)
	h.changed = make(chan struct{})
}

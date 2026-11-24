// Package tunneltest provides an in-process fake Hub for testing machine tunnels.
package tunneltest

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"maps"
	"net"
	"sync"
	"testing"

	"github.com/psviderski/uncloud/api/pb"
	"github.com/psviderski/uncloud/hub/tunnel"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Hub is a fake Hub that serves tunnels over plaintext gRPC. It authenticates machines added with AddMachine or
// connected with ConnectCluster. Like Hub, it accepts an unknown machine if another machine of the same cluster with
// an open tunnel lists it.
type Hub struct {
	pb.UnimplementedHubServer

	addr string

	mu   sync.Mutex
	keys map[machineKey][]byte
	// tokens maps unused connection tokens to the IDs of the clusters they were issued for.
	tokens map[string]string
	// usedTokens maps used connection tokens to the requests that used them.
	usedTokens map[string]usedToken
	tunnels    map[string]*machineTunnel
	// changed is closed and replaced every time a tunnel opens or closes.
	changed chan struct{}
}

// Options configures a fake Hub.
type Options struct {
	// ListenAddr is the TCP address that the Hub listens on. Defaults to a random port on 127.0.0.1.
	ListenAddr string
}

type machineKey struct {
	clusterID string
	machineID string
}

// usedToken is a connection token that connected a cluster.
type usedToken struct {
	clusterID string
	// keys maps the IDs of the machines in the request to their WireGuard public keys.
	keys map[string][]byte
}

type machineTunnel struct {
	clusterID string
	conn      *tunnel.Conn
	cc        *grpc.ClientConn
}

// NewHub starts a fake Hub on localhost that stops when the test ends.
func NewHub(t testing.TB) *Hub {
	t.Helper()
	return NewHubWithOptions(t, Options{})
}

// NewHubWithOptions starts a fake Hub configured with opts that stops when the test ends.
func NewHubWithOptions(t testing.TB, opts Options) *Hub {
	t.Helper()

	addr := opts.ListenAddr
	if addr == "" {
		addr = "127.0.0.1:0"
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	h := &Hub{
		addr:       listener.Addr().String(),
		keys:       make(map[machineKey][]byte),
		tokens:     make(map[string]string),
		usedTokens: make(map[string]usedToken),
		tunnels:    make(map[string]*machineTunnel),
		changed:    make(chan struct{}),
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

// ForgetMachine removes the machine from the machines the Hub knows as if it were added to the cluster after
// the cluster connected. The Hub learns it again from another machine of the cluster when it opens a tunnel.
func (h *Hub) ForgetMachine(clusterID, machineID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	delete(h.keys, machineKey{clusterID: clusterID, machineID: machineID})
}

// IssueToken issues a single-use connection token for the cluster.
func (h *Hub) IssueToken(clusterID string) string {
	h.mu.Lock()
	defer h.mu.Unlock()

	token := rand.Text()
	h.tokens[token] = clusterID
	return token
}

// APIConn waits for the tunnel of the machine to open and returns a gRPC client connection to the Uncloud API
// through it. The connection closes when the tunnel closes.
func (h *Hub) APIConn(ctx context.Context, machineID string) (*grpc.ClientConn, error) {
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

// ConnectCluster implements pb.HubServer. It replaces the machines the Hub knows for the cluster of the token.
func (h *Hub) ConnectCluster(_ context.Context, req *pb.ConnectClusterRequest) (*pb.ConnectClusterResponse, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	keys := make(map[string][]byte, len(req.Machines))
	for _, m := range req.Machines {
		if m.Id == "" || len(m.Network.GetPublicKey()) == 0 {
			return nil, status.Error(codes.InvalidArgument, "machine ID and WireGuard public key must be set")
		}
		keys[m.Id] = m.Network.PublicKey
	}

	// A retry of the request that used the token gets the same response and doesn't change the machines.
	if used, ok := h.usedTokens[req.Token]; ok && maps.EqualFunc(used.keys, keys, bytes.Equal) {
		return &pb.ConnectClusterResponse{ClusterId: used.clusterID}, nil
	}
	clusterID, ok := h.tokens[req.Token]
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "invalid or used connection token")
	}

	delete(h.tokens, req.Token)
	h.usedTokens[req.Token] = usedToken{clusterID: clusterID, keys: keys}

	for k := range h.keys {
		if k.clusterID == clusterID {
			delete(h.keys, k)
		}
	}
	for id, key := range keys {
		h.keys[machineKey{clusterID: clusterID, machineID: id}] = key
	}

	return &pb.ConnectClusterResponse{ClusterId: clusterID}, nil
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
	t := &machineTunnel{clusterID: m.ClusterID, conn: conn, cc: cc}

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

func (h *Hub) resolveKey(ctx context.Context, clusterID, machineID string) ([]byte, error) {
	k := machineKey{clusterID: clusterID, machineID: machineID}

	h.mu.Lock()
	key, ok := h.keys[k]
	var cc *grpc.ClientConn
	if !ok {
		for _, t := range h.tunnels {
			if t.clusterID == clusterID {
				cc = t.cc
				break
			}
		}
	}
	h.mu.Unlock()

	if ok {
		return key, nil
	}
	if cc == nil {
		return nil, tunnel.ErrUnknownMachine
	}

	// Learn the key of a new machine from the machines listed by another machine of the cluster.
	resp, err := pb.NewClusterClient(cc).ListMachines(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, fmt.Errorf("%w: list machines through a tunnel of the cluster: %v", tunnel.ErrUnknownMachine, err)
	}
	for _, m := range resp.Machines {
		if m.Machine.GetId() != machineID || len(m.Machine.GetNetwork().GetPublicKey()) == 0 {
			continue
		}

		h.mu.Lock()
		h.keys[k] = m.Machine.Network.PublicKey
		h.mu.Unlock()
		return m.Machine.Network.PublicKey, nil
	}

	return nil, tunnel.ErrUnknownMachine
}

func (h *Hub) notifyLocked() {
	close(h.changed)
	h.changed = make(chan struct{})
}

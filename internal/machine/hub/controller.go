package hub

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/psviderski/uncloud/api/pb"
	"github.com/psviderski/uncloud/internal/machine/store"
	"github.com/psviderski/uncloud/internal/secret"
	"github.com/siderolabs/grpc-proxy/proxy"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ConnectionSubscriber watches the connection of the cluster to Hub.
type ConnectionSubscriber interface {
	// SubscribeConnection returns the current connection of the cluster to Hub, and a channel that receives an event
	// every time the connection changes. The channel is closed when ctx is cancelled or the subscription fails.
	SubscribeConnection(ctx context.Context) (store.HubConnectionEvent, <-chan store.HubConnectionEvent, error)
}

// TunnelStatus describes the tunnel from the machine to Hub and the cluster connection it's for.
type TunnelStatus struct {
	// Connection is the connection of the cluster to Hub that the machine uses. It's nil if the machine doesn't
	// see one or can't read it.
	Connection *store.HubConnection
	Status
}

// Proto returns the status as a protobuf message.
func (s TunnelStatus) Proto() *pb.HubTunnel {
	t := &pb.HubTunnel{
		State:       stateToProto(s.State),
		Since:       timestampOrNil(s.Since),
		RetryAt:     timestampOrNil(s.RetryAt),
		LastErrorAt: timestampOrNil(s.LastErrorAt),
	}
	if s.Connection != nil {
		t.ClusterId = s.Connection.ClusterID
		t.Endpoint = s.Connection.Endpoint
	}
	if s.LastError != nil {
		t.LastError = s.LastError.Error()
	}
	return t
}

func stateToProto(s State) pb.HubTunnel_State {
	switch s {
	case StateInactive:
		return pb.HubTunnel_INACTIVE
	case StateConnecting:
		return pb.HubTunnel_CONNECTING
	case StateConnected:
		return pb.HubTunnel_CONNECTED
	case StateRetrying:
		return pb.HubTunnel_RETRYING
	case StateFailed:
		return pb.HubTunnel_FAILED
	default:
		return pb.HubTunnel_UNKNOWN
	}
}

func timestampOrNil(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}

// Controller keeps a tunnel from the machine to Hub open while the cluster is connected to Hub. It serves Hub requests
// from the tunnel with a read-only API server, see NewAPIServer.
type Controller struct {
	connections ConnectionSubscriber
	machineID   string
	privateKey  secret.Secret
	director    proxy.StreamDirector

	mu     sync.Mutex
	status TunnelStatus
	// changed is closed and replaced every time the status changes.
	changed chan struct{}
}

// NewController creates a Controller for the machine that routes Hub requests with the director.
func NewController(
	connections ConnectionSubscriber, machineID string, privateKey secret.Secret, director proxy.StreamDirector,
) *Controller {
	return &Controller{
		connections: connections,
		machineID:   machineID,
		privateKey:  privateKey,
		director:    director,
		status:      TunnelStatus{Status: Status{State: StateInactive}},
		changed:     make(chan struct{}),
	}
}

// Status returns the current status of the tunnel and a channel that is closed when the status changes.
func (c *Controller) Status() (TunnelStatus, <-chan struct{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.status, c.changed
}

func (c *Controller) setStatus(conn *store.HubConnection, status Status) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.status = TunnelStatus{Connection: conn, Status: status}
	close(c.changed)
	c.changed = make(chan struct{})
}

// Run opens, reopens, or closes the tunnel every time the connection of the cluster to Hub changes until ctx is
// cancelled. It returns an error if it can't watch the cluster connection.
func (c *Controller) Run(ctx context.Context) error {
	current, events, err := c.connections.SubscribeConnection(ctx)
	if err != nil {
		return fmt.Errorf("subscribe to Hub connection changes: %w", err)
	}

	stop := c.handleInBackground(ctx, current)
	defer func() {
		stop()
	}()

	for {
		var e store.HubConnectionEvent
		var ok bool

		select {
		case e, ok = <-events:
			if !ok {
				if ctx.Err() != nil {
					return nil
				}
				return errors.New("Hub connection subscription failed")
			}
		case <-ctx.Done():
			return nil
		}

		if sameTunnel(current, e) {
			continue
		}
		current = e

		stop()
		stop = c.handleInBackground(ctx, e)
	}
}

// sameTunnel reports whether the events need the same tunnel. An event with an error never needs the same tunnel,
// so the status shows every error and the event after an error is always handled.
func sameTunnel(a, b store.HubConnectionEvent) bool {
	if a.Err != nil || b.Err != nil {
		return false
	}
	if a.Connection == nil || b.Connection == nil {
		return a.Connection == b.Connection
	}
	return a.Connection.Endpoint == b.Connection.Endpoint && a.Connection.ClusterID == b.Connection.ClusterID
}

// handleInBackground runs handleConnection for the event in a goroutine. The returned stop function cancels it
// and waits for it to return.
func (c *Controller) handleInBackground(ctx context.Context, e store.HubConnectionEvent) (stop func()) {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		c.handleConnection(ctx, e)
	}()

	return func() {
		cancel()
		<-done
	}
}

// handleConnection sets the status for the cluster connection in the event. If the cluster is connected, it keeps
// a tunnel to Hub open until ctx is cancelled.
func (c *Controller) handleConnection(ctx context.Context, e store.HubConnectionEvent) {
	now := time.Now()
	if e.Err != nil {
		// The machine can't tell if the cluster is connected, so the tunnel fails rather than becoming inactive.
		err := fmt.Errorf("read cluster connection: %w", e.Err)
		c.setStatus(nil, Status{State: StateFailed, Since: now, LastError: err, LastErrorAt: now})
		slog.Error("Failed to read the connection of the cluster to Hub.", "err", e.Err)
		return
	}

	conn := e.Connection
	if conn == nil {
		c.setStatus(nil, Status{State: StateInactive, Since: now})
		slog.Info("Cluster isn't connected to Hub.")
		return
	}

	slog.Info("Cluster is connected to Hub, opening tunnel.", "endpoint", conn.Endpoint, "cluster_id", conn.ClusterID)
	server := NewAPIServer(c.director)
	defer func() {
		server.Stop()
		slog.Info("API server for Hub stopped.", "endpoint", conn.Endpoint, "cluster_id", conn.ClusterID)
	}()

	connector, err := NewConnector(Config{
		Endpoint:   conn.Endpoint,
		ClusterID:  conn.ClusterID,
		MachineID:  c.machineID,
		PrivateKey: c.privateKey,
	}, server)
	if err != nil {
		c.setStatus(conn, Status{State: StateFailed, Since: now, LastError: err, LastErrorAt: now})
		slog.Error("Failed to start tunnel to Hub.",
			"endpoint", conn.Endpoint, "cluster_id", conn.ClusterID, "err", err)
		return
	}
	connector.onStatus = func(s Status) {
		// Ignore statuses after the handler is stopped, e.g. inactive when the connector returns. The handler for
		// the next cluster connection sets its own status right after, so the status doesn't briefly show inactive.
		if ctx.Err() == nil {
			c.setStatus(conn, s)
		}
	}

	if err = connector.Run(ctx); err != nil {
		now = time.Now()
		c.setStatus(conn, Status{State: StateFailed, Since: now, LastError: err, LastErrorAt: now})
		slog.Error("Tunnel to Hub stopped.", "endpoint", conn.Endpoint, "cluster_id", conn.ClusterID, "err", err)
	}
}

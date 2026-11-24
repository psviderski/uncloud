package hub

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/psviderski/uncloud/api/pb"
	"github.com/psviderski/uncloud/hub/tunnel/tunneltest"
	apiproxy "github.com/psviderski/uncloud/internal/machine/api/proxy"
	"github.com/psviderski/uncloud/internal/machine/network"
	"github.com/psviderski/uncloud/internal/machine/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestController_HandlesClusterConnection(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	hub := tunneltest.NewHub(t)
	ctrl, events := newTestController(t, hub, store.HubConnectionEvent{}, "cluster-2")
	runErr := runController(ctx, ctrl)

	s := waitStatus(t, ctx, ctrl, func(s TunnelStatus) bool { return !s.Since.IsZero() })
	assert.Equal(t, StateInactive, s.State)
	assert.Nil(t, s.Connection)

	t.Run("connected", func(t *testing.T) {
		events <- connectedEvent(hub.Endpoint(), clusterID)
		s := waitStatus(t, ctx, ctrl, func(s TunnelStatus) bool { return s.State == StateConnected })
		require.NotNil(t, s.Connection)
		assert.Equal(t, clusterID, s.Connection.ClusterID)

		// Hub can call the read-only machine API through the tunnel.
		cc, err := hub.Client(ctx, machineID)
		require.NoError(t, err)
		resp, err := pb.NewClusterClient(cc).ListMachines(ctx, &emptypb.Empty{})
		require.NoError(t, err)
		assert.Len(t, resp.Machines, 1)
	})

	t.Run("reconnected to another cluster", func(t *testing.T) {
		events <- connectedEvent(hub.Endpoint(), "cluster-2")
		s := waitStatus(t, ctx, ctrl, func(s TunnelStatus) bool {
			return s.State == StateConnected && s.Connection.ClusterID == "cluster-2"
		})
		assert.Nil(t, s.LastError, "the status of the previous connection must not be kept")
	})

	t.Run("disconnected", func(t *testing.T) {
		events <- store.HubConnectionEvent{}
		s := waitStatus(t, ctx, ctrl, func(s TunnelStatus) bool { return s.State == StateInactive })
		assert.Nil(t, s.Connection)
		assert.Eventually(t, func() bool { return !hub.Connected(machineID) }, 5*time.Second, 10*time.Millisecond)
	})

	t.Run("invalid connection fails", func(t *testing.T) {
		events <- connectedEvent("hub.uncloud.run", clusterID)
		s := waitStatus(t, ctx, ctrl, func(s TunnelStatus) bool { return s.State == StateFailed })
		assert.Error(t, s.LastError)
		assert.Equal(t, clusterID, s.Connection.ClusterID)
	})

	t.Run("stops when cancelled", func(t *testing.T) {
		events <- connectedEvent(hub.Endpoint(), clusterID)
		waitStatus(t, ctx, ctrl, func(s TunnelStatus) bool { return s.State == StateConnected })

		cancel()
		select {
		case err := <-runErr:
			assert.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("controller didn't stop after the context was cancelled")
		}
		assert.Eventually(t, func() bool { return !hub.Connected(machineID) }, 5*time.Second, 10*time.Millisecond)
	})
}

func TestController_ConnectionError(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	hub := tunneltest.NewHub(t)
	ctrl, events := newTestController(t, hub, store.HubConnectionEvent{Err: errors.New("unreadable")})
	runController(ctx, ctrl)

	// The machine can't tell if the cluster is connected, so the tunnel fails rather than becoming inactive.
	s := waitStatus(t, ctx, ctrl, func(s TunnelStatus) bool { return s.State == StateFailed })
	assert.Nil(t, s.Connection)
	assert.ErrorContains(t, s.LastError, "read cluster connection: unreadable")

	events <- store.HubConnectionEvent{}
	waitStatus(t, ctx, ctrl, func(s TunnelStatus) bool { return s.State == StateInactive })

	events <- connectedEvent(hub.Endpoint(), clusterID)
	waitStatus(t, ctx, ctrl, func(s TunnelStatus) bool { return s.State == StateConnected })

	events <- store.HubConnectionEvent{Err: errors.New("unreadable")}
	s = waitStatus(t, ctx, ctrl, func(s TunnelStatus) bool { return s.State == StateFailed })
	assert.Nil(t, s.Connection)
	assert.Eventually(t, func() bool { return !hub.Connected(machineID) }, 5*time.Second, 10*time.Millisecond)

	// The same connection as before the error opens the tunnel again.
	events <- connectedEvent(hub.Endpoint(), clusterID)
	waitStatus(t, ctx, ctrl, func(s TunnelStatus) bool { return s.State == StateConnected })
}

func TestController_SubscriptionClosed(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	hub := tunneltest.NewHub(t)
	ctrl, events := newTestController(t, hub, store.HubConnectionEvent{})
	runErr := runController(ctx, ctrl)

	close(events)
	select {
	case err := <-runErr:
		assert.Error(t, err)
	case <-ctx.Done():
		t.Fatal("controller didn't return after the subscription was closed")
	}
}

// newTestController creates a controller for a machine that the hub knows in the default cluster and the extra
// clusters. The controller starts with the current cluster connection. It returns the channel that feeds
// the controller with cluster connection changes.
func newTestController(
	t *testing.T, hub *tunneltest.Hub, current store.HubConnectionEvent, extraClusters ...string,
) (*Controller, chan store.HubConnectionEvent) {
	t.Helper()

	priv, pub, err := network.NewMachineKeys()
	require.NoError(t, err)
	for _, id := range append([]string{clusterID}, extraClusters...) {
		hub.AddMachine(id, machineID, pub)
	}

	director := apiproxy.NewDirector(serveLocalMachineAPI(t), 0, nil)
	t.Cleanup(director.Close)

	events := make(chan store.HubConnectionEvent)
	subscriber := fakeSubscriber{current: current, events: events}
	return NewController(subscriber, machineID, priv, director.Director), events
}

// runController runs the controller in the background and returns the channel that receives the result of Run.
func runController(ctx context.Context, ctrl *Controller) <-chan error {
	runErr := make(chan error, 1)
	go func() { runErr <- ctrl.Run(ctx) }()
	return runErr
}

// waitStatus waits for the status of the controller to match.
func waitStatus(t *testing.T, ctx context.Context, ctrl *Controller, match func(TunnelStatus) bool) TunnelStatus {
	t.Helper()

	for {
		s, changed := ctrl.Status()
		if match(s) {
			return s
		}
		select {
		case <-changed:
		case <-ctx.Done():
			t.Fatalf("status didn't match, last status: %+v", s)
		}
	}
}

// connectedEvent returns the event for the cluster connected to Hub at endpoint.
func connectedEvent(endpoint, id string) store.HubConnectionEvent {
	return store.HubConnectionEvent{Connection: &store.HubConnection{Endpoint: endpoint, ClusterID: id}}
}

type fakeSubscriber struct {
	current store.HubConnectionEvent
	events  chan store.HubConnectionEvent
}

func (f fakeSubscriber) SubscribeConnection(
	context.Context,
) (store.HubConnectionEvent, <-chan store.HubConnectionEvent, error) {
	return f.current, f.events, nil
}
